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
	openidutil "github.com/datasance/iofog-operator/v3/internal/util"
	util "github.com/datasance/iofog-operator/v3/internal/util/certs"

	// "github.com/skupperproject/skupper/pkg/certs"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	loadBalancerTimeout   = 360
	errProxyRouterMissing = "missing Proxy.Router data for non LoadBalancer Router service"
	errParseControllerURL = "failed to parse Controller endpoint as URL (%s): %s"
)

// getEventsIfConfigured returns a pointer to Events if it's configured (at least one field is set), otherwise nil
func getEventsIfConfigured(events cpv3.Events) *cpv3.Events {
	// Check if at least one field is configured
	if events.AuditEnabled != nil || events.CaptureIpAddress != nil || events.RetentionDays != 0 || events.CleanupInterval != 0 {
		return &events
	}
	return nil
}

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
		routerAdaptorImage: r.cp.Spec.Images.RouterAdaptor,
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
		logLevel:           r.cp.Spec.Controller.LogLevel,
		events:             getEventsIfConfigured(r.cp.Spec.Events),
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

	// Create Controller Microservice
	ms := newControllerMicroservice(r.cp.Namespace, config)

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

	// Import router certificates
	if recon := r.ImportCertificates(iofogClient); recon.IsFinal() {
		return recon
	}

	// Wait for Controller LB to actually work
	r.log.Info(fmt.Sprintf("Waiting for IP/LB Service in iofog-controller reconcile for ControlPlane %s", r.cp.Name))

	var viewerEndpoint string

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
		viewerEndpoint = fmt.Sprintf("%s://%s", scheme, host)
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

		if r.cp.Spec.Ingresses.Controller.Host != "" {
			viewerEndpoint = fmt.Sprintf("%s://%s", scheme, r.cp.Spec.Ingresses.Controller.Host)
		}
	}

	if shouldRestartPods {
		r.log.Info(fmt.Sprintf("Restarting pods for ControlPlane %s", r.cp.Name))

		if err := r.restartPodsForDeployment(ctx, ms.name, r.cp.Namespace); err != nil {
			return op.ReconcileWithError(err)
		}
	}

	// Update ECN Viewer Client Root URL
	if viewerEndpoint != "" {
		r.log.Info(fmt.Sprintf("Updating ECN Viewer Client Root URL for ControlPlane %s to %s", r.cp.Name, viewerEndpoint))
		if err := openidutil.UpdateECNViewerClientRootURL(r.cp.Spec.Auth, viewerEndpoint); err != nil {
			r.log.Info(fmt.Sprintf("Failed to update ECN Viewer Client Root URL for ControlPlane %s: %s", r.cp.Name, err.Error()))
			// Continue even if update fails, as it's not critical for the reconcile process
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

func (r *ControlPlaneReconciler) ImportCertificates(iofogClient *iofogclient.Client) op.Reconciliation {
	r.log.Info(fmt.Sprintf("Importing certificates for ControlPlane %s", r.cp.Name))
	if err := r.ImportRouterCACertificate(iofogClient, "pot-site-ca"); err != nil {
		r.log.Info(fmt.Sprintf("Failed to import certificates for ControlPlane %s: %s", r.cp.Name, err.Error()))
		return op.ReconcileWithRequeue(time.Second * 10)
	}

	if err := r.ImportRouterCACertificate(iofogClient, "default-router-local-ca"); err != nil {
		r.log.Info(fmt.Sprintf("Failed to import certificates for ControlPlane %s: %s", r.cp.Name, err.Error()))
		return op.ReconcileWithRequeue(time.Second * 10)
	}

	return op.Continue()
}

func (r *ControlPlaneReconciler) reconcileRouter(ctx context.Context) op.Reconciliation {
	// Configure
	volumeMountPath := "/etc/skupper-router-certs"

	// Check if HA is enabled (default to false if not specified)
	haEnabled := false
	// if r.cp.Spec.Router.HA != nil {
	// 	haEnabled = *r.cp.Spec.Router.HA
	// }

	routerMicroservices := newRouterMicroservices(routerMicroserviceConfig{
		image:              r.cp.Spec.Images.Router,
		adaptorImage:       r.cp.Spec.Images.RouterAdaptor,
		imagePullSecret:    r.cp.Spec.Images.PullSecret,
		serviceType:        r.cp.Spec.Services.Router.Type,
		serviceAnnotations: r.cp.Spec.Services.Router.Annotations,
		volumeMountPath:    volumeMountPath,
		ha:                 haEnabled,
	})

	// Use the primary router for service creation and IP resolution
	ms := routerMicroservices[0]

	// Service Account, Role, and Role Binding (only for primary router)
	if err := r.createServiceAccount(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

	if err := r.createRole(ctx, ms); err != nil {
		return op.ReconcileWithError(err)
	}

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

	if err := r.createRouterSecrets(r.cp.ObjectMeta.Namespace, ms, address); err != nil {
		return op.ReconcileWithError(err)
	}
	// Create secrets
	r.log.Info(fmt.Sprintf("Creating secrets for router reconcile for Controlplane %s", r.cp.Name))

	if err := r.createSecrets(ctx, ms); err != nil {
		r.log.Info(fmt.Sprintf("Failed to create secrets %v for router reconcile for Controlplane %s", err, r.cp.Name))

		return op.ReconcileWithError(err)
	}

	// Router ConfigMap
	r.log.Info(fmt.Sprintf("Creating configmap for router reconcile for Controlplane %s", r.cp.Name))

	if err := r.createConfigMap(ctx); err != nil {
		r.log.Info(fmt.Sprintf("Failed to create configmap %v for router reconcile for Controlplane %s", err, r.cp.Name))
		return op.ReconcileWithError(err)
	}

	// Deployments
	r.log.Info(fmt.Sprintf("Creating deployments for router reconcile for Controlplane %s", r.cp.Name))

	for _, routerMS := range routerMicroservices {
		if err := r.createDeployment(ctx, routerMS); err != nil {
			r.log.Info(fmt.Sprintf("Failed to create deployment %v for router reconcile for Controlplane %s", err, r.cp.Name))
			return op.ReconcileWithError(err)
		}
	}

	r.log.Info(fmt.Sprintf("op.Continue for router reconcile for Controlplane %s", r.cp.Name))

	return op.Continue()
}

// createRouterSecrets creates the secrets for the router.
// It generates the CA and secrets for the router.
// It also appends the secrets to the microservice.secrets slice.
func (r *ControlPlaneReconciler) createRouterSecrets(namespace string, ms *microservice, address string) (err error) {
	r.log.Info(fmt.Sprintf("Creating routerSecrets definition for router reconcile for Controlplane %s", r.cp.Name))

	defer func() {
		if recoverResult := recover(); recoverResult != nil {
			r.log.Info(fmt.Sprintf("Recover result %v for creating secrets for router reconcile for Controlplane %s", recoverResult, r.cp.Name))
			err = fmt.Errorf("createRouterSecrets failed: %v", recoverResult)
		}
	}()

	const (
		LocalClientSecret        string = "skupper-local-client"
		LocalServerSecret        string = "pot-router-local-server"
		LocalCaSecret            string = "default-router-local-ca"
		SiteServerSecret         string = "pot-router-site-server"
		SiteCaSecret             string = "pot-site-ca"
		ConsoleServerSecret      string = "skupper-console-certs"
		ConsoleUsersSecret       string = "skupper-console-users"
		PrometheusServerSecret   string = "skupper-prometheus-certs"
		OauthRouterConsoleSecret string = "skupper-router-console-certs"
		ServiceCaSecret          string = "skupper-service-ca"
		ServiceClientSecret      string = "skupper-service-client" // Secret that is used in sslProfiles for all http2 connectors with tls enabled
	)

	// Check if secrets already exist
	existingSiteCA := &corev1.Secret{}
	existingLocalCA := &corev1.Secret{}
	existingSiteServer := &corev1.Secret{}
	existingLocalServer := &corev1.Secret{}
	siteSecretAddress := fmt.Sprintf("%s.%s.svc.cluster.local,%s", ms.name, namespace, address)
	localSecretAddress := fmt.Sprintf("%s.%s.svc.cluster.local,%s", ms.name, namespace, address)
	siteSecretSubject := fmt.Sprintf("pot-router")
	localSecretSubject := fmt.Sprintf("pot-router-local")

	// Try to get existing secrets
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: SiteCaSecret, Namespace: namespace}, existingSiteCA)
	siteCAExists := err == nil
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: LocalCaSecret, Namespace: namespace}, existingLocalCA)
	localCAExists := err == nil
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: SiteServerSecret, Namespace: namespace}, existingSiteServer)
	siteServerExists := err == nil
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: LocalServerSecret, Namespace: namespace}, existingLocalServer)
	localServerExists := err == nil

	// If all secrets exist, use them
	if siteCAExists && localCAExists && siteServerExists && localServerExists {
		r.log.Info(fmt.Sprintf("Using existing secrets for Controlplane %s", r.cp.Name))
		ms.secrets = append(ms.secrets, *existingSiteServer, *existingLocalServer)
		return nil
	}

	// Generate new secrets if any are missing
	r.log.Info(fmt.Sprintf("Generating new secrets for Controlplane %s", r.cp.Name))

	var siteCA, localCA corev1.Secret

	// Generate CA certificates if they don't exist
	if !siteCAExists {
		r.log.Info(fmt.Sprintf("Generating site CA Secret for Controlplane %s", r.cp.Name))
		siteCA = util.GenerateSecret(SiteCaSecret, SiteCaSecret, SiteCaSecret, 0, nil)
		siteCA.Namespace = namespace
		ms.secrets = append(ms.secrets, siteCA)
	} else {
		siteCA = *existingSiteCA
	}

	if !localCAExists {
		r.log.Info(fmt.Sprintf("Generating local CA Secret for Controlplane %s", r.cp.Name))
		localCA = util.GenerateSecret(LocalCaSecret, LocalCaSecret, LocalCaSecret, 0, nil)
		localCA.Namespace = namespace
		ms.secrets = append(ms.secrets, localCA)
	} else {
		localCA = *existingLocalCA
	}

	// Generate server certificates if they don't exist
	if !siteServerExists {
		r.log.Info(fmt.Sprintf("Generating site server certificate for Controlplane %s with address %s", r.cp.Name, address))
		siteSecret := util.GenerateSecret(SiteServerSecret, siteSecretSubject, siteSecretAddress, 0, &siteCA)
		siteSecret.Namespace = namespace
		ms.secrets = append(ms.secrets, siteSecret)
	} else {
		ms.secrets = append(ms.secrets, *existingSiteServer)
	}

	if !localServerExists {
		r.log.Info(fmt.Sprintf("Generating local server certificate for Controlplane %s with address %s", r.cp.Name, address))
		localSecret := util.GenerateSecret(LocalServerSecret, localSecretSubject, localSecretAddress, 0, &localCA)
		localSecret.Namespace = namespace
		ms.secrets = append(ms.secrets, localSecret)
	} else {
		ms.secrets = append(ms.secrets, *existingLocalServer)
	}

	r.log.Info(fmt.Sprintf("Secrets generated/retrieved for Controlplane %s", r.cp.Name))

	return nil
}

func newK8sClient() (*k8sclient.Client, error) {
	kubeConf := os.Getenv("KUBECONFIG")
	if kubeConf == "" {
		return k8sclient.NewInCluster()
	}

	return k8sclient.New(kubeConf)
}
