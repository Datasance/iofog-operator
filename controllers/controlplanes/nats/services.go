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

package nats

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewNatsHeadlessService creates the headless Service for the NATS StatefulSet.
// When headlessPorts is true: exposes cluster (6222), monitoring (8222), client (4222).
// When false: exposes only cluster (6222); client and monitoring are on the client Service.
func NewNatsHeadlessService(namespace string, labels map[string]string, headlessPorts bool) *corev1.Service {
	ports := []corev1.ServicePort{
		{Name: "cluster", Port: int32(DefaultClusterPort), TargetPort: intstr.FromInt(DefaultClusterPort)},
	}
	if headlessPorts {
		ports = append(ports,
			corev1.ServicePort{Name: "client", Port: int32(DefaultServerPort), TargetPort: intstr.FromInt(DefaultServerPort)},
			corev1.ServicePort{Name: "monitor", Port: int32(DefaultHttpPort), TargetPort: intstr.FromInt(DefaultHttpPort)},
		)
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HeadlessServiceName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labels,
			Ports:     ports,
		},
	}
}

// NewNatsClientService creates the client-facing Service for NATS (leaf, mqtt; optionally client and monitor when headlessPorts is false).
func NewNatsClientService(namespace string, labels map[string]string, serviceType corev1.ServiceType, headlessPorts bool, annotations map[string]string) *corev1.Service {
	ports := []corev1.ServicePort{
		{Name: "leaf", Port: int32(DefaultLeafPort), TargetPort: intstr.FromInt(DefaultLeafPort)},
		{Name: "mqtt", Port: int32(DefaultMqttPort), TargetPort: intstr.FromInt(DefaultMqttPort)},
	}
	if !headlessPorts {
		ports = append(ports,
			corev1.ServicePort{Name: "client", Port: int32(DefaultServerPort), TargetPort: intstr.FromInt(DefaultServerPort)},
			corev1.ServicePort{Name: "monitor", Port: int32(DefaultHttpPort), TargetPort: intstr.FromInt(DefaultHttpPort)},
		)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ClientServiceName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: labels,
			Ports:    ports,
		},
	}
	return svc
}
