package oci

import (
	"fmt"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("HOME") + "/.kube/config"
		if kubeconfigPath := os.Getenv("KUBECONFIG"); kubeconfigPath != "" {
			kubeconfig = kubeconfigPath
		}

		k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return clientset, nil
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
