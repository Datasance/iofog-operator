package controllers

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	iofogclient "github.com/datasance/iofog-go-sdk/v3/pkg/client"
	k8sclient "github.com/datasance/iofog-go-sdk/v3/pkg/k8s"
	op "github.com/datasance/iofog-go-sdk/v3/pkg/k8s/operator"
	cpv3 "github.com/datasance/iofog-operator/v3/apis/controlplanes/v3"
	"github.com/datasance/iofog-operator/v3/controllers/controlplanes/router"
	"github.com/datasance/iofog-operator/v3/internal/util"
	"github.com/skupperproject/skupper/pkg/certs"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	loadBalancerTimeout       = 360
	errProxyRouterMissing     = "missing Proxy.Router data for non LoadBalancer Router service"
	errParseControllerURL     = "failed to parse Controller endpoint as URL (%s): %s"
	portManagerDeploymentName = "port-manager"
)

func reconcileRoutine(ctx context.Context, recon func(context.Context) op.Reconciliation, reconChan chan op.Reconciliation) {
	reconChan <- recon(ctx)
}

func (r *ControlPlaneReconciler) reconcileDBCredentialsSecret(ctx context.Context, ms *microservice) (shouldRestartPod bool, err error) {
	for i := range ms.secrets {
		secret := &ms.secrets[i]

		if secret.Name == controllerDBCredentialsSecretName {
			found := &corev1.Secret{}

			err := r.Client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					return false, err
				}
				// Create secret
				err = r.Client.Create(ctx, secret)
				if err != nil {
					return false, err
				}

				return false, nil
			}
			// Secret already exists
			// Update secret
			err = r.Client.Update(ctx, secret)
			if err != nil {
				return false, err
			}
			// Restart pod
			return true, nil
		}
	}

	return false, nil
}

