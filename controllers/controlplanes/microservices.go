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
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	cpv3 "github.com/datasance/iofog-operator/v3/apis/controlplanes/v3"
	"github.com/datasance/iofog-operator/v3/controllers/controlplanes/router"
	"github.com/datasance/iofog-operator/v3/internal/util"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	routerName                                     = "router"
	controllerName                                 = "controller"
	controllerCredentialsSecretName                = "controller-credentials"
	emailSecretKey                                 = "email"
	passwordSecretKey                              = "password"
	controlllerAuthCredentialsSecretName           = "controller-auth-credentials" //nolint:gosec
	controlllerAuthUrlSecretKey                    = "auth-url"
	controlllerAuthRealmSecretKey                  = "auth-realm"
	controlllerAuthRealmKeySecretKey               = "auth-realm-key"
	controlllerAuthSSLSecretKey                    = "auth-ssl-req"
	controlllerAuthControllerClientSecretKey       = "auth-controller-client"
	controlllerAuthControllerClientSecretSecretKey = "auth-controller-client-secret"
	controlllerAuthViewerClientSecretKey           = "auth-viewer-client"
	controllerDBCredentialsSecretName              = "controller-db-credentials" //nolint:gosec
	controllerDBUserSecretKey                      = "username"
	controllerDBDBNameSecretKey                    = "dbname"
	controllerDBPasswordSecretKey                  = "password"
	controllerDBHostSecretKey                      = "host"
	controllerDBPortSecretKey                      = "port"
)

type service struct {
	name               string
	loadBalancerAddr   string
	trafficPolicy      string
	serviceType        string
	serviceAnnotations map[string]string
	ports              []corev1.ServicePort
}

type microservice struct {
	name                  string
	services              []service
	imagePullSecret       string
	replicas              int32
	containers            []container
	labels                map[string]string
	annotations           map[string]string
	secrets               []corev1.Secret
	volumes               []corev1.Volume
	rbacRules             []rbacv1.PolicyRule
	mustRecreateOnRollout bool
	availableDelay        int32
}

type container struct {
	name            string
	image           string
	imagePullPolicy string
	args            []string
	readinessProbe  *corev1.Probe
	env             []corev1.EnvVar
	command         []string
	ports           []corev1.ContainerPort
	resources       corev1.ResourceRequirements
	volumeMounts    []corev1.VolumeMount
}

type controllerMicroserviceConfig struct {
	replicas           int32
	image              string
	imagePullSecret    string
	serviceType        string
	serviceAnnotations map[string]string
	https              *bool
	scheme             string
	secretName         string
	loadBalancerAddr   string
	auth               *cpv3.Auth
	db                 *cpv3.Database
	proxyImage         string
	routerImage        string
	ecn                string
	pidBaseDir         string
	ecnViewerPort      int
	ecnViewerURL       string
}

func filterControllerConfig(cfg *controllerMicroserviceConfig) {
	if cfg.replicas == 0 {
		cfg.replicas = 1
	}

	if cfg.image == "" {
		cfg.image = util.GetControllerImage()
	}

	if cfg.serviceType == "" {
		cfg.serviceType = string(corev1.ServiceTypeLoadBalancer)
	}

	if cfg.ecnViewerPort == 0 {
		cfg.ecnViewerPort = 8008
	}

	if cfg.pidBaseDir == "" {
		cfg.pidBaseDir = "/home/runner"
	}

	if cfg.https == nil || *cfg.https == false {
		cfg.scheme = "http"
	} else {
		cfg.scheme = "https"
	}

}

func getControllerPort(msvc *microservice) (int, error) {
	if len(msvc.services) == 0 || len(msvc.services[0].ports) == 0 {
		return 0, errors.New("controller microservice does not have requisite ports")
	}

	return int(msvc.services[0].ports[0].Port), nil
}

