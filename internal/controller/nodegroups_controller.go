package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeGroupsReconciler struct {
	Config *rest.Config
}

func (r *NodeGroupsReconciler) Start(ctx context.Context) error {
	return r.StartInformer(ctx)
}

func (r *NodeGroupsReconciler) StartInformer(ctx context.Context) error {
	logger := log.FromContext(ctx)

	clientset := kubernetes.NewForConfigOrDie(r.Config)
	factory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	stop := ctx.Done()

	defer runtime.HandleCrash()

	go factory.Start(stop)

	handle, err := nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.addFunc,
		UpdateFunc: r.updateFunc,
		DeleteFunc: r.deleteFunc,
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

func (r *NodeGroupsReconciler) addFunc(obj interface{}) {
	logger := log.FromContext(context.Background())

	node, ok := obj.(*corev1.Node)
	if !ok {
		logger.Error(fmt.Errorf("failed to cast to node"), "failed to cast to node")
		return
	}

	logger.Info("new node added", "name", node.Name)
}

func (r *NodeGroupsReconciler) updateFunc(oldObj, newObj interface{}) {

}

func (r *NodeGroupsReconciler) deleteFunc(obj interface{}) {
	logger := log.FromContext(context.Background())
	logger.Info("delete not yet implemented")

}

func (r *NodeGroupsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(r)
}
