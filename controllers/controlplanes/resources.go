/*
 *  *******************************************************************************
 *  * Copyright (c) 2023 Datasance Teknoloji A.S.
 *  *
 *  * This program and the accompanying materials are made available under the
 *  * terms of the Eclipse Public License v. 2.0 which is available at
 *  * http://www.eclipse.org/legal/epl-2.0
 *  *
 *  * SPDX-License-Identifier: EPL-2.0
 *  *******************************************************************************
 *
 */

package controllers

import (
	"github.com/datasance/iofog-operator/v3/controllers/controlplanes/router"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	standardLabelManagedBy = "iofog-operator"
	standardLabelName      = "pot"
)

// getStandardLabels returns Kubernetes and Datasance standard labels for operator-created resources.
func getStandardLabels(component, instanceName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       standardLabelName,
		"app.kubernetes.io/instance":   instanceName,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/managed-by": standardLabelManagedBy,
		"datasance.com/component":      component,
	}
}

// mergeLabels merges existing labels with standard labels; standard labels take precedence.
func mergeLabels(standard, existing map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range standard {
		merged[k] = v
	}
	return merged
}

// getComponentFromMicroservice returns the component name for labeling ("controller" or "router").
func getComponentFromMicroservice(ms *microservice) string {
	if ms.name == "controller" {
		return "controller"
	}
	return "router"
}

func newServices(namespace, instanceName string, ms *microservice) (svcs []*corev1.Service) {
	labels := mergeLabels(getStandardLabels(getComponentFromMicroservice(ms), instanceName), ms.labels)
	for _, msvcSvc := range ms.services {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        msvcSvc.name,
				Namespace:   namespace,
				Labels:      labels,
				Annotations: msvcSvc.serviceAnnotations,
			},
			Spec: corev1.ServiceSpec{
				Type:                  corev1.ServiceType(msvcSvc.serviceType),
				ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyType(msvcSvc.trafficPolicy),
				LoadBalancerIP:        msvcSvc.loadBalancerAddr,
				Selector:              labels,
			},
		}
		// Add ports
		for _, port := range msvcSvc.ports {
			svc.Spec.Ports = append(svc.Spec.Ports, port)
		}

		svcs = append(svcs, svc)
	}

	return svcs
}

type controllerIngressConfig struct {
	annotations      map[string]string
	ingressClassName string
	host             string
	secretName       string
}

func newControllerIngress(namespace, instanceName string, cfg *controllerIngressConfig) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	labels := getStandardLabels("controller", instanceName)

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pot-controller",
			Namespace:   namespace,
			Labels:      labels,
			Annotations: cfg.annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &cfg.ingressClassName,
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{cfg.host},
					SecretName: cfg.secretName,
				},
			},
			Rules: []networkingv1.IngressRule{
				{
					Host: cfg.host,
					IngressRuleValue: networkingv1.IngressRuleValue{ // Correct field name
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "controller",
											Port: networkingv1.ServiceBackendPort{
												Name: "ecn-viewer",
											},
										},
									},
								},
								{
									Path:     "/api/v3",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "controller",
											Port: networkingv1.ServiceBackendPort{
												Name: "controller-api",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newDeployment(namespace, instanceName string, ms *microservice) *appsv1.Deployment {
	maxUnavailable := intstr.FromInt(0)
	maxSurge := intstr.FromInt(1)
	strategy := appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &maxUnavailable,
			MaxSurge:       &maxSurge,
		},
	}

	if ms.mustRecreateOnRollout {
		strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
	}

	labels := mergeLabels(getStandardLabels(getComponentFromMicroservice(ms), instanceName), ms.labels)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			MinReadySeconds: ms.availableDelay,
			Replicas:        &ms.replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Strategy: strategy,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: ms.name,
					Volumes:            ms.volumes,
					SecurityContext:    ms.securityContext,
				},
			},
		},
	}

	// Handle init containers
	initContainers := &dep.Spec.Template.Spec.InitContainers
	for i := range ms.initContainers {
		msInitCont := &ms.initContainers[i]
		initCont := corev1.Container{
			Name:            msInitCont.name,
			Image:           msInitCont.image,
			Command:         msInitCont.command,
			Args:            msInitCont.args,
			Ports:           msInitCont.ports,
			Env:             msInitCont.env,
			Resources:       msInitCont.resources,
			ReadinessProbe:  msInitCont.readinessProbe,
			LivenessProbe:   msInitCont.livenessProbe,
			VolumeMounts:    msInitCont.volumeMounts,
			ImagePullPolicy: corev1.PullPolicy(msInitCont.imagePullPolicy),
		}
		*initContainers = append(*initContainers, initCont)
	}

	// Handle main containers
	containers := &dep.Spec.Template.Spec.Containers
	for i := range ms.containers {
		msCont := &ms.containers[i]
		cont := corev1.Container{
			Name:            msCont.name,
			Image:           msCont.image,
			Command:         msCont.command,
			Args:            msCont.args,
			Ports:           msCont.ports,
			Env:             msCont.env,
			Resources:       msCont.resources,
			ReadinessProbe:  msCont.readinessProbe,
			LivenessProbe:   msCont.livenessProbe,
			VolumeMounts:    msCont.volumeMounts,
			ImagePullPolicy: corev1.PullPolicy(msCont.imagePullPolicy),
		}
		*containers = append(*containers, cont)
	}

	return dep
}

func newServiceAccount(namespace, instanceName string, ms *microservice) *corev1.ServiceAccount {
	labels := mergeLabels(getStandardLabels(getComponentFromMicroservice(ms), instanceName), ms.labels)
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
			Labels:    labels,
		},
	}
}

func newRoleBinding(namespace, instanceName string, ms *microservice) *rbacv1.RoleBinding {
	labels := mergeLabels(getStandardLabels(getComponentFromMicroservice(ms), instanceName), ms.labels)
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
			Labels:    labels,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: ms.name,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     ms.name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}

func newRole(namespace, instanceName string, ms *microservice) *rbacv1.Role {
	labels := mergeLabels(getStandardLabels(getComponentFromMicroservice(ms), instanceName), ms.labels)
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
			Labels:    labels,
		},
		Rules: ms.rbacRules,
	}
}

func newRouterConfigMap(namespace, instanceName string) *corev1.ConfigMap {
	labels := getStandardLabels("router", instanceName)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pot-router",
			Namespace: namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"skrouterd.json": router.GetConfig(namespace),
		},
	}
}