func (r *ControlPlaneReconciler) reconcileIofogController(ctx context.Context) op.Reconciliation {
	// Configure Controller
	config := &controllerMicroserviceConfig{
		replicas:           r.cp.Spec.Replicas.Controller,
		image:              r.cp.Spec.Images.Controller,
		imagePullSecret:    r.cp.Spec.Images.PullSecret,
		proxyImage:         r.cp.Spec.Images.Proxy,
		routerImage:        r.cp.Spec.Images.Router,
		db:                 &r.cp.Spec.Database,
		auth:               &r.cp.Spec.Auth,
		serviceType:        r.cp.Spec.Services.Controller.Type,
		serviceAnnotations: r.cp.Spec.Services.Controller.Annotations,
		loadBalancerAddr:   r.cp.Spec.Services.Controller.Address,
		https:              r.cp.Spec.Controller.Https,
		secretName:         r.cp.Spec.Controller.SecretName,
		ecn:                r.cp.Spec.Controller.ECNName,
		pidBaseDir:         r.cp.Spec.Controller.PidBaseDir,
		ecnViewerPort:      r.cp.Spec.Controller.EcnViewerPort,
		ecnViewerURL:       r.cp.Spec.Controller.EcnViewerURL,
	}

	ingressConfig := &controllerIngressConfig{
		annotations:      r.cp.Spec.Ingresses.Controller.Annotations,
		ingressClassName: r.cp.Spec.Ingresses.Controller.IngressClassName,
		host:             r.cp.Spec.Ingresses.Controller.Host,
		secretName:       r.cp.Spec.Ingresses.Controller.SecretName,
	}

	// get scheme for controller endpoint
	var scheme string
	if config.https == nil || *config.https == false {
		scheme = "http"
	} else {
		scheme = "https"
	}

	if config.db.Provider != "" {
		// Create controller database
		r.log.Info(fmt.Sprintf("Creating Controller Database %s", config.db.DatabaseName))

		// Create the database and wait for completion
		if err := util.CreateControllerDatabase(config.db.Host, config.db.User, config.db.Password, config.db.Provider, config.db.DatabaseName, config.db.Port); err != nil {
			r.log.Error(err, "Failed to create controller database")
			return op.ReconcileWithError(err)
		}
	}

	// Create Controller Microservice
	ms := newControllerMicroservice(r.cp.Namespace, config)

	// Service Account
	if err := r.createServiceAccount(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Handle DB credentials secret
	shouldRestartPods, err := r.reconcileDBCredentialsSecret(ctx, ms)
	if err != nil {
		return op.ReconcileWithError(err)
	}
	// Create secrets
	r.log.Info(fmt.Sprintf("Creating secrets for controller reconcile for Controlplane %s", r.cp.Name))

	if err := r.createSecrets(ctx, ms); err != nil {
		r.log.Info(fmt.Sprintf("Failed to create secrets %v for controller reconcile for Controlplane %s", err, r.cp.Name))

		return op.ReconcileWithError(err)
	}

	// Service
	if err := r.createService(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Ingress
	if strings.EqualFold(r.cp.Spec.Services.Controller.Type, string(corev1.ServiceTypeClusterIP)) {
		if err := r.createIngress(ctx, ingressConfig); err != nil {
			return op.ReconcileWithError(err)
		}
	}

	// PVC
	if err := r.createPersistentVolumeClaims(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	alreadyExists, err := r.deploymentExists(ctx, r.cp.Namespace, ms.name)
	if err != nil {
		return op.ReconcileWithError(err)
	}

	// Deployment
	if err := r.createDeployment(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// The deployment was just created, requeue to hide latency
	if !alreadyExists {
		return op.ReconcileWithRequeue(time.Second * 5) //nolint:gomnd
	}
	// Instantiate Controller client
	ctrlPort, err := getControllerPort(ms)
	if err != nil {
		return op.ReconcileWithError(err)
	}

	host := fmt.Sprintf("%s.%s.svc.cluster.local", ms.name, r.cp.ObjectMeta.Namespace)
	iofogClient, fin := r.getIofogClient(scheme, host, ctrlPort)

	if fin.IsFinal() {
		return fin
	}
	// Set up user
	if err := r.loginIofogClient(iofogClient); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "invalid credentials") {
			r.log.Info(fmt.Sprintf("Could not login to ControlPlane %s: %s", r.cp.Name, err.Error()))
			return op.ReconcileWithError(err)
		}
	}

	// Connect to cluster
	k8sClient, err := newK8sClient()
	if err != nil {
		return op.ReconcileWithError(err)
	}

	// Get Router or Router Proxy
	var routerProxy cpv3.RouterIngress

	if strings.EqualFold(r.cp.Spec.Services.Router.Type, string(corev1.ServiceTypeLoadBalancer)) {
		//nolint:contextcheck // k8sClient unfortunately does not accept context
		routerAddr, err := k8sClient.WaitForLoadBalancer(r.cp.Namespace, routerName, loadBalancerTimeout)
		if err != nil {
			return op.ReconcileWithError(err)
		}

		routerProxy = cpv3.RouterIngress{
			Address:      routerAddr,
			MessagePort:  router.MessagePort,
			InteriorPort: router.InteriorPort,
			EdgePort:     router.EdgePort,
		}
	} else if r.cp.Spec.Ingresses.Router.Address != "" {
		routerProxy = r.cp.Spec.Ingresses.Router
	} else {
		err := fmt.Errorf("reconcile Controller failed: %s", errProxyRouterMissing)

		return op.ReconcileWithError(err)
	}

	if err := r.createDefaultRouter(iofogClient, routerProxy); err != nil {
		return op.ReconcileWithError(err)
	}

	// Wait for Controller LB to actually work

	r.log.Info(fmt.Sprintf("Waiting for IP/LB Service in iofog-controller reconcile for ControlPlane %s", r.cp.Name))

	if strings.EqualFold(r.cp.Spec.Services.Controller.Type, string(corev1.ServiceTypeLoadBalancer)) {
		//nolint:contextcheck // k8sClient unfortunately does not accept context
		host, err := k8sClient.WaitForLoadBalancer(r.cp.Namespace, controllerName, loadBalancerTimeout)
		if err != nil {
			return op.ReconcileWithError(err)
		}
		// Check LB connection works
		if _, fin := r.getIofogClient(scheme, host, ctrlPort); fin.IsFinal() {
			r.log.Info(fmt.Sprintf("LB Connection works for ControlPlane %s", r.cp.Name))

			return fin
		}
	}

	if strings.EqualFold(r.cp.Spec.Services.Controller.Type, string(corev1.ServiceTypeClusterIP)) {
		// Retrieve the Ingress resource
		ingress := &networkingv1.Ingress{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: "pot-controller", Namespace: r.cp.Namespace}, ingress)
		if err != nil {
			return op.ReconcileWithError(fmt.Errorf("failed to get Ingress resource: %w", err))
		}

		// Check if LoadBalancer ingress exists
		if len(ingress.Status.LoadBalancer.Ingress) == 0 {
			return op.ReconcileWithError(fmt.Errorf("no LoadBalancer ingress found for Ingress resource"))
		}
	}

	if shouldRestartPods {
		r.log.Info(fmt.Sprintf("Restarting pods for ControlPlane %s", r.cp.Name))

		if err := r.restartPodsForDeployment(ctx, ms.name, r.cp.Namespace); err != nil {
			return op.ReconcileWithError(err)
		}
	}

	r.log.Info(fmt.Sprintf("op.Continue in iofog-controller reconcile for ControlPlane %s", r.cp.Name))

	return op.Continue()
}

func (r *ControlPlaneReconciler) getIofogClient(scheme string, host string, port int) (*iofogclient.Client, op.Reconciliation) {
	baseURL := fmt.Sprintf("%v://%s:%d/api/v3", scheme, host, port) //nolint:nosprintfhostport

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, op.ReconcileWithError(fmt.Errorf(errParseControllerURL, baseURL, err.Error()))
	}

	iofogClient := iofogclient.New(iofogclient.Options{
		BaseURL: parsedURL,
		Timeout: 1,
	})

	if _, err = iofogClient.GetStatus(); err != nil {
		r.log.Info(fmt.Sprintf("Could not get Controller status for ControlPlane %s: %s", r.cp.Name, err.Error()))

		return nil, op.ReconcileWithRequeue(time.Second * 3) //nolint:gomnd
	}

	return iofogClient, op.Continue()
}

