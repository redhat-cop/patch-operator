/*
Copyright 2021.

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

package v1alpha1

import (
	utilsv1alpha1 "github.com/redhat-cop/operator-utils/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const PatchControllerFinalizerName = "patch-controller"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PatchSpec defines the desired state of Patch
type PatchSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Patches is a list of patches that should be enforced at runtime.
	// +kubebuilder:validation:Required
	Patch *utilsv1alpha1.PatchSpec `json:"patch,omitempty"`

	// ServiceAccountRef is the service account to be used to run the controllers associated with this configuration
	// +kubebuilder:validation:Required
	// +kubebuilder:default={"name": "default"}
	ServiceAccountRef corev1.LocalObjectReference `json:"serviceAccountRef,omitempty"`
}

// PatchStatus defines the observed state of Patch
type PatchStatus struct {
	// ReconcileStatus this is the general status of the main reconciler
	// +kubebuilder:validation:Optional
	Conditions utilsv1alpha1.Conditions `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	//LockedResourceStatuses contains the reconcile status for each of the managed resources
	// +kubebuilder:validation:Optional
	PatchedResourceStatuses map[string]utilsv1alpha1.Conditions `json:"patchedResourceStatuses,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Patch is the Schema for the patches API
type Patch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PatchSpec   `json:"spec,omitempty"`
	Status PatchStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PatchList contains a list of Patch
type PatchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Patch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Patch{}, &PatchList{})
}
