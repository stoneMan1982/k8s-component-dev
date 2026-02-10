package main

import (
	"custom-deployment-controller/api/appsv1alpha1"
	"custom-deployment-controller/internal/controller"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	// 这里是 main 函数的入口，通常会在这里设置 Manager 和 Controller

	logger := ctrl.Log.WithName("setup")
	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add appsv1alpha1 to scheme")
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add apps/v1 to scheme")
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add core/v1 to scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	reconciler := &controller.CustomDeploymentController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.CustomDeployment{}).
		Owns(&appsv1.Deployment{}).
		Complete(reconciler); err != nil {
		logger.Error(err, "Unable to create controller")
		os.Exit(1)
	}

	logger.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
