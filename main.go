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

package main

import (
	"flag"
	"os"

	appsv3 "github.com/datasance/iofog-operator/v3/apis/apps/v3"
	cpv3 "github.com/datasance/iofog-operator/v3/apis/controlplanes/v3"
	appscontroller "github.com/datasance/iofog-operator/v3/controllers/apps"
	controlplanescontroller "github.com/datasance/iofog-operator/v3/controllers/controlplanes"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var scheme = runtime.NewScheme() //nolint:gochecknoglobals

func init() { //nolint:gochecknoinits
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(appsv3.AddToScheme(scheme))
	utilruntime.Must(cpv3.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
} //nolint:wsl

// getWatchNamespace returns the Namespace the operator should be watching for changes.
func getWatchNamespace() string {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	ns, _ := os.LookupEnv("WATCH_NAMESPACE")

	return ns
}

func main() {
	setupLog := ctrl.Log.WithName("setup")

	var metricsAddr string

	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:           scheme,
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "44586fd0.datasance.com",
		Metrics:          server.Options{BindAddress: metricsAddr},
		Cache:            cache.Options{
						  DefaultNamespaces: map[string]cache.Config{
						  getWatchNamespace(): {},
						  },
		},
		// WebhookServer:    wb.NewServer(webhook.Options{Port: 9443}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&appscontroller.ApplicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Application"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Application")
		os.Exit(1)
	}

	if err = (&controlplanescontroller.ControlPlaneReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ControlPlane"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ControlPlane")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