func newControllerMicroservice(namespace string, cfg *controllerMicroserviceConfig) *microservice {
	filterControllerConfig(cfg)

	msvc := &microservice{
		availableDelay: 5,
		name:           "controller",
		labels: map[string]string{
			"name": "controller",
		},
		imagePullSecret: cfg.imagePullSecret,
		replicas:        cfg.replicas,
		services: []service{
			{
				name:               "controller",
				serviceType:        cfg.serviceType,
				serviceAnnotations: cfg.serviceAnnotations,
				trafficPolicy:      getTrafficPolicy(cfg.serviceType),
				loadBalancerAddr:   cfg.loadBalancerAddr,
				ports: []corev1.ServicePort{
					{
						Name:       "controller-api",
						Port:       51121,
						TargetPort: intstr.FromInt(51121),
						Protocol:   corev1.Protocol("TCP"),
					},
					{
						Name:       "ecn-viewer",
						Port:       80,
						TargetPort: intstr.FromInt(cfg.ecnViewerPort),
						Protocol:   corev1.Protocol("TCP"),
					},
				},
			},
		},
		secrets: []corev1.Secret{
			{
				Type: corev1.SecretTypeOpaque,
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      controllerDBCredentialsSecretName,
				},
				StringData: map[string]string{
					controllerDBDBNameSecretKey:   cfg.db.DatabaseName,
					controllerDBHostSecretKey:     cfg.db.Host,
					controllerDBPortSecretKey:     strconv.Itoa(cfg.db.Port),
					controllerDBUserSecretKey:     cfg.db.User,
					controllerDBPasswordSecretKey: cfg.db.Password,
				},
			},
			{
				Type: corev1.SecretTypeOpaque,
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      controlllerAuthCredentialsSecretName,
				},
				StringData: map[string]string{
					controlllerAuthUrlSecretKey:                    cfg.auth.URL,
					controlllerAuthRealmSecretKey:                  cfg.auth.Realm,
					controlllerAuthRealmKeySecretKey:               cfg.auth.RealmKey,
					controlllerAuthSSLSecretKey:                    cfg.auth.SSL,
					controlllerAuthControllerClientSecretKey:       cfg.auth.ControllerClient,
					controlllerAuthControllerClientSecretSecretKey: cfg.auth.ControllerSecret,
					controlllerAuthViewerClientSecretKey:           cfg.auth.ViewerClient,
				},
			},
		},
		volumes: []corev1.Volume{},
		containers: []container{
			{
				name:            "controller",
				image:           cfg.image,
				imagePullPolicy: "Always",
				readinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path:   "/api/v3/status",
							Port:   intstr.FromInt(51121), //nolint:gomnd
							Scheme: corev1.URIScheme(strings.ToUpper(cfg.scheme)),
						},
					},
					InitialDelaySeconds: 10,
					TimeoutSeconds:      10,
					PeriodSeconds:       5,
					FailureThreshold:    2,
				},
				volumeMounts: []corev1.VolumeMount{},
				env: []corev1.EnvVar{
					{
						Name: "KC_URL",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthUrlSecretKey,
							},
						},
					},
					{
						Name: "KC_REALM",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthRealmSecretKey,
							},
						},
					},
					{
						Name: "KC_REALM_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthRealmKeySecretKey,
							},
						},
					},
					{
						Name: "KC_SSL_REQ",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthSSLSecretKey,
							},
						},
					},
					{
						Name: "KC_CLIENT",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthControllerClientSecretKey,
							},
						},
					},
					{
						Name: "KC_CLIENT_SECRET",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthControllerClientSecretSecretKey,
							},
						},
					},
					{
						Name: "KC_VIEWER_CLIENT",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthViewerClientSecretKey,
							},
						},
					},
					{
						Name:  "DB_PROVIDER",
						Value: cfg.db.Provider,
					},
					{
						Name: "DB_NAME",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controllerDBCredentialsSecretName,
								},
								Key: controllerDBDBNameSecretKey,
							},
						},
					},
					{
						Name: "DB_USERNAME",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controllerDBCredentialsSecretName,
								},
								Key: controllerDBUserSecretKey,
							},
						},
					},
					{
						Name: "DB_PASSWORD",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controllerDBCredentialsSecretName,
								},
								Key: controllerDBPasswordSecretKey,
							},
						},
					},
					{
						Name: "DB_HOST",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controllerDBCredentialsSecretName,
								},
								Key: controllerDBHostSecretKey,
							},
						},
					},
					{
						Name: "DB_PORT",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controllerDBCredentialsSecretName,
								},
								Key: controllerDBPortSecretKey,
							},
						},
					},
					{
						Name:  "SystemImages_Proxy_1",
						Value: cfg.proxyImage,
					},
					{
						Name:  "SystemImages_Proxy_2",
						Value: cfg.proxyImage,
					},
					{
						Name:  "SystemImages_Router_1",
						Value: cfg.routerImage,
					},
					{
						Name:  "SystemImages_Router_2",
						Value: cfg.routerImage,
					},
					{
						Name:  "ECN_NAME",
						Value: cfg.ecn,
					},
					{
						Name:  "PID_BASE",
						Value: cfg.pidBaseDir,
					},
					{
						Name:  "VIEWER_PORT",
						Value: strconv.Itoa(cfg.ecnViewerPort),
					},
					{
						Name:  "VIEWER_URL",
						Value: cfg.ecnViewerURL,
					},
				},
				// resources: corev1.ResourceRequirements{
				// 	Limits: corev1.ResourceList{
				// 		"cpu":    resource.MustParse("1800m"),
				// 		"memory": resource.MustParse("3Gi"),
				// 	},
				// 	Requests: corev1.ResourceList{
				// 		"cpu":    resource.MustParse("400m"),
				// 		"memory": resource.MustParse("1Gi"),
				// 	},
				// },
			},
		},
	}

	// Add PVC details if no external DB provided
	if cfg.db.Host == "" {
		msvc.mustRecreateOnRollout = true
		msvc.volumes = append(msvc.volumes, corev1.Volume{
			Name: "controller-sqlite",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "controller-sqlite",
					ReadOnly:  false,
				},
			},
		})

		msvc.containers[0].volumeMounts = append(msvc.containers[0].volumeMounts, corev1.VolumeMount{
			Name:      "controller-sqlite",
			MountPath: "home/runner/.npm-global/lib/node_modules/@datasance/iofogcontroller/src/data/sqlite_files/",
			SubPath:   "prod_database.sqlite",
		})
	}

	// Add TLS secret details if type is https and secretname is provided
	if cfg.https != nil && *cfg.https == true {
		msvc.volumes = append(msvc.volumes, corev1.Volume{
			Name: "controller-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cfg.secretName,
				},
			},
		})

		msvc.containers[0].volumeMounts = append(msvc.containers[0].volumeMounts, corev1.VolumeMount{
			Name:      "controller-cert",
			MountPath: "/etc/pot/controller-cert/",
		})

		msvc.containers[0].command = []string{
			"/bin/sh",
			"-c",
		}

		msvc.containers[0].args = []string{
			// Check if ca.crt exists and conditionally configure the controller
			`if [ -f /etc/pot/controller-cert/ca.crt ]; then 
				iofog-controller config add -c /etc/pot/controller-cert/tls.crt && \
				iofog-controller config add -i /etc/pot/controller-cert/ca.crt && \
				iofog-controller config add -k /etc/pot/controller-cert/tls.key && \
				iofog-controller config dev-mode --off; 
			else 
				iofog-controller config add -c /etc/pot/controller-cert/tls.crt && \
				iofog-controller config add -i /etc/pot/controller-cert/tls.crt && \
				iofog-controller config add -k /etc/pot/controller-cert/tls.key && \
				iofog-controller config dev-mode --off; 
			fi && \
			node /home/runner/.npm-global/lib/node_modules/@datasance/iofogcontroller/src/server.js`,
		}

	}

	return msvc
}

