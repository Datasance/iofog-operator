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

package v3

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	cond "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	conditionReady     = "ready"
	conditionDeploying = "deploying"
	conditionUpdating  = "updating"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ControlPlaneSpec defines the desired state of ControlPlane.
type ControlPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	// Auth contains Keycloak Client Configuration of Controller and ECN Viewer
	Auth Auth `json:"auth"`
	// Database for ioFog Controller
	Database Database `json:"database"`
	// Ingresses allow Router and Port Manager to configure endpoint addresses correctly
	Ingresses Ingresses `json:"ingresses,omitempty"`
	// Services should be LoadBalancer unless Ingress is being configured
	Services Services `json:"services,omitempty"`
	// Replicas of ioFog Controller should be 1 unless an external DB is configured
	Replicas Replicas `json:"replicas,omitempty"`
	// Images specifies which containers to run for each component of the ControlPlane
	Images Images `json:"images,omitempty"`
	// Controller contains runtime configuration for ioFog Controller
	Controller Controller `json:"controller,omitempty"`
	// Router contains runtime configuration for ioFog Router
	Router Router `json:"router,omitempty"`
	// Proxy contains runtime configuration for ioFog Proxy
	Proxy Proxy `json:"proxy,omitempty"`
}

type Replicas struct {
	Controller int32 `json:"controller,omitempty"`
}

type Services struct {
	Controller Service `json:"controller,omitempty"`
	Router     Service `json:"router,omitempty"`
	Proxy      Service `json:"proxy,omitempty"`
}

type Service struct {
	Type        string            `json:"type,omitempty"`
	Address     string            `json:"address,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Images struct {
	PullSecret  string `json:"pullSecret,omitempty"`
	Controller  string `json:"controller,omitempty"`
	Router      string `json:"router,omitempty"`
	PortManager string `json:"portManager,omitempty"`
	Proxy       string `json:"proxy,omitempty"`
}

type Auth struct {
	URL              string `json:"url"`
	Realm            string `json:"realm"`
	SSL              string `json:"ssl"`
	RealmKey         string `json:"realmKey"`
	ControllerClient string `json:"controllerClient"`
	ControllerSecret string `json:"controllerSecret"`
	ViewerClient     string `json:"viewerClient"`
}

type Database struct {
	Provider     string `json:"provider"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	Password     string `json:"password"`
	DatabaseName string `json:"databaseName"`
}

type User struct {
	Name            string `json:"name"`
	Surname         string `json:"surname"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	SubscriptionKey string `json:"subscriptionKey"`
}

type RouterIngress struct {
	Address      string `json:"address,omitempty"`
	MessagePort  int    `json:"messagePort,omitempty"`
	InteriorPort int    `json:"interiorPort,omitempty"`
	EdgePort     int    `json:"edgePort,omitempty"`
}

type ControllerIngress struct {
	Annotations      map[string]string `json:"annotations,omitempty"`
	IngressClassName string            `json:"ingressClassName,omitempty"`
	Host             string            `json:"host,omitempty"`
	SecretName       string            `json:"secretName,omitempty"`
}

type Ingress struct {
	Address string `json:"address,omitempty"`
}

type Ingresses struct {
	Controller ControllerIngress `json:"controller,omitempty"`
	Router     RouterIngress     `json:"router,omitempty"`
	HTTPProxy  Ingress           `json:"httpProxy,omitempty"`
	TCPProxy   Ingress           `json:"tcpProxy,omitempty"`
}

type Controller struct {
	PidBaseDir    string `json:"pidBaseDir,omitempty"`
	EcnViewerPort int    `json:"ecnViewerPort,omitempty"`
	EcnViewerURL  string `json:"ecnViewerUrl,omitempty"`
	ECNName       string `json:"ecn,omitempty"`
	Https         *bool  `json:"https,omitempty"`
	SecretName    string `json:"secretName,omitempty"`
}

type Router struct {
	InternalSecret   string `json:"internalSecret,omitempty"`
	AmqpsSecret      string `json:"amqpsSecret,omitempty"`
	RequireSsl       string `json:"requireSsl,omitempty"`
	SaslMechanisms   string `json:"saslMechanisms,omitempty"`
	AuthenticatePeer string `json:"authenticatePeer,omitempty"`
}

type Proxy struct {
	ServerName string `json:"serverName,omitempty"`
	Transport  string `json:"transport,omitempty"`
}

// ControlPlaneStatus defines the observed state of ControlPlane.
type ControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// ControlPlane is the Schema for the controlplanes API.
type ControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlPlaneSpec   `json:"spec,omitempty"`
	Status ControlPlaneStatus `json:"status,omitempty"`
}

func (cp *ControlPlane) setCondition(conditionType string, log *logr.Logger) {
	now := metav1.NewTime(time.Now())
	// Clear all
	for idx := range cp.Status.Conditions {
		condition := &cp.Status.Conditions[idx]
		// Migration: all lower case, no spaces, no -
		condition.Reason = strings.ToLower(condition.Reason)
		condition.Reason = strings.Replace(condition.Reason, " ", "_", -1)
		condition.Reason = strings.Replace(condition.Reason, "-", "_", -1)

		if condition.Status == metav1.ConditionTrue {
			condition.Status = metav1.ConditionFalse
			condition.Reason = fmt.Sprintf("transition_to_%s", conditionType)
			condition.LastTransitionTime = now
			condition.ObservedGeneration = cp.ObjectMeta.Generation
		}
	}
	// Add / overwrite
	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionTrue,
		Reason:             "initial_status",
		LastTransitionTime: now,
		ObservedGeneration: cp.ObjectMeta.Generation,
	}

	if log != nil {
		log.Info(fmt.Sprintf("reconcileDeploying() ControlPlane %s setCondition %v -- Existing conditions %v", cp.Name, newCondition, cp.Status.Conditions))
	}

	cond.SetStatusCondition(&cp.Status.Conditions, newCondition)
}

func (cp *ControlPlane) SetConditionDeploying(log *logr.Logger) {
	cp.setCondition(conditionDeploying, log)
}

func (cp *ControlPlane) SetConditionReady(log *logr.Logger) {
	cp.setCondition(conditionReady, log)
}

func (cp *ControlPlane) SetConditionUpdating(log *logr.Logger) {
	cp.setCondition(conditionUpdating, log)
}

func (cp *ControlPlane) GetCondition() string {
	state := conditionDeploying

	for _, condition := range cp.Status.Conditions {
		if condition.Status == metav1.ConditionTrue {
			if condition.ObservedGeneration == cp.ObjectMeta.Generation {
				state = condition.Type
			} else {
				state = conditionUpdating
			}
			break
		}
	}

	return state
}

func (cp *ControlPlane) IsReady() bool {
	return cp.GetCondition() == conditionReady
}

func (cp *ControlPlane) IsDeploying() bool {
	return cp.GetCondition() == conditionDeploying
}

func (cp *ControlPlane) IsUpdating() bool {
	return cp.GetCondition() == conditionUpdating
}

// +kubebuilder:object:root=true

// ControlPlaneList contains a list of ControlPlane.
type ControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlPlane `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&ControlPlane{}, &ControlPlaneList{})
}
