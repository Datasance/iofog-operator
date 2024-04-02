package controllers

import (
	"context"
	"crypto/tls"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	iofogclient "github.com/datasance/iofog-go-sdk/v3/pkg/client"
	cpv3 "github.com/datasance/iofog-operator/v3/apis/controlplanes/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ControlPlaneReconciler) deploymentExists(ctx context.Context, namespace, name string) (bool, error) {
	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	dep := &appsv1.Deployment{}

	err := r.Client.Get(ctx, key, dep)
	if err == nil {
		return true, nil
	}

	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

func (r *ControlPlaneReconciler) restartPodsForDeployment(ctx context.Context, deploymentName, namespace string) error {
	// Check if this resource already exists
	found := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespace}, found); err != nil {
		return err
	}

	originValue := int32(1)
	if found.Spec.Replicas == nil {
		found.Spec.Replicas = &originValue
	}

	if err := r.Client.Update(ctx, found); err != nil {
		return err
	}

	return r.Client.Update(ctx, found)
}

func (r *ControlPlaneReconciler) createDeployment(ctx context.Context, ms *microservice) error {
	dep := newDeployment(r.cp.ObjectMeta.Namespace, ms)
	// Set ControlPlane instance as the owner and controller
	if err := controllerutil.SetControllerReference(&r.cp, dep, r.Scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &appsv1.Deployment{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)

		err = r.Client.Create(ctx, dep)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - update it
	r.log.Info("Updating existing Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

	if err := r.Client.Update(ctx, dep); err != nil {
		return err
	}

	return nil
}

func (r *ControlPlaneReconciler) createPersistentVolumeClaims(ctx context.Context, ms *microservice) error {
	for i := range ms.volumes {
		if ms.volumes[i].VolumeSource.PersistentVolumeClaim == nil {
			continue
		}

		storageSize, err := resource.ParseQuantity("1Gi")
		if err != nil {
			return err
		}

		pvc := corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"storage": storageSize,
					},
				},
			},
		}

		pvc.ObjectMeta.Name = ms.volumes[i].Name
		pvc.ObjectMeta.Namespace = r.cp.Namespace
		// Set ControlPlane instance as the owner and controller
		if err := controllerutil.SetControllerReference(&r.cp, &pvc, r.Scheme); err != nil {
			return err
		}

		// Check if this resource already exists
		found := &corev1.PersistentVolumeClaim{}

		err = r.Client.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, found)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Info("Creating a new PersistentVolumeClaim", "PersistentVolumeClaim.Namespace", pvc.Namespace, "PersistentVolumeClaim.Name", pvc.Name)

			err = r.Client.Create(ctx, &pvc)
			if err != nil {
				return err
			}

			// Resource created successfully - don't requeue
			continue
		} else if err != nil {
			return err
		}

		// Resource already exists - don't requeue
		r.log.Info("Skip reconcile: Secret already exists", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
	}

	return nil
}

func (r *ControlPlaneReconciler) createSecrets(ctx context.Context, ms *microservice) error {
	return r.createOrUpdateSecrets(ctx, ms, false)
}

func (r *ControlPlaneReconciler) createOrUpdateSecrets(ctx context.Context, ms *microservice, update bool) error {
	defer func() {
		if recoverResult := recover(); recoverResult != nil {
			r.log.Info(fmt.Sprintf("Recover result %v for creating secrets for Controlplane %s", recoverResult, r.cp.Name))
		}
	}()

	for i := range ms.secrets {
		secret := &ms.secrets[i]
		r.log.Info(fmt.Sprintf("Creating secret %s", secret.ObjectMeta.Name))
		// Set ControlPlane instance as the owner and controller
		r.log.Info(fmt.Sprintf("Setting owner reference for secret %s", secret.ObjectMeta.Name))

		if err := controllerutil.SetControllerReference(&r.cp, secret, r.Scheme); err != nil {
			r.log.Info(fmt.Sprintf("Failed to set owner reference for secret %s: %v", secret.ObjectMeta.Name, err))

			return err
		}

		// Check if this resource already exists
		r.log.Info(fmt.Sprintf("Checking if secret %s exists", secret.ObjectMeta.Name))

		found := &corev1.Secret{}

		err := r.Client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)
		r.log.Info(fmt.Sprintf("secret %s: Exists: %s Error: %v", secret.ObjectMeta.Name, found.Name, err))

		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Service.Name", secret.Name)

			err = r.Client.Create(ctx, secret)
			if err != nil {
				return err
			}

			// Resource created successfully - don't requeue
			continue
		} else if err != nil {
			r.log.Info(fmt.Sprintf("Failed with error %v for secret %s:", err, secret.ObjectMeta.Name))

			return err
		}

		// Resource already exists - don't requeue
		if update {
			r.log.Info("Updating secret...", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)

			err = r.Client.Update(ctx, secret)
			if err != nil {
				return err
			}
		} else {
			r.log.Info("Skip reconciliation: Secret already exists.", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
		}
	}

	r.log.Info(fmt.Sprintf("Done Creating secrets for router reconcile for Controlplane %s", r.cp.Name))

	return nil
}

