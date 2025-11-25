package oci

import (
	"fmt"
	"os"

	stove8sv1beta1 "bud.studio/stove8s/api/v1beta1"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Status struct {
	Stage stove8sv1beta1.SnapShotStatusStage `json:"stage"`
	State stove8sv1beta1.SnapShotStatusState `json:"state"`
}

type Resource struct {
	jobs      map[uuid.UUID]*Status
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

func (rs Resource) Init() (chi.Router, error) {
	k8sClient, err := k8sClientInit()
	if err != nil {
		return nil, err
	}

	rs.k8sClient = k8sClient
	rs.jobs = make(map[uuid.UUID]*Status)

	r := chi.NewRouter()

	r.Get("/{id}", rs.Get)
	r.Get("/", rs.List)
	r.Post("/", rs.Create)

	return r, nil
}
