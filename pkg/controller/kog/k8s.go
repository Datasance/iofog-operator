package kog

import (
	"context"
	"strings"
	"time"

	k8sv1alpha2 "github.com/eclipse-iofog/iofog-operator/pkg/apis/k8s/v1alpha2"
	iofogclient "github.com/eclipse-iofog/iofogctl/pkg/iofog/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileKog) createDeployment(kog *k8sv1alpha2.Kog, ms *microservice) error {
	dep := newDeployment(kog.ObjectMeta.Namespace, ms)
	// Set Kog instance as the owner and controller
	if err := controllerutil.SetControllerReference(kog, dep, r.scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - update it
	r.logger.Info("Updating existing Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
	if err = r.client.Update(context.TODO(), dep); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileKog) createService(kog *k8sv1alpha2.Kog, ms *microservice) error {
	svc := newService(kog.ObjectMeta.Namespace, ms)
	// Set Kog instance as the owner and controller
	if err := controllerutil.SetControllerReference(kog, svc, r.scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err = r.client.Create(context.TODO(), svc)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - don't requeue
	r.logger.Info("Skip reconcile: Service already exists", "Service.Namespace", found.Namespace, "Service.Name", found.Name)
	return nil
}

func (r *ReconcileKog) createServiceAccount(kog *k8sv1alpha2.Kog, ms *microservice) error {
	svcAcc := newServiceAccount(kog.ObjectMeta.Namespace, ms)

	// Set Kog instance as the owner and controller
	if err := controllerutil.SetControllerReference(kog, svcAcc, r.scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &corev1.ServiceAccount{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: svcAcc.Name, Namespace: svcAcc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("Creating a new Service Account", "ServiceAccount.Namespace", svcAcc.Namespace, "ServiceAccount.Name", svcAcc.Name)
		err = r.client.Create(context.TODO(), svcAcc)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - don't requeue
	r.logger.Info("Skip reconcile: Service Account already exists", "ServiceAccount.Namespace", found.Namespace, "ServiceAccount.Name", found.Name)
	return nil
}

func (r *ReconcileKog) createClusterRoleBinding(kog *k8sv1alpha2.Kog, ms *microservice) error {
	crb := newClusterRoleBinding(kog.ObjectMeta.Namespace, ms)

	// Set Kog instance as the owner and controller
	if err := controllerutil.SetControllerReference(kog, crb, r.scheme); err != nil {
		return err
	}

	// Check if this resource already exists
	found := &rbacv1.ClusterRoleBinding{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: crb.Name, Namespace: crb.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("Creating a new Cluster Role Binding", "ClusterRoleBinding.Namespace", crb.Namespace, "ClusterRoleBinding.Name", crb.Name)
		err = r.client.Create(context.TODO(), crb)
		if err != nil {
			return err
		}

		// Resource created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// Resource already exists - don't requeue
	r.logger.Info("Skip reconcile: Cluster Role Binding already exists", "ClusterRoleBinding.Namespace", found.Namespace, "ClusterRoleBinding.Name", found.Name)
	return nil
}

func (r *ReconcileKog) waitForControllerAPI() (err error) {
	iofogClient := iofogclient.New(r.apiEndpoint)

	connected := false
	iter := 0
	for !connected {
		// Time out
		if iter > 60 {
			err = errors.NewTimeoutError("Timed out waiting for Controller API", iter)
			return
		}
		// Check the status endpoint
		if _, err = iofogClient.GetStatus(); err != nil {
			// Retry if connection is refused, this is usually only necessary on K8s Controller
			if strings.Contains(err.Error(), "connection refused") {
				time.Sleep(time.Millisecond * 1000)
				iter = iter + 1
				continue
			}
			// Return the error otherwise
			return
		}
		// No error, connected
		connected = true
		continue
	}

	return
}

func (r *ReconcileKog) createIofogUser(user *k8sv1alpha2.IofogUser) (err error) {
	iofogClient := iofogclient.New(r.apiEndpoint)

	if err = iofogClient.CreateUser(iofogclient.User(*user)); err != nil {
		// If not error about account existing, fail
		if !strings.Contains(err.Error(), "already an account associated") {
			return err
		}
		// Try to log in
		if err = iofogClient.Login(iofogclient.LoginRequest{
			Email:    user.Email,
			Password: user.Password,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileKog) getKubeletToken(user *k8sv1alpha2.IofogUser) (token string, err error) {
	iofogClient := iofogclient.New(r.apiEndpoint)
	if err = iofogClient.Login(iofogclient.LoginRequest{
		Email:    user.Email,
		Password: user.Password,
	}); err != nil {
		return
	}
	token = iofogClient.GetAccessToken()
	return
}
