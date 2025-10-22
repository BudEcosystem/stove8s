package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"slices"
	"time"

	"bud.studio/stove8s/internal/version"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/compression"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/runtime-spec/specs-go"
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

func main() {
	idx, err := ociBuild("/home/sinan/c.tar")
	if err != nil {
		log.Fatal("Building oci image: ", err)
		return
	}

	ref, err := name.ParseReference("harbor.bud.studio/stove8s/test:latest")
	if err != nil {
		log.Fatalln("Creating reference", err)
	}

	err = remote.WriteIndex(
		ref,
		idx,
		remote.WithAuth(authn.FromConfig(authn.AuthConfig{
			Username: "robot$stove8s",
			Password: "<change_me>",
		})),
	)
	if err != nil {
		log.Fatalln("Pushing to remote", err)
	}

	_, err = layout.Write("/home/sinan/img.tar", idx)
	if err != nil {
		log.Fatalln("Creating reference", err)
	}
}

func ociBuild(checkpointDumpPath string) (v1.ImageIndex, error) {
	checkpointDump, err := os.Open(checkpointDumpPath)
	if err != nil {
		return nil, err
	}
	defer checkpointDump.Close()

	// TODO: checkpointDumpLayer := stream.NewLayer(checkpointDump)
	checkpointDumpLayer, err := tarball.LayerFromFile(
		checkpointDumpPath,
		// TODO: this is not working, layer is still application/vnd.docker.image.rootfs.diff.tar.gzip
		tarball.WithMediaType(types.OCILayer),
		tarball.WithCompression(compression.None),
	)
	if err != nil {
		return nil, fmt.Errorf("Getting layer from file: %v", err)
	}

	img, err := mutate.AppendLayers(empty.Image, checkpointDumpLayer)
	if err != nil {
		return nil, fmt.Errorf("Appending Layer: %v", err)
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("Getting configFile: %v", err)
	}
	cfg.Architecture = runtime.GOARCH
	cfg.OS = runtime.GOOS
	cfg.Config = v1.Config{
		Labels: map[string]string{
			"studio.bud.stove8s.version": version.Version,
		},
	}

	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		return nil, fmt.Errorf("Mutating configFile: %v", err)
	}

	spec, config, err := dumpInspect(checkpointDump)
	if err != nil {
		return nil, err
	}

	img, err = mutate.Time(img, config.CheckpointedAt)
	if err != nil {
		return nil, fmt.Errorf("Mutating time: %v", err)
	}

	img = mutate.ConfigMediaType(img, types.OCIManifestSchema1)
	idx := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
		Add: img,
		Descriptor: v1.Descriptor{
			MediaType: types.OCIManifestSchema1,
			Platform: &v1.Platform{
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
			},
		},
	})
	annotations, err := annotationsFromDump(spec, config)
	if err != nil {
		return nil, fmt.Errorf("Getting annotations: %v", err)
	}
	idx = mutate.Annotations(idx, annotations).(v1.ImageIndex)

	return idx, nil
}

func annotationsFromDump(spec *specs.Spec, config *ContainerConfig) (map[string]string, error) {
	annotations := make(map[string]string)

	criName, ok := spec.Annotations["io.container.manager"]
	// NOTE: we only support containerd for the initial phase
	// podman and cri-o sets this, so exclude them
	if ok {
		return nil, fmt.Errorf("Unsupported high-level container runtime %v", criName)
	}

	annotations[CheckpointAnnotationEngine] = "containerd"
	annotations[CheckpointAnnotationName] = spec.Annotations["io.kubernetes.cri.container-name"]
	annotations[CheckpointAnnotationPod] = spec.Annotations["io.kubernetes.cri.sandbox-name"]
	annotations[CheckpointAnnotationNamespace] = spec.Annotations["io.kubernetes.cri.sandbox-namespace"]
	annotations[CheckpointAnnotationRootfsImageUserRequested] = config.RootfsImage
	annotations[CheckpointAnnotationRootfsImageName] = config.RootfsImageName
	annotations[CheckpointAnnotationRootfsImageID] = config.RootfsImageRef
	annotations[CheckpointAnnotationRuntimeName] = config.OCIRuntime

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

	var config ContainerConfig
	err = json.Unmarshal(raw[ConfigDumpFile], &config)
	if err != nil {
		return nil, nil, err
	}

	return &spec, &config, nil
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
			return res, fmt.Errorf("Can't extract file %s", index)
		}
	}

	return res, nil
}
