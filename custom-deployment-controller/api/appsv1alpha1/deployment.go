package appsv1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{
		Group:   "apps.myorg.io",
		Version: "v1alpha1",
	}

	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = schemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&CustomDeployment{},
		&CustomDeploymentList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

type CustomDeploymentSpec struct {
	Replicas int32 `json:"replicas,omitempty"`
}

type CustomDeploymentStatus struct {
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type CustomDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CustomDeploymentSpec   `json:"spec,omitempty"`
	Status            CustomDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type CustomDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomDeployment `json:"items"`
}