func (r *ControlPlaneReconciler) createService(ctx context.Context, ms *microservice) error {
	svcs := newServices(r.cp.ObjectMeta.Namespace, ms)
	for _, svc := range svcs {
		// Set ControlPlane instance as the owner and controller
		if err := controllerutil.SetControllerReference(&r.cp, svc, r.Scheme); err != nil {
			return err
		}

		// Check if this resource already exists
		found := &corev1.Service{}

		err := r.Client.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)

			err = r.Client.Create(ctx, svc)
			if err != nil {
				return err
			}

			// Resource created successfully - don't requeue
			continue
		} else if err != nil {
			return err
		}

		// Resource already exists - don't requeue
		r.log.Info("Skip reconcile: Service already exists", "Service.Namespace", found.Namespace, "Service.Name", found.Name)
	}

	return nil
}

func (r *ControlPlaneReconciler) createServiceAccount(ctx context.Context, ms *microservice) error {
	svcAcc := newServiceAccount(r.cp.ObjectMeta.Namespace, ms)

	// Set image pull secret for the service account
	if ms.imagePullSecret != "" {
		secret := &corev1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: svcAcc.Namespace,
			Name:      ms.imagePullSecret,
		}, secret)

		if err != nil || secret.Type != corev1.SecretTypeDockerConfigJson {
			r.log.Error(err, "Failed to create a new Service Account with imagePullSecret",
				"ServiceAccount.Namespace", svcAcc.Namespace,
				"ServiceAccount.Name", svcAcc.Name,
				"pullSecret", ms.imagePullSecret)

			return err
		}

		svcAcc.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: ms.imagePullSecret},
		}
	}

	// Set ControlPlane instance as the owner and controller
	if err := controllerutil.SetControllerReference(&r.cp, svcAcc, r.Scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &corev1.ServiceAccount{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: svcAcc.Name, Namespace: svcAcc.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating a new Service Account", "ServiceAccount.Namespace", svcAcc.Namespace, "ServiceAccount.Name", svcAcc.Name)
		// TODO: Find out why the IsAlreadyExists() check is necessary here. Happens when CP redeployed
		if err = r.Client.Create(ctx, svcAcc); err != nil && !k8serrors.IsAlreadyExists(err) {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - don't requeue
	r.log.Info("Skip reconcile: Service Account already exists", "ServiceAccount.Namespace", found.Namespace, "ServiceAccount.Name", found.Name)

	return nil
}

func (r *ControlPlaneReconciler) createRole(ctx context.Context, ms *microservice) error { //nolint:dupl
	role := newRole(r.cp.ObjectMeta.Namespace, ms)

	// Set ControlPlane instance as the owner and controller
	if err := controllerutil.SetControllerReference(&r.cp, role, r.Scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &rbacv1.Role{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating a new Role ", "Role.Namespace", role.Namespace, "Role.Name", role.Name)

		err = r.Client.Create(ctx, role)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - don't requeue
	r.log.Info("Skip reconcile: Role already exists", "Role.Namespace", found.Namespace, "Role.Name", found.Name)

	return nil
}

func (r *ControlPlaneReconciler) createRoleBinding(ctx context.Context, ms *microservice) error { //nolint:dupl
	crb := newRoleBinding(r.cp.ObjectMeta.Namespace, ms)

	// Set ControlPlane instance as the owner and controller
	if err := controllerutil.SetControllerReference(&r.cp, crb, r.Scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &rbacv1.RoleBinding{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: crb.Name, Namespace: crb.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating a new Role Binding", "RoleBinding.Namespace", crb.Namespace, "RoleBinding.Name", crb.Name)

		err = r.Client.Create(ctx, crb)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - don't requeue
	r.log.Info("Skip reconcile: Role Binding already exists", "RoleBinding.Namespace", found.Namespace, "RoleBinding.Name", found.Name)

	return nil
}

func (r *ControlPlaneReconciler) loginIofogClient(iofogClient *iofogclient.Client) error {
	authURL := r.cp.Spec.Auth.URL
	realm := r.cp.Spec.Auth.Realm
	clientID := r.cp.Spec.Auth.ControllerClient
	clientSecret := r.cp.Spec.Auth.ControllerSecret

	type LoginResponse struct {
		AccessToken string `json:"access_token"`
	}

	r.log.Info("Generating Client Access Token")
	// Construct the URL for token request
	url := fmt.Sprintf("%srealms/%s/protocol/openid-connect/token", authURL, realm)
	method := "POST"
	payload := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s", clientID, clientSecret)

	// Create HTTP client with custom transport to skip certificate verification
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	// Create request
	req, err := http.NewRequest(method, url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Check response status
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	// Read response body
	var response LoginResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return err
	}

	// Assign access token
	iofogClient.SetAccessToken(response.AccessToken)
	return nil
}

func newInt(val int) *int {
	return &val
}

func (r *ControlPlaneReconciler) createDefaultRouter(iofogClient *iofogclient.Client, proxy cpv3.RouterIngress) (err error) {
	routerConfig := iofogclient.Router{
		Host: proxy.Address,
		RouterConfig: iofogclient.RouterConfig{
			InterRouterPort: newInt(proxy.InteriorPort),
			EdgeRouterPort:  newInt(proxy.EdgePort),
			MessagingPort:   newInt(proxy.MessagePort),
		},
	}

	return iofogClient.PutDefaultRouter(routerConfig)
}

func DecodeBase64(encoded string) (string, error) {
	decodedBytes, err := b64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	return string(decodedBytes), nil
}
