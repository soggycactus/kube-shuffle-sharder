package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeGroupsReconciler struct {
	Config                      *rest.Config
	Client                      client.Client
	NodeGroupAutoDiscoveryLabel string
}

func (r *NodeGroupsReconciler) Start(ctx context.Context) error {
	return r.StartInformer(ctx)
}

func (r *NodeGroupsReconciler) StartInformer(ctx context.Context) error {
	logger := log.FromContext(ctx)

	if err := r.initializeDefaultNodeGroups(ctx); err != nil {
		logger.Error(err, "failed to initialize default node group")
		return err
	}

	clientset := kubernetes.NewForConfigOrDie(r.Config)
	factory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	stop := ctx.Done()

	defer runtime.HandleCrash()

	go factory.Start(stop)

	handle, err := nodeInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: r.filterFunc,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    r.addFunc,
			UpdateFunc: r.updateFunc,
			DeleteFunc: r.deleteFunc,
		},
	})
	if err != nil {
		return err
	}

	if !cache.WaitForCacheSync(stop, handle.HasSynced, nodeInformer.HasSynced) {
		err := errors.New("Timed out waiting for caches to sync")
		runtime.HandleError(err)
		return err
	}

	logger.Info("cache synced, informer started")

	<-stop

	return nil
}

func (r *NodeGroupsReconciler) filterFunc(obj interface{}) bool {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	object, err := meta.Accessor(obj)
	if err != nil {
		logger.Error(nil, ErrUnableToCastMeta.Error())
		return false
	}

	if _, ok := object.GetLabels()[r.NodeGroupAutoDiscoveryLabel]; !ok {
		logger.Info("skipping node, auto-discovery label not found", "missingLabel", r.NodeGroupAutoDiscoveryLabel, "nodeName", object.GetName())
		return false
	}

	return true
}

func (r *NodeGroupsReconciler) addFunc(obj interface{}) {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	node, ok := obj.(*corev1.Node)
	if !ok {
		logger.Error(nil, ErrUnableToCastNode.Error())
		return
	}

	group, ok := node.Labels[r.NodeGroupAutoDiscoveryLabel]
	if !ok {
		logger.Error(ErrMissingNodeAutoDiscoveryLabel, fmt.Sprintf("missing label: %v", r.NodeGroupAutoDiscoveryLabel))
		return
	}

	defaultNodeGroups, err := r.getDefaultNodeGroups(ctx)
	if err != nil {
		logger.Error(err, "failed to fetch default node groups")
		return
	}

	_, ok = defaultNodeGroups.Status.NodeGroups[group]
	if !ok {
		defaultNodeGroups.Status.NodeGroups[group] = 1
	} else {
		defaultNodeGroups.Status.NodeGroups[group] += 1
	}

	if err := r.updateDefaultNodeGroups(ctx, defaultNodeGroups); err != nil {
		logger.Error(err, "failed to update NodeGroups")
		return
	}

	logger.Info("new node added", "name", node.Name, "group", group)
}

func (r *NodeGroupsReconciler) updateFunc(oldObj, newObj interface{}) {

}

func (r *NodeGroupsReconciler) deleteFunc(obj interface{}) {
	logger := log.FromContext(context.Background())
	logger.Info("delete not yet implemented")

}

func (r *NodeGroupsReconciler) initializeDefaultNodeGroups(ctx context.Context) error {
	defaultNodeGroups := v1.NodeGroups{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "default",
			ResourceVersion: v1.GroupVersion.Version,
		},
	}

	if err := r.Client.Create(ctx, &defaultNodeGroups); err != nil {
		// if the resource already exists, exit early
		if client.IgnoreAlreadyExists(err) == nil {
			return nil
		}
		return err
	}

	defaultNodeGroups.Status = v1.NodeGroupsStatus{
		NodeGroups: make(map[string]int),
	}

	return r.Client.Status().Update(ctx, &defaultNodeGroups)
}

func (r *NodeGroupsReconciler) getDefaultNodeGroups(ctx context.Context) (*v1.NodeGroups, error) {
	var nodeGroups v1.NodeGroups
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "default"}, &nodeGroups); err != nil {
		return nil, err
	}

	return &nodeGroups, nil
}

func (r *NodeGroupsReconciler) updateDefaultNodeGroups(ctx context.Context, nodeGroups *v1.NodeGroups) error {
	return r.Client.Status().Update(ctx, nodeGroups)
}

func (r *NodeGroupsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(r)
}