type portManagerConfig struct {
	image              string
	imagePullSecret    string
	proxyImage         string
	https              *bool
	scheme             string
	serviceAnnotations map[string]string
	httpProxyAddress   string
	tcpProxyAddress    string
	watchNamespace     string
}

func filterPortManagerConfig(cfg *portManagerConfig) {
	if cfg.image == "" {
		cfg.image = util.GetPortManagerImage()
	}

	if cfg.proxyImage == "" {
		cfg.proxyImage = util.GetProxyImage()
	}

	if cfg.https == nil || *cfg.https == false {
		cfg.scheme = "http"
	} else {
		cfg.scheme = "https"
	}
}

func newPortManagerMicroservice(cfg *portManagerConfig) *microservice {
	filterPortManagerConfig(cfg)

	// Convert the serviceAnnotations map to a JSON string
	var serviceAnnotationsJSON string
	if len(cfg.serviceAnnotations) > 0 {
		annotations, err := json.Marshal(cfg.serviceAnnotations)
		if err == nil {
			serviceAnnotationsJSON = string(annotations)
		}
	}

	return &microservice{
		mustRecreateOnRollout: true,
		name:                  portManagerDeploymentName,
		labels: map[string]string{
			"name": "port-manager",
		},
		imagePullSecret: cfg.imagePullSecret,
		replicas:        1,
		rbacRules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
				APIGroups: []string{"", "apps"},
				Resources: []string{"deployments", "services", "pods", "configmaps"},
			},
		},
		secrets: []corev1.Secret{
			{
				Type: corev1.SecretTypeOpaque,
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfg.watchNamespace,
					Name:      controllerCredentialsSecretName,
				},
			},
		},
		volumes: []corev1.Volume{},
		containers: []container{
			{
				name:            "port-manager",
				image:           cfg.image,
				imagePullPolicy: "Always",
				readinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{
							Command: []string{
								"stat",
								"/tmp/operator-sdk-ready",
							},
						},
					},
					InitialDelaySeconds: 4,
					TimeoutSeconds:      10,
					PeriodSeconds:       5,
					FailureThreshold:    2,
				},
				// resources: corev1.ResourceRequirements{
				// 	 Limits: corev1.ResourceList{
				// 	 	"cpu":    resource.MustParse("200m"),
				// 	 	"memory": resource.MustParse("1Gi"),
				// 	 },
				// 	 Requests: corev1.ResourceList{
				// 	 	"cpu":    resource.MustParse("50m"),
				// 	 	"memory": resource.MustParse("200Mi"),
				// 	 },
				// },
				volumeMounts: []corev1.VolumeMount{},
				env: []corev1.EnvVar{
					{
						Name:  "WATCH_NAMESPACE",
						Value: cfg.watchNamespace,
					},
					{
						Name: "POD_NAME",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "metadata.name",
							},
						},
					},
					{
						Name:  "OPERATOR_NAME",
						Value: "port-manager",
					},
					{
						Name: "KC_URL",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthUrlSecretKey,
							},
						},
					},
					{
						Name: "KC_REALM",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthRealmSecretKey,
							},
						},
					},
					{
						Name: "KC_CLIENT",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthControllerClientSecretKey,
							},
						},
					},
					{
						Name: "KC_CLIENT_SECRET",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: controlllerAuthCredentialsSecretName,
								},
								Key: controlllerAuthControllerClientSecretSecretKey,
							},
						},
					},
					{
						Name:  "PROXY_IMAGE",
						Value: cfg.proxyImage,
					},
					{
						Name:  "HTTP_PROXY_ADDRESS",
						Value: cfg.httpProxyAddress,
					},
					{
						Name:  "TCP_PROXY_ADDRESS",
						Value: cfg.tcpProxyAddress,
					},
					{
						Name:  "ROUTER_ADDRESS",
						Value: routerName,
					},
					{
						Name:  "PROXY_SERVICE_ANNOTATIONS",
						Value: string(serviceAnnotationsJSON),
					},
					{
						Name:  "CONTROLLER_SCHEME",
						Value: cfg.scheme,
					},
					{
						Name:  "PULL_SECRET_NAME",
						Value: cfg.imagePullSecret,
					},
				},
			},
		},
	}
}

