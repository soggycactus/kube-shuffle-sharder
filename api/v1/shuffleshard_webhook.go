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
	"errors"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var ErrShuffleShardIsImmutable = errors.New("ShuffleShard is immutable")
var ErrMissingTenant = errors.New("spec.tenant must not be empty")
var ErrNotEnoughNodeGroups = errors.New("spec.nodeGroups must contain at least 2 elements")
var ErrEmptyNodeGroup = errors.New("spec.nodeGroups must not contain an empty string")
var ErrDuplicateNodeGroups = errors.New("spec.nodeGroups must contain unique elements")
var ErrCannotCastToShard = errors.New("cannot cast object to ShuffleShard")

func (r *ShuffleShard) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-kube-shuffle-sharder-io-v1-shuffleshard,mutating=false,failurePolicy=fail,sideEffects=None,groups=kube-shuffle-sharder.io,resources=shuffleshards,verbs=create;update,versions=v1,name=vshuffleshard.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ShuffleShard{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ShuffleShard) ValidateCreate() (admission.Warnings, error) {
	if r.Spec.Tenant == "" {
		return nil, ErrMissingTenant
	}

	if len(r.Spec.NodeGroups) < 2 {
		return nil, ErrNotEnoughNodeGroups
	}

	set := make(map[string]struct{})
	for _, group := range r.Spec.NodeGroups {
		if group == "" {
			return nil, ErrEmptyNodeGroup
		}
		set[group] = struct{}{}
	}

	if len(set) != len(r.Spec.NodeGroups) {
		return nil, ErrDuplicateNodeGroups
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ShuffleShard) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	shard, ok := old.(*ShuffleShard)
	if !ok {
		return nil, ErrCannotCastToShard
	}

	if !reflect.DeepEqual(shard.Spec, r.Spec) {
		return nil, ErrShuffleShardIsImmutable
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ShuffleShard) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
