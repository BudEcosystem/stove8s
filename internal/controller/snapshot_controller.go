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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	stove8sv1beta1 "bud.studio/stove8s/api/v1beta1"
	"bud.studio/stove8s/internal/daemonset/resources/oci"
	oci_utils "bud.studio/stove8s/internal/oci"
)

const (
	podCaCertPath     = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	podTokenPath      = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	daemonsetNodePort = 31008
)

// SnapShotReconciler reconciles a SnapShot object
type SnapShotReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	kubeletClient http.Client
	podToken      string
}

type CheckPointResp struct {
	Items []string `json:"items"`
}

// +kubebuilder:rbac:groups=stove8s.bud.studio,resources=snapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stove8s.bud.studio,resources=snapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stove8s.bud.studio,resources=snapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources="nodes/checkpoint",verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
// NOTE: breaking down the Reconcile() function furhter will reduce the readability
// nolint: gocyclo
func (r *SnapShotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	snapshot := &stove8sv1beta1.SnapShot{}
	err := r.Get(ctx, req.NamespacedName, snapshot)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			log.Info("Snapshot resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "failed to get snapshot")
		return ctrl.Result{}, err
	}

	pod, requeue, err := r.podFromObjectRef(ctx, snapshot.Spec.Selector.Object, snapshot.Namespace)
	if err != nil {
		if requeue {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}
	err = controllerutil.SetControllerReference(snapshot, pod, r.Scheme)
	if err != nil {
		log.Error(err, "unable to set controller reference")
		return ctrl.Result{}, err
	}

	containerIdx := slices.IndexFunc(pod.Spec.Containers, func(container corev1.Container) bool {
		return container.Name == snapshot.Spec.Selector.Container
	})
	if containerIdx == -1 {
		log.Info("Container not found in Pod", "container", snapshot.Spec.Selector.Container)
		return ctrl.Result{}, nil
	}
	if pod.Spec.Containers[containerIdx].Image == snapshot.Spec.Output.ContainerRegistry.ImageReference {
		// pod already running snapshot image
		return ctrl.Result{}, nil
	}

	// NOTE: stateless till here

	if snapshot.Status.OutPutReferenceIsValid {
		err := r.PodImageUpdate(
			ctx,
			pod,
			snapshot.Spec.Output.ContainerRegistry.ImageReference,
			containerIdx,
			snapshot.Spec.Output.ContainerRegistry.ImagePushSecret.Name,
		)
		if err != nil {
			log.Error(err, "unable to swap the container image")
		}
		return ctrl.Result{}, err
	}

	secretNamespace := snapshot.Spec.Output.ContainerRegistry.ImagePushSecret.Namespace
	if secretNamespace == "" {
		secretNamespace = snapshot.Namespace
	}
	containerRegistrySecret := corev1.Secret{}
	err = r.Get(
		ctx,
		apitypes.NamespacedName{
			Name:      snapshot.Spec.Output.ContainerRegistry.ImagePushSecret.Name,
			Namespace: secretNamespace,
		},
		&containerRegistrySecret,
	)
	if err != nil {
		log.Error(err, "Failed to get image push secret")
		return ctrl.Result{}, err
	}

	valid, err := oci_utils.ReferenceIsValid(snapshot.Spec.Output.ContainerRegistry.ImageReference, &containerRegistrySecret)
	if err != nil {
		log.Error(err, "unable to check output image existence")
		return ctrl.Result{}, err
	}
	if valid {
		snapshot.Status.OutPutReferenceIsValid = true
		if err := r.Status().Update(ctx, snapshot); err != nil {
			log.Error(err, "unable to update Snapshot status")
			return ctrl.Result{}, err
		}
		err := r.PodImageUpdate(
			ctx,
			pod,
			snapshot.Spec.Output.ContainerRegistry.ImageReference,
			containerIdx,
			snapshot.Spec.Output.ContainerRegistry.ImagePushSecret.Name,
		)
		if err != nil {
			log.Error(err, "unable to swap the container image")
		}
		return ctrl.Result{}, err
	}

	readyIdx := slices.IndexFunc(pod.Status.Conditions, func(condition corev1.PodCondition) bool {
		return condition.Type == corev1.PodReady
	})
	if readyIdx == -1 || pod.Status.Conditions[readyIdx].Status != corev1.ConditionTrue {
		log.Info("Pod not in ready status, waiting for events", "Pod", pod.Name)
		return ctrl.Result{}, nil
	}

	if snapshot.Status.Stage == "" {
		snapshot.Status.Stage = stove8sv1beta1.CriuDumping
		snapshot.Status.State = stove8sv1beta1.Started
		if err := r.Status().Update(ctx, snapshot); err != nil {
			log.Error(err, "unable to update Snapshot status")
			return ctrl.Result{}, err
		}
	}

	if snapshot.Status.Node == (stove8sv1beta1.SnapShotStatusNode{}) {
		nodeName, nodeAddr, kubeletPort, err := r.kubeletEndpointFromPod(ctx, pod)
		if err != nil {
			log.Error(err, "unable to get kubelet endpoint for pod")
			return ctrl.Result{}, err
		}
		snapshot.Status.Node = stove8sv1beta1.SnapShotStatusNode{
			Name:          nodeName,
			Addr:          nodeAddr,
			DeamonsetPort: daemonsetNodePort,
			KubeletPort:   kubeletPort,
		}
		if err := r.Status().Update(ctx, snapshot); err != nil {
			log.Error(err, "unable to update Snapshot status")
			return ctrl.Result{}, err
		}
	}

	if snapshot.Status.CheckPointNodePath == "" {
		checkPointNodePath, err := r.checkpoint(ctx, pod, snapshot.Spec.Selector.Container)
		if err != nil {
			snapshot.Status.State = stove8sv1beta1.Failed
			if err := r.Status().Update(ctx, snapshot); err != nil {
				log.Error(err, "unable to update Snapshot status")
			}
			return ctrl.Result{}, err
		}
		snapshot.Status.CheckPointNodePath = checkPointNodePath
		if err := r.Status().Update(ctx, snapshot); err != nil {
			log.Error(err, "unable to update Snapshot status")
			return ctrl.Result{}, err
		}
	}

	if snapshot.Status.JobID == "" {
		jobID, err := daemonsetInit(
			snapshot.Spec.Output.ContainerRegistry,
			snapshot.Status.CheckPointNodePath,
			snapshot.Status.Node,
			secretNamespace,
		)
		if err != nil {
			log.Error(err, "unable init daemonset job")
			return ctrl.Result{}, err
		}
		snapshot.Status.JobID = jobID
		if err := r.Status().Update(ctx, snapshot); err != nil {
			log.Error(err, "unable to update Snapshot status")
			return ctrl.Result{}, err
		}
	}

	if snapshot.Status.Stage != stove8sv1beta1.Pushing || snapshot.Status.State != stove8sv1beta1.Success {
		ociStatus, err := daemonsetStausFetch(snapshot.Status.JobID, snapshot.Status.Node)
		if err != nil {
			log.Error(err, "unable fetch daemonset job status")
			return ctrl.Result{}, err
		}
		snapshot.Status.Stage = ociStatus.Stage
		snapshot.Status.State = ociStatus.State
		if err := r.Status().Update(ctx, snapshot); err != nil {
			log.Error(err, "unable to update Snapshot status with daemonset status")
			return ctrl.Result{}, err
		}
	}
	if snapshot.Status.Stage != stove8sv1beta1.Pushing || snapshot.Status.State != stove8sv1beta1.Success {
		return ctrl.Result{}, nil
	}

	valid, err = oci_utils.ReferenceIsValid(snapshot.Spec.Output.ContainerRegistry.ImageReference, &containerRegistrySecret)
	if err != nil {
		log.Error(err, "unable to check output image existence")
		return ctrl.Result{}, err
	}
	if !valid {
		err := errors.New("invalid reference")
		log.Error(err, "image push is not reflected in container registry")
		return ctrl.Result{}, err
	}

	snapshot.Status.OutPutReferenceIsValid = true
	if err := r.Status().Update(ctx, snapshot); err != nil {
		log.Error(err, "unable to update Snapshot status")
		return ctrl.Result{}, err
	}
	err = r.PodImageUpdate(
		ctx,
		pod,
		snapshot.Spec.Output.ContainerRegistry.ImageReference,
		containerIdx,
		snapshot.Spec.Output.ContainerRegistry.ImagePushSecret.Name,
	)
	if err != nil {
		log.Error(err, "unable to swap the container image")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *SnapShotReconciler) PodImageUpdate(
	ctx context.Context,
	pod *corev1.Pod, imageRef string,
	containerIdx int,
	imagePullSecret string,
) error {
	pod.Spec.Containers[containerIdx].Image = imageRef
	// Forbidden: pod updates may not change fields other than `spec.containers[*].image`
	// pod.Spec.ImagePullSecrets = append(
	// 	pod.Spec.ImagePullSecrets,
	// 	corev1.LocalObjectReference{Name: imagePullSecret},
	// )
	return r.Update(ctx, pod)
}

func daemonsetStausFetch(
	jobID string,
	node stove8sv1beta1.SnapShotStatusNode,
) (*oci.Status, error) {
	ociEndpoint := fmt.Sprintf("http://%s:%v/oci/%s", node.Addr, node.DeamonsetPort, jobID)
	req, err := http.NewRequest(http.MethodGet, ociEndpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var ociStatus oci.Status
	err = json.NewDecoder(resp.Body).Decode(&ociStatus)
	if err != nil {
		return nil, err
	}

	return &ociStatus, nil
}

func daemonsetInit(
	output stove8sv1beta1.SnapShotOutputContainerRegistry,
	checkPointNodePath string,
	node stove8sv1beta1.SnapShotStatusNode,
	secretNamespace string,
) (string, error) {
	data := oci.CreateReq{
		CheckpointDumpPath: checkPointNodePath,
		ImagePushSecret: oci.CreateReqImagePushSecret{
			Name:      output.ImagePushSecret.Name,
			Namespace: secretNamespace,
		},
		ImageReference: output.ImageReference,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	ociEndpoint := fmt.Sprintf("http://%s:%v/oci", node.Addr, node.DeamonsetPort)
	req, err := http.NewRequest(http.MethodPost, ociEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	var createResp oci.CreateResp
	err = json.NewDecoder(resp.Body).Decode(&createResp)
	if err != nil {
		return "", err
	}

	return createResp.JobID, nil
}

func (r *SnapShotReconciler) kubeletEndpointFromPod(ctx context.Context, pod *corev1.Pod) (string, string, int32, error) {
	node := corev1.Node{}

	err := r.Get(ctx, apitypes.NamespacedName{Name: pod.Spec.NodeName}, &node)
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to get node: %v", err)
	}
	idx := slices.IndexFunc(node.Status.Addresses, func(addr corev1.NodeAddress) bool {
		return addr.Type == corev1.NodeInternalIP
	})
	if idx < 0 {
		return "", "", -1, fmt.Errorf("fo internaIP for node: %v", node.Name)
	}

	name := node.Name
	addr := node.Status.Addresses[idx].Address
	port := node.Status.DaemonEndpoints.KubeletEndpoint.Port
	return name, addr, port, nil
}

func (r *SnapShotReconciler) checkpoint(ctx context.Context, pod *corev1.Pod, containerName string) (string, error) {
	_, nodeAddr, kubeletPort, err := r.kubeletEndpointFromPod(ctx, pod)
	if err != nil {
		return "", fmt.Errorf("failed to kubelet endpoint: %v", err)
	}
	url := fmt.Sprintf(
		"https://%v:%v/checkpoint/%v/%v/%v",
		nodeAddr,
		kubeletPort,
		pod.Namespace,
		pod.Name,
		containerName,
	)
	checkpointReq, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating http request object: %v", err)
	}
	checkpointReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", r.podToken))
	resp, err := r.kubeletClient.Do(checkpointReq)
	if err != nil {
		return "", fmt.Errorf("checkpoint request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var cr CheckPointResp
	err = json.NewDecoder(resp.Body).Decode(&cr)
	if err != nil {
		return "", err
	}
	if len(cr.Items) != 1 {
		return "", errors.New("unexpected output")
	}

	return cr.Items[0], nil
}

func (r *SnapShotReconciler) podFromObjectRef(
	ctx context.Context,
	obj stove8sv1beta1.ObjectReference,
	snapShotNamespace string,
) (*corev1.Pod, bool, error) {
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
	podToken, err := os.ReadFile(podTokenPath)
	if err != nil {
		return err
	}
	r.podToken = string(podToken)

	return ctrl.NewControllerManagedBy(mgr).
		For(&stove8sv1beta1.SnapShot{}).
		Owns(&corev1.Pod{}).
		Named("snapshot").
		Complete(r)
}