func (r *ControlPlaneReconciler) reconcilePortManager(ctx context.Context) op.Reconciliation {
	ms := newPortManagerMicroservice(&portManagerConfig{
		image:              r.cp.Spec.Images.PortManager,
		imagePullSecret:    r.cp.Spec.Images.PullSecret,
		proxyImage:         r.cp.Spec.Images.Proxy,
		https:              r.cp.Spec.Controller.Https,
		serviceAnnotations: r.cp.Spec.Services.Proxy.Annotations,
		routerServerName:   r.cp.Spec.Proxy.ServerName,
		routerTransport:    r.cp.Spec.Proxy.Transport,
		httpProxyAddress:   r.cp.Spec.Ingresses.HTTPProxy.Address,
		tcpProxyAddress:    r.cp.Spec.Ingresses.TCPProxy.Address,
		watchNamespace:     r.cp.ObjectMeta.Namespace,
	})

	// Service Account
	if err := r.createServiceAccount(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}
	// Role
	if err := r.createRole(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}
	// RoleBinding
	if err := r.createRoleBinding(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Create secrets
	r.log.Info(fmt.Sprintf("Creating secrets for port-manager reconcile for Controlplane %s", r.cp.Name))

	if err := r.createSecrets(ctx, ms); err != nil {
		r.log.Info(fmt.Sprintf("Failed to create secrets %v for port-manager reconcile for Controlplane %s", err, r.cp.Name))

		return op.ReconcileWithError(err)
	}

	// Deployment
	if err := r.createDeployment(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	return op.Continue()
}

func (r *ControlPlaneReconciler) reconcileRouter(ctx context.Context) op.Reconciliation {
	// Configure
	volumeMountPath := "/etc/skupper-router/qpid-dispatch-certs/"
	secretWithCa := new(bool)
	*secretWithCa = true

	// if internalSecret and amqpsSecret are provided check if ca.crt present
	if r.cp.Spec.Router.InternalSecret != "" && r.cp.Spec.Router.AmqpsSecret != "" {
		// Check for ca.crt in the provided secrets
		if err := r.checkSecretsForCaCert(r.cp.Spec.Router.InternalSecret, r.cp.Spec.Router.AmqpsSecret, secretWithCa); err != nil {
			return op.ReconcileWithError(err)
		}
	}

	ms := newRouterMicroservice(routerMicroserviceConfig{
		image:              r.cp.Spec.Images.Router,
		imagePullSecret:    r.cp.Spec.Images.PullSecret,
		internalSecret:     r.cp.Spec.Router.InternalSecret,
		amqpsSecret:        r.cp.Spec.Router.AmqpsSecret,
		requireSsl:         r.cp.Spec.Router.RequireSsl,
		saslMechanisms:     r.cp.Spec.Router.SaslMechanisms,
		authenticatePeer:   r.cp.Spec.Router.AuthenticatePeer,
		serviceType:        r.cp.Spec.Services.Router.Type,
		serviceAnnotations: r.cp.Spec.Services.Router.Annotations,
		volumeMountPath:    volumeMountPath,
		secretWithCa:       secretWithCa,
	})

	// Service Account
	if err := r.createServiceAccount(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Role
	if err := r.createRole(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Role binding
	if err := r.createRoleBinding(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Service
	if err := r.createService(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	// Wait for IP
	k8sClient, err := newK8sClient()
	if err != nil {
		return op.ReconcileWithError(err)
	}

	// Wait for external IP of LB Service

	r.log.Info(fmt.Sprintf("Waiting for IP/LB Service in router reconcile for ControlPlane %s", r.cp.Name))

	var address string

	if strings.EqualFold(r.cp.Spec.Services.Router.Type, string(corev1.ServiceTypeLoadBalancer)) {
		//nolint:contextcheck // k8sClient unfortunately does not accept context
		address, err = k8sClient.WaitForLoadBalancer(r.cp.ObjectMeta.Namespace, ms.name, loadBalancerTimeout)
		if err != nil {
			return op.ReconcileWithError(err)
		}
	} else if r.cp.Spec.Ingresses.Router.Address != "" {
		address = r.cp.Spec.Ingresses.Router.Address
	} else {
		err = fmt.Errorf("reconcile Router failed: %s", errProxyRouterMissing)

		return op.ReconcileWithError(err)
	}

	r.log.Info(fmt.Sprintf("Found address %s for router reconcile for Controlplane %s", address, r.cp.Name))

	// Skip createRouterSecrets if internalSecret and amqpsSecret are provided
	if r.cp.Spec.Router.InternalSecret == "" && r.cp.Spec.Router.AmqpsSecret == "" {
		// Proceed with creating router secrets
		if err := r.createRouterSecrets(ms, address); err != nil {
			return op.ReconcileWithError(err)
		}
	}

	// Create secrets
	r.log.Info(fmt.Sprintf("Creating secrets for router reconcile for Controlplane %s", r.cp.Name))

	if err := r.createSecrets(ctx, ms); err != nil {
		r.log.Info(fmt.Sprintf("Failed to create secrets %v for router reconcile for Controlplane %s", err, r.cp.Name))

		return op.ReconcileWithError(err)
	}

	// Deployment
	r.log.Info(fmt.Sprintf("Creating deployment for router reconcile for Controlplane %s", r.cp.Name))

	if err := r.createDeployment(ctx, ms); err != nil {
		r.log.Info(fmt.Sprintf("Failed to create deployment %v for router reconcile for Controlplane %s", err, r.cp.Name))

		return op.ReconcileWithError(err)
	}

	r.log.Info(fmt.Sprintf("op.Continue for router reconcile for Controlplane %s", r.cp.Name))

	return op.Continue()
}

func (r *ControlPlaneReconciler) checkSecretsForCaCert(internalSecretName, amqpsSecretName string, secretWithCa *bool) error {
	k8sClient, err := newK8sClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Check internalSecret for ca.crt
	if err := r.checkCaCertInSecret(k8sClient, internalSecretName, secretWithCa); err != nil {
		return err
	}

	// Check amqpsSecret for ca.crt
	if err := r.checkCaCertInSecret(k8sClient, amqpsSecretName, secretWithCa); err != nil {
		return err
	}

	return nil
}

func (r *ControlPlaneReconciler) checkCaCertInSecret(k8sClient *k8sclient.Client, secretName string, secretWithCa *bool) error {
	secret, err := k8sClient.CoreV1().Secrets(r.cp.ObjectMeta.Namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret %s: %v", secretName, err)
	}

	// Check if ca.crt exists in the secret
	if _, exists := secret.Data["ca.crt"]; exists {
		r.log.Info(fmt.Sprintf("Found ca.crt in secret %s", secretName))
		*secretWithCa = true
	} else {
		r.log.Info(fmt.Sprintf("ca.crt not found in secret %s", secretName))
		*secretWithCa = false
	}

	// Log updated secretWithCa
	r.log.Info(fmt.Sprintf("Updated secretWithCa value: %v", *secretWithCa))

	return nil
}

func (r *ControlPlaneReconciler) createRouterSecrets(ms *microservice, address string) (err error) {
	r.log.Info(fmt.Sprintf("Creating routerSecrets definition for router reconcile for Controlplane %s", r.cp.Name))

	defer func() {
		if recoverResult := recover(); recoverResult != nil {
			r.log.Info(fmt.Sprintf("Recover result %v for creating secrets for router reconcile for Controlplane %s", recoverResult, r.cp.Name))
			err = fmt.Errorf("createRouterSecrets failed: %v", recoverResult)
		}
	}()
	// CA

	r.log.Info(fmt.Sprintf("Generating CA Secret secrets for router reconcile for Controlplane %s", r.cp.Name))

	caName := "router-ca"
	caSecret := certs.GenerateCASecret(caName, caName)
	caSecret.ObjectMeta.Namespace = r.cp.ObjectMeta.Namespace
	ms.secrets = append(ms.secrets, caSecret)

	// AMQPS and Internal
	for _, suffix := range []string{"amqps", "internal"} {
		r.log.Info(fmt.Sprintf("Generating %s Secret secrets for router reconcile for Controlplane %s", suffix, r.cp.Name))
		secret := certs.GenerateSecret("router-"+suffix, address, address, &caSecret)
		secret.ObjectMeta.Namespace = r.cp.ObjectMeta.Namespace
		ms.secrets = append(ms.secrets, secret)
	}

	r.log.Info(fmt.Sprintf("Secrets generated for Controlplane %s", r.cp.Name))

	return err
}

func newK8sClient() (*k8sclient.Client, error) {
	kubeConf := os.Getenv("KUBECONFIG")
	if kubeConf == "" {
		return k8sclient.NewInCluster()
	}

	return k8sclient.New(kubeConf)
}
