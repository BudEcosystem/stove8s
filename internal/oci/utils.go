package oci

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"bud.studio/stove8s/internal/version"
	"github.com/docker/cli/cli/config"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/opencontainers/runtime-spec/specs-go"
	corev1 "k8s.io/api/core/v1"
)

const (
	ConfigDumpFile = "config.dump"
	SpecDumpFile   = "spec.dump"
)

type ContainerConfig struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	RootfsImage     string    `json:"rootfsImage,omitempty"`
	RootfsImageRef  string    `json:"rootfsImageRef,omitempty"`
	RootfsImageName string    `json:"rootfsImageName,omitempty"`
	OCIRuntime      string    `json:"runtime,omitempty"`
	CreatedTime     time.Time `json:"createdTime"`
	CheckpointedAt  time.Time `json:"checkpointedTime"`
	RestoredAt      time.Time `json:"restoredTime"`
	Restored        bool      `json:"restored"`
}

func BuildImage(checkpointDumpPath string) (v1.Image, error) {
	checkpointDump, err := os.Open(checkpointDumpPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := checkpointDump.Close()
		if err != nil {
			log.Fatalln("Closing checkpoint dump", err)
		}
	}()

	// TODO: checkpointDumpLayer := stream.NewLayer(checkpointDump)
	checkpointDumpLayer, err := tarball.LayerFromFile(
		checkpointDumpPath,
	)
	if err != nil {
		return nil, fmt.Errorf("getting layer from file: %v", err)
	}

	img, err := mutate.AppendLayers(empty.Image, checkpointDumpLayer)
	if err != nil {
		return nil, fmt.Errorf("appending Layer: %v", err)
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("getting configFile: %v", err)
	}
	cfg.Architecture = runtime.GOARCH
	cfg.OS = runtime.GOOS
	cfg.Config = v1.Config{
		WorkingDir: "/",
		Labels: map[string]string{
			"studio.bud.stove8s.version": version.Version,
		},
	}
	if len(cfg.History) != 1 {
		return nil, fmt.Errorf("expected len(History) == 1 and got %d", len(cfg.History))
	}
	cfg.History[0].CreatedBy = "stove8s"

	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		return nil, fmt.Errorf("mutating configFile: %v", err)
	}

	spec, dumpConfig, err := dumpInspect(checkpointDump)
	if err != nil {
		return nil, err
	}

	img, err = mutate.Time(img, dumpConfig.CheckpointedAt)
	if err != nil {
		return nil, fmt.Errorf("mutating time: %v", err)
	}

	annotations, err := annotationsFromDump(spec, dumpConfig)
	if err != nil {
		return nil, fmt.Errorf("getting annotations: %v", err)
	}
	img = mutate.Annotations(img, annotations).(v1.Image)

	return img, nil
}

func annotationsFromDump(spec *specs.Spec, containerConfig *ContainerConfig) (map[string]string, error) {
	annotations := make(map[string]string)

	criName, ok := spec.Annotations["io.container.manager"]
	// NOTE: we only support containerd for the initial phase
	// podman and cri-o sets this, so exclude them
	if ok {
		return nil, fmt.Errorf("unsupported high-level container runtime %v", criName)
	}

	annotations[CheckpointAnnotationEngine] = "containerd"
	annotations[CheckpointAnnotationName] = spec.Annotations["io.kubernetes.cri.container-name"]
	annotations[CheckpointAnnotationPod] = spec.Annotations["io.kubernetes.cri.sandbox-name"]
	annotations[CheckpointAnnotationNamespace] = spec.Annotations["io.kubernetes.cri.sandbox-namespace"]
	annotations[CheckpointAnnotationRootfsImageUserRequested] = containerConfig.RootfsImage
	annotations[CheckpointAnnotationRootfsImageName] = containerConfig.RootfsImageName
	annotations[CheckpointAnnotationRootfsImageID] = containerConfig.RootfsImageRef
	annotations[CheckpointAnnotationRuntimeName] = containerConfig.OCIRuntime

	return annotations, nil
}

func dumpInspect(checkpointDump io.Reader) (*specs.Spec, *ContainerConfig, error) {
	files := []string{
		SpecDumpFile,
		ConfigDumpFile,
	}

	raw, err := tarFilesRead(files, checkpointDump)
	if err != nil {
		return nil, nil, err
	}

	var spec specs.Spec
	err = json.Unmarshal(raw[SpecDumpFile], &spec)
	if err != nil {
		return nil, nil, err
	}

	var continerConfig ContainerConfig
	err = json.Unmarshal(raw[ConfigDumpFile], &continerConfig)
	if err != nil {
		return nil, nil, err
	}

	return &spec, &continerConfig, nil
}

func ReferenceIsValid(refStr string, authSecret *corev1.Secret) (bool, error) {
	ref, err := name.ParseReference(refStr)
	if err != nil {
		return false, err
	}

	auth, err := AuthFromK8sSecret(authSecret, ref.Context().Registry.String())
	if err != nil {
		return false, err
	}

	_, err = remote.Head(ref, remote.WithAuth(auth))
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			return false, nil
		} else {
			return false, err
		}

	}

	return true, nil
}

func tarFilesRead(files []string, tarFile io.Reader) (map[string][]byte, error) {
	res := make(map[string][]byte)
	tr := tar.NewReader(tarFile)

	for _, file := range files {
		res[file] = nil
	}

	for {
		tarHeader, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			break
		}

		switch tarHeader.Typeflag {
		case tar.TypeReg:
		default:
			continue
		}

		index := slices.IndexFunc(files, func(file string) bool {
			return file == tarHeader.Name
		})
		if index < 0 {
			continue
		}

		res[tarHeader.Name], err = io.ReadAll(tr)
		if err != nil {
			return res, err
		}
	}

	for index := range res {
		if res[index] == nil {
			return res, fmt.Errorf("can't extract file %s", index)
		}
	}

	return res, nil
}

func AuthFromK8sSecret(secret *corev1.Secret, registry string) (authn.Authenticator, error) {
	if secret.Type != corev1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("secret is not of type %s", corev1.SecretTypeDockerConfigJson)
	}
	dockerConfigBytes, exists := secret.Data[corev1.DockerConfigJsonKey]
	if !exists {
		return nil, errors.New("secret missing .dockerconfigjson field")
	}

	fileCfg, err := config.LoadFromReader(bytes.NewReader(dockerConfigBytes))
	if err != nil {
		return nil, err
	}
	authCfg, err := fileCfg.GetAuthConfig(registry)
	if err != nil {
		return nil, err
	}

	return authn.FromConfig(authn.AuthConfig{
		Username:      authCfg.Username,
		Password:      authCfg.Password,
		Auth:          authCfg.Auth,
		IdentityToken: authCfg.IdentityToken,
		RegistryToken: authCfg.RegistryToken,
	}), nil
}
