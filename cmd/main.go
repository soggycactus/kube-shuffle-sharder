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

package main

import (
	"flag"
	"fmt"
	"os"
	"sync"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kubeshufflersharderiov1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"github.com/soggycactus/kube-shuffle-sharder/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme        = runtime.NewScheme()
	setupLog      = ctrl.Log.WithName("setup")
	certDirectory = os.Getenv("CERT_DIRECTORY")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubeshufflersharderiov1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

var (
	metricsAddr                 string
	enableLeaderElection        bool
	probeAddr                   string
	nodeGroupAutoDiscoveryLabel string
	tenantLabel                 string
	numNodeGroups               int
)

func main() {
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&nodeGroupAutoDiscoveryLabel, "node-group-auto-discovery-label", "kube-shuffle-sharder.io/node-group", "The label to inspect on nodes to determine node group membership.")
	flag.StringVar(&tenantLabel, "tenant-label", "kube-shuffle-sharder.io/tenant", "The label to inspect on pods to determine the tenant.")
	flag.IntVar(&numNodeGroups, "num-node-groups", 2, "The number of node groups to assign each shuffle shard.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Don't allow fewer than 2 node groups, since that defeats the purpose of shuffle sharding
	if numNodeGroups < 2 {
		setupLog.Error(fmt.Errorf("invalid number of node groups, got %d, must be at least 2", numNodeGroups), "unable to start manager")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "26c66254.kube-shuffle-sharder.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	// if CERT_DIRECTORY is specified, we are running locally
	// this manually sets the webhook server's cert
	if certDirectory != "" {
		setupLog.Info("CERT_DIRECTORY environment variable detected, overriding webhook server config")
		options.WebhookServer = webhook.NewServer(webhook.Options{
			CertDir: certDirectory,
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.ShuffleShardReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ShuffleShard")
		os.Exit(1)
	}
	if err = (&controller.PodMutatingWebhook{
		Mu:                          new(sync.Mutex),
		NodeCache:                   make(controller.NodeGroupCollection),
		EndpointGraph:               controller.NewGraph(),
		NodeGroupAutoDiscoveryLabel: nodeGroupAutoDiscoveryLabel,
		TenantLabel:                 tenantLabel,
		NumNodeGroups:               numNodeGroups,
		Decoder:                     admission.NewDecoder(scheme),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "PodMutatingWebhook")
		os.Exit(1)
	}

	if err = (&kubeshufflersharderiov1.ShuffleShard{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ShuffleShard")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
