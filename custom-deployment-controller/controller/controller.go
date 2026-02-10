package controller

import (
	"context"
	"custom-deployment-controller/appsv1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CustomDeploymentController struct {
	client.Client
	Scheme *runtime.Scheme
}

func (c *CustomDeploymentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cd := &appsv1alpha1.CustomDeployment{}
	if err := c.Get(ctx, req.NamespacedName, cd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	deployName := cd.Name
	deploy := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: deployName, Namespace: cd.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		// 创建 Deployment
		deploy = desiredDeployment(cd)
		if err := ctrl.SetControllerReference(cd, deploy, c.Scheme); err != nil {
			logger.Error(err, "Failed to set owner reference")
			return ctrl.Result{}, err
		}
		if err := c.Create(ctx, deploy); err != nil {
			logger.Error(err, "Failed to create Deployment")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	} else {
		updated := false
		if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != cd.Spec.Replicas {
			deploy.Spec.Replicas = ptr.To(cd.Spec.Replicas)
			updated = true
		}
		if updated {
			if err := c.Update(ctx, deploy); err != nil {
				logger.Error(err, "Failed to update Deployment")
				return ctrl.Result{}, err
			}

			logger.Info("Deployment updated successfully", "name", deploy.Name)
		}
	}

	if cd.Status.AvailableReplicas != deploy.Status.AvailableReplicas {
		cd.Status.AvailableReplicas = deploy.Status.AvailableReplicas
		if err := c.Status().Update(ctx, cd); err != nil {
			logger.Error(err, "Failed to update CustomDeployment status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func desiredDeployment(cd *appsv1alpha1.CustomDeployment) *appsv1.Deployment {
	labels := map[string]string{
		"app": cd.Name,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name,
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(cd.Spec.Replicas),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}
}
