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

// ShuffleShardSpec defines the desired state of ShuffleShard
type ShuffleShardSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Tenant is a unique identifier representing the tenant to which a ShuffleShard is assigned
	Tenant string `json:"tenant"`
	// NodeGroups is a unique combination of nodes representing an individual ShuffleShard
	NodeGroups []string `json:"nodeGroups"`
}

// ShuffleShardStatus defines the observed state of ShuffleShard
type ShuffleShardStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ShardHash is a hash value representing the unique value of the ShuffleShard
	ShardHash string `json:"shardHash"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status

// ShuffleShard is the Schema for the shuffleshards API
type ShuffleShard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShuffleShardSpec   `json:"spec,omitempty"`
	Status ShuffleShardStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ShuffleShardList contains a list of ShuffleShard
type ShuffleShardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShuffleShard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ShuffleShard{}, &ShuffleShardList{})
}