type routerMicroserviceConfig struct {
	image              string
	imagePullSecret    string
	serviceType        string
	serviceAnnotations map[string]string
	volumeMountPath    string
}

func filterRouterConfig(cfg routerMicroserviceConfig) routerMicroserviceConfig {
	if cfg.image == "" {
		cfg.image = util.GetRouterImage()
	}

	if cfg.serviceType == "" {
		cfg.serviceType = string(corev1.ServiceTypeLoadBalancer)
	}

	return cfg
}

func newRouterMicroservice(cfg routerMicroserviceConfig) *microservice {
	cfg = filterRouterConfig(cfg)

	return &microservice{
		name: routerName,
		labels: map[string]string{
			"name":                 routerName,
			"application":          "interior-router",
			"skupper.io/component": "router",
		},
		annotations: map[string]string{
			"prometheus.io/port":   "9090",
			"prometheus.io/scrape": "true",
		},
		services: []service{
			{
				name:               "router",
				serviceType:        cfg.serviceType,
				serviceAnnotations: cfg.serviceAnnotations,
				trafficPolicy:      getTrafficPolicy(cfg.serviceType),
				ports: []corev1.ServicePort{
					{
						Name:       "router-message",
						Port:       router.MessagePort,
						TargetPort: intstr.FromInt(router.MessagePort),
						Protocol:   corev1.Protocol("TCP"),
					},
					{
						Name:       "router-interior",
						Port:       router.InteriorPort,
						TargetPort: intstr.FromInt(router.InteriorPort),
						Protocol:   corev1.Protocol("TCP"),
					},
					{
						Name:       "router-edge",
						Port:       router.EdgePort,
						TargetPort: intstr.FromInt(router.EdgePort),
						Protocol:   corev1.Protocol("TCP"),
					},
				},
			},
		},
		imagePullSecret: cfg.imagePullSecret,
		replicas:        1,
		rbacRules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			},
		},
		volumes: []corev1.Volume{
			{
				Name: routerName + "-internal",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "router-internal",
					},
				},
			},
			{
				Name: routerName + "-amqps",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: routerName + "-amqps",
					},
				},
			},
		},
		containers: []container{
			{
				name:            routerName,
				image:           cfg.image,
				imagePullPolicy: "Always",
				command: []string{
					"/home/skrouterd/bin/launch.sh",
				},
				readinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Port: intstr.FromInt(9090), //nolint:gomnd
							Path: "/healthz",
						},
					},
					InitialDelaySeconds: 10,
					TimeoutSeconds:      10,
					PeriodSeconds:       5,
					FailureThreshold:    2,
				},
				env: []corev1.EnvVar{
					{
						Name:  "APPLICATION_NAME",
						Value: routerName,
					},
					{
						Name:  "QDROUTERD_AUTO_MESH_DISCOVERY",
						Value: "QUERY",
					},
					{
						Name:  "QDROUTERD_CONF",
						Value: router.GetConfig(),
					},
					{
						Name: "POD_NAMESPACE",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "metadata.namespace",
							},
						},
					},
					{
						Name: "POD_IP",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "status.podIP",
							},
						},
					},
				},
				volumeMounts: []corev1.VolumeMount{
					{
						Name:      routerName + "-internal",
						MountPath: cfg.volumeMountPath + "/router-internal",
					},
					{
						Name:      routerName + "-amqps",
						MountPath: cfg.volumeMountPath + "/" + routerName + "-amqps",
					},
				},
				// resources: corev1.ResourceRequirements{
				// 	 Limits: corev1.ResourceList{
				// 	 	"cpu":    resource.MustParse("200m"),
				// 	 	"memory": resource.MustParse("1Gi"),
				// 	 },
				// 	 Requests: corev1.ResourceList{
				// 	 	"cpu":    resource.MustParse("50m"),
				// 	 	"memory": resource.MustParse("200Mi"),
				// 	 },
				// },
			},
		},
	}
}

func getTrafficPolicy(serviceType string) string {
	if strings.EqualFold(serviceType, string(corev1.ServiceTypeLoadBalancer)) {
		return string(corev1.ServiceExternalTrafficPolicyTypeLocal)
	}

	return ""
}
