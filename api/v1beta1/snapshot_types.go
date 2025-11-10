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

package v1beta1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ObjectReference struct {
	// +required
	Kind string `json:"kind"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +required
	Name string `json:"name"`
}

type KindReference struct {
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +required
	Name string `json:"name"`
}

type SnapShotSelector struct {
	// +required
	Object ObjectReference `json:"object"`
	// +required
	Container string `json:"container"`
}

type SnapShotInputPolicy string

// TODO: add support for Replace, etc ...
const IfNotPresent SnapShotInputPolicy = "IfNotPresent"

type SnapShotInput struct {
	// +required
	Delay time.Duration `json:"delay"`
	// +required
	Policy SnapShotInputPolicy `json:"policy"`
	// TODO: implement me
	// Schedule string              `json:"schedule"`
}

type SnapShotOutputContainerRegistry struct {
	// +optional
	ImagePushSecret KindReference `json:"imagePushSecret"`
	// +required
	ImageReference string `json:"image_reference"`
}

type SnapShotOutput struct {
	// +required
	ContainerRegistry SnapShotOutputContainerRegistry `json:"containerRegistry"`
}

// SnapShotSpec defines the desired state of SnapShot
type SnapShotSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// +required
	Selector SnapShotSelector `json:"selector"`
	// +required
	Input SnapShotInput `json:"input"`
	// +required
	Output SnapShotOutput `json:"output"`
}

type SnapShotStatusStage string

const (
	CriuDumping SnapShotStatusStage = "CriuDumping"
	Fromating   SnapShotStatusStage = "Fromating"
	Pushing     SnapShotStatusStage = "Pushing"
)

type SnapShotStatusState string

const (
	Idle    SnapShotStatusState = "Idle"
	Started SnapShotStatusState = "Started"
	Failed  SnapShotStatusState = "Failed"
	Success SnapShotStatusState = "Success"
)

// SnapShotStatus defines the observed state of SnapShot.
type SnapShotStatus struct {
	// +kubebuilder:default:=Fromating
	Stage SnapShotStatusStage `json:"stage"`
	// +kubebuilder:default:=Idle
	State SnapShotStatusState `json:"state"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SnapShot is the Schema for the snapshots API
type SnapShot struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of SnapShot
	// +required
	Spec SnapShotSpec `json:"spec"`

	// status defines the observed state of SnapShot
	// +optional
	Status SnapShotStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// SnapShotList contains a list of SnapShot
type SnapShotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SnapShot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SnapShot{}, &SnapShotList{})
}
