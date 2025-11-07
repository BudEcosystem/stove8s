package oci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/cli/cli/config"
	"github.com/go-chi/chi/v5"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type ociStatusStage string
type ociStatusState string

const (
	Fromating ociStatusStage = "Fromating"
	Pushing   ociStatusStage = "Pushing"

	Started ociStatusState = "Started"
	Failed  ociStatusState = "Failed"
	Success ociStatusState = "Success"
)

type OciStatus struct {
	Stage  ociStatusStage `json:"stage"`
	State  ociStatusState `json:"state"`
	Reason string         `json:"reason"`
}

type OciResource struct {
	jobs      map[uuid.UUID]*OciStatus
	k8sClient *kubernetes.Clientset
}

func k8sClientInit() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("HOME") + "/.kube/config"
		if kubeconfigPath := os.Getenv("KUBECONFIG"); kubeconfigPath != "" {
			kubeconfig = kubeconfigPath
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return clientset, nil
}

func (rs OciResource) k8sImagePushSecretGet(namespace, secretName, registry string) (authn.Authenticator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	secret, err := rs.k8sClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if secret.Type != corev1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("secret %s is not of type %s", secretName, corev1.SecretTypeDockerConfigJson)
	}
	dockerConfigBytes, exists := secret.Data[corev1.DockerConfigJsonKey]
	if !exists {
		return nil, fmt.Errorf("secret %s missing .dockerconfigjson field", secretName)
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

func (rs OciResource) Init() (chi.Router, error) {
	k8sClient, err := k8sClientInit()
	if err != nil {
		return nil, err
	}

	rs.k8sClient = k8sClient
	rs.jobs = make(map[uuid.UUID]*OciStatus)

	r := chi.NewRouter()

	r.Get("/", rs.List)
	r.Post("/", rs.Create)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", rs.Get)
	})

	return r, nil
}
