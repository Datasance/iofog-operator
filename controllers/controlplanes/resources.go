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

func newServices(namespace string, ms *microservice) (svcs []*corev1.Service) {
	for _, msvcSvc := range ms.services {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        msvcSvc.name,
				Namespace:   namespace,
				Labels:      ms.labels,
				Annotations: msvcSvc.serviceAnnotations,
			},
			Spec: corev1.ServiceSpec{
				Type:                  corev1.ServiceType(msvcSvc.serviceType),
				ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyType(msvcSvc.trafficPolicy),
				LoadBalancerIP:        msvcSvc.loadBalancerAddr,
				Selector:              ms.labels,
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

func newControllerIngress(namespace string, cfg *controllerIngressConfig) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pot-controller",
			Namespace:   namespace,
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

func newDeployment(namespace string, ms *microservice) *appsv1.Deployment {
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

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
			Labels:    ms.labels,
		},
		Spec: appsv1.DeploymentSpec{
			MinReadySeconds: ms.availableDelay,
			Replicas:        &ms.replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ms.labels,
			},
			Strategy: strategy,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ms.labels,
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

func newServiceAccount(namespace string, ms *microservice) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
		},
	}
}

func newRoleBinding(namespace string, ms *microservice) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
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

func newRole(namespace string, ms *microservice) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ms.name,
			Namespace: namespace,
		},
		Rules: ms.rbacRules,
	}
}

func newRouterConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pot-router",
			Namespace: namespace,
		},
		Data: map[string]string{
			"skrouterd.json": router.GetConfig(namespace),
		},
	}
}
