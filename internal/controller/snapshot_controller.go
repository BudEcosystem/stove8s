/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	stove8sv1beta1 "bud.studio/stove8s/api/v1beta1"
)

const podCaCertPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

// SnapShotReconciler reconciles a SnapShot object
type SnapShotReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	kubeletClient http.Client
}

type CheckPointResp struct {
	Items []string `json:"items"`
}

// +kubebuilder:rbac:groups=stove8s.bud.studio,resources=snapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stove8s.bud.studio,resources=snapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stove8s.bud.studio,resources=snapshots/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *SnapShotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	snapshot := &stove8sv1beta1.SnapShot{}
	err := r.Get(ctx, req.NamespacedName, snapshot)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("memcached resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get memcached")
		return ctrl.Result{}, err
	}

	pod, requeue, err := r.podFromObjectRef(ctx, snapshot.Spec.Selector.Object, snapshot.Namespace)
	if err != nil {
		if requeue {
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	snapshot.Status.Stage = stove8sv1beta1.CriuDumping
	// checkPointNodePath, err := r.checkpoint(ctx, pod, snapshot.Spec.Selector.Container)
	_, err = r.checkpoint(ctx, pod, snapshot.Spec.Selector.Container)
	if err != nil {
		if requeue {
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *SnapShotReconciler) checkpoint(ctx context.Context, pod *corev1.Pod, containerName string) (string, error) {
	log := logf.FromContext(ctx)

	idx := slices.IndexFunc(pod.Spec.Containers, func(container corev1.Container) bool {
		return container.Name == containerName
	})
	if idx < 0 {
		err := errors.New("no such container")
		log.Error(err, containerName)
		return "", err
	}

	node := corev1.Node{}
	err := r.Get(ctx, apitypes.NamespacedName{Name: pod.Spec.NodeName}, &node)
	if err != nil {
		log.Error(err, "Failed to get node")
		return "", err
	}

	idx = slices.IndexFunc(node.Status.Addresses, func(addr corev1.NodeAddress) bool {
		return addr.Type == corev1.NodeInternalIP
	})
	if idx < 0 {
		err := errors.New("no internaIP for node")
		log.Error(err, node.Name)
		return "", err
	}

	url := fmt.Sprintf(
		"https://%v:%v/checkpoint/%v/%v/%v",
		node.Status.Addresses[idx].Address,
		node.Status.DaemonEndpoints.KubeletEndpoint,
		pod.Namespace,
		pod.Name,
		containerName,
	)
	checkpointReq, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Error(err, "Creating http request object")
		return "", err
	}
	resp, err := r.kubeletClient.Do(checkpointReq)
	if err != nil {
		log.Error(err, "Checkpoint request failed")
		return "", err
	}

	var cr CheckPointResp
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err, "Reading response body")
		return "", err
	}

	err = json.Unmarshal(body, &cr)
	if err != nil {
		log.Error(err, "Unmarsheling response")
		return "", err
	}

	if len(cr.Items) != 1 {
		err := errors.New("unexpected output, ooga booga")
		log.Error(err, node.Name)
		return "", err
	}

	return cr.Items[0], nil
}

func (r *SnapShotReconciler) podFromObjectRef(ctx context.Context, obj stove8sv1beta1.ObjectReference, snapShotNamespace string) (*corev1.Pod, bool, error) {
	log := logf.FromContext(ctx)
	pod := &corev1.Pod{}
	var namespacedName apitypes.NamespacedName

	namespace := obj.Namespace
	if namespace == "" {
		namespace = snapShotNamespace
	}

	switch obj.Kind {
	// TODO: implement deployment, job, etc...
	case "Pod":
		namespacedName = apitypes.NamespacedName{
			Namespace: namespace,
			Name:      obj.Name,
		}
	default:
		err := fmt.Errorf("unsupported kind: %v", obj.Kind)
		log.Error(err, obj.Kind)
		return nil, false, err
	}

	err := r.Get(ctx, namespacedName, pod)
	if err != nil {
		log.Error(err, "Failed to get pod")
		return nil, true, err
	}

	return pod, false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SnapShotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	caCert, err := os.ReadFile(podCaCertPath)
	if err != nil {
		return err
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	if !ok {
		return errors.New("failed to get cert from PEM")
	}

	r.kubeletClient = http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&stove8sv1beta1.SnapShot{}).
		Named("snapshot").
		Complete(r)
}
