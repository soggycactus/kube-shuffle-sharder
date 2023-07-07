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

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ShuffleShardReconciler reconciles a ShuffleShard object
type ShuffleShardReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kube-shuffler-sharder.io,resources=shuffleshards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-shuffler-sharder.io,resources=shuffleshards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-shuffler-sharder.io,resources=shuffleshards/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ShuffleShard object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *ShuffleShardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var shuffleShard v1.ShuffleShard
	if err := r.Get(ctx, req.NamespacedName, &shuffleShard); err != nil {
		logger.Error(err, "unable to fetch ShuffleShard")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sort.Strings(shuffleShard.Spec.NodeGroups)
	nodeGroups := strings.Join(shuffleShard.Spec.NodeGroups, "")
	hasher := sha256.New()
	_, err := hasher.Write([]byte(nodeGroups))
	if err != nil {
		logger.Error(err, "unable to hash shard values")
		return ctrl.Result{}, err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	shuffleShard.Status.ShardHash = hash

	if err := r.Status().Update(ctx, &shuffleShard); err != nil {
		logger.Error(err, "failed to update ShuffleShard")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShuffleShardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add an index field on the ShuffleShard's ShardHash to allow clients to query by this value
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v1.ShuffleShard{}, "status.shardHash", func(o client.Object) []string {
		shuffleShard := o.(*v1.ShuffleShard)
		return []string{shuffleShard.Status.ShardHash}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.ShuffleShard{}).
		Complete(r)
}
