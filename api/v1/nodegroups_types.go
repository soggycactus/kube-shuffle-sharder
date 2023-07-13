/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeGroupsSpec defines the desired state of NodeGroups
type NodeGroupsSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// NodeGroupsStatus defines the observed state of NodeGroups
type NodeGroupsStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// NodeGroups is a map of node groups & the number of nodes contained in each group
	NodeGroups map[string]NodeGroupMap `json:"nodeGroups"`
}

type NodeGroupMap struct {
	Nodes map[string]bool `json:"nodes"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status

// NodeGroups is the Schema for the nodegroups API
type NodeGroups struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeGroupsSpec   `json:"spec,omitempty"`
	Status NodeGroupsStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeGroupsList contains a list of NodeGroups
type NodeGroupsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeGroups `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeGroups{}, &NodeGroupsList{})
}
