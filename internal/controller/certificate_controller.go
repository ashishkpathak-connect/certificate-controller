package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	certsv1 "certificate-controller/api/v1"
	controller "certificate-controller/internal"
)

// CertificateReconciler reconciles a Certificate object
type CertificateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=certs.k8c.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certs.k8c.io,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certs.k8c.io,resources=certificates/finalizers,verbs=update
// +kubebuilder:rbac:groups=certs.k8c.io,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	certObj := &certsv1.Certificate{}

	err := r.Get(ctx, req.NamespacedName, certObj)

	switch {
	case errors.IsNotFound(err):
		return ctrl.Result{}, client.IgnoreNotFound(err)
	case err != nil:
		log.Error(err, "could not fetch certificate resource")
		return ctrl.Result{}, fmt.Errorf("could not fetch certificate resource: %+v", err)
	}
	finalizer := fmt.Sprintf("secrets/%v", certObj.Spec.SecretRef.Name)
	secretObj := &corev1.Secret{}

	err = r.Get(ctx, client.ObjectKey{
		Namespace: req.Namespace,
		Name:      certObj.Spec.SecretRef.Name,
	}, secretObj)

	switch {
	case errors.IsNotFound(err):
		log.Info("secret not found. Creating...")
		if certObj.Status.Condition != nil && certObj.Status.Condition.Type == certsv1.CertificateAvailable {

			secretObj := &corev1.Secret{}
			err := r.getSecret(ctx, secretObj, req.Namespace, *certObj.Status.SecretName)
			if err != nil {
				log.Error(err, "unable to fetch secret")
				return ctrl.Result{}, fmt.Errorf("unable to fetch secret: %+v", err)
			} else if errors.IsNotFound(err) {
				log.Info("old secret not found. Proceeding..")
				certObj.Status.Condition = &metav1.Condition{}
			} else {
				log.Info("deleting old secret", "secret", secretObj.Name)
				err := r.deleteResource(ctx, secretObj)
				if err != nil {
					log.Error(err, "unable to delete old secret")
				} else {
					log.Info("secret deleted", "secret", secretObj.Name)

					certObj.Status.Condition = &metav1.Condition{}
				}
			}
		}
	case err != nil:
		log.Error(err, "could not fetch secret")
		return ctrl.Result{}, fmt.Errorf("could not fetch secret: %+v", err)
	default:
		log.Info("secret found", "secret", secretObj.Name)
		// deletion flow
		if !certObj.DeletionTimestamp.IsZero() {
			log.Info("deleting secret resource")
			if err := r.deleteResource(ctx, secretObj); err != nil {
				log.Error(err, "unable to delete secret resource")
				return ctrl.Result{}, fmt.Errorf("unable to delete secret resource: %+v", err)
			}
			log.Info("secret Deleted", "secret", secretObj.Name)
			if controllerutil.ContainsFinalizer(certObj, finalizer) {
				controllerutil.RemoveFinalizer(certObj, finalizer)
			}
			r.Update(ctx, certObj)
			return ctrl.Result{}, nil
		} else {
			// validation flow
			match, err := validateResource(ctx, secretObj, certObj)
			if err != nil {
				return ctrl.Result{}, err
			}
			if match {
				if (certObj.Status.Condition != nil) && (certObj.Status.Condition.Type != certsv1.CertificateAvailable) {
					log.Info("setting status type Available")
					setCertStatusCondition(certObj, certsv1.CertificateAvailable, certsv1.ReasonAvailable, "created secret")
					if err := r.Status().Update(ctx, certObj); err != nil {
						log.Error(err, "could not update status")
						return ctrl.Result{}, fmt.Errorf("could not update status: %+v", err)
					}
				}
				return ctrl.Result{}, nil
			} else {
				log.Info("setting status type InProgress")
				setCertStatusCondition(certObj, certsv1.CertificateProgressing, certsv1.ReasonProgressing, "creating secret")
				if err := r.Status().Update(ctx, certObj); err != nil {
					log.Error(err, "could not update progress status")
					return ctrl.Result{}, fmt.Errorf("could not update progress status: %+v", err)
				}
				if err := r.updateResource(ctx, secretObj, certObj); err != nil {
					log.Error(err, "unable to update resource")
					return ctrl.Result{}, fmt.Errorf("unable to delete secret: %+v", err)
				}
				return ctrl.Result{}, nil
			}
		}
	}
	if certObj.Status.Condition == nil {
		certObj.Status.Condition = &metav1.Condition{}
	}

	if certObj.Status.Condition.Type == "" {
		log.Info("setting status type InProgress")
		setCertStatusCondition(certObj, certsv1.CertificateProgressing, certsv1.ReasonProgressing, "creating secret")
		if err := r.Status().Update(ctx, certObj); err != nil {
			log.Error(err, "could not update progress status")
			return ctrl.Result{}, fmt.Errorf("could not update progress status: %+v", err)
		}

		if certObj.Status.SecretName != nil {
			if controllerutil.ContainsFinalizer(certObj, fmt.Sprintf("secrets/%v", *certObj.Status.SecretName)) {
				controllerutil.RemoveFinalizer(certObj, fmt.Sprintf("secrets/%v", *certObj.Status.SecretName))
			}
		}

		if !controllerutil.ContainsFinalizer(certObj, finalizer) {
			controllerutil.AddFinalizer(certObj, finalizer)
		}
		if err := r.Update(ctx, certObj); err != nil {
			log.Error(err, "unable to update finalizer")
			return ctrl.Result{}, fmt.Errorf("unable to update finalizer: %+v", err)
		}
		if err := r.createResource(ctx, certObj); err != nil {
			log.Error(err, "unable to create resource")
			return ctrl.Result{}, fmt.Errorf("unable to create resource: %+v", err)
		}

	} else if (certObj.Status.Condition.Type == certsv1.CertificateProgressing) || (certObj.Status.Condition.Type == certsv1.CertificateDegraded) {
		err := r.createResource(ctx, certObj)
		if !errors.IsNotFound(err) {
			log.Info("secret exists")
			return ctrl.Result{}, fmt.Errorf("secret exists %+v", err)
		} else if err != nil {
			log.Error(err, "unable to create resource")
			if certObj.Status.Condition.Type != certsv1.CertificateDegraded {
				log.Info("setting status type Degraded")

				setCertStatusCondition(certObj, certsv1.CertificateDegraded, certsv1.ReasonDegraded, err.Error())
				if err := r.Status().Update(ctx, certObj); err != nil {
					log.Error(err, "could not update degrade  status")
					return ctrl.Result{}, fmt.Errorf("could not update degrade status: %+v", err)
				}
			}
			return ctrl.Result{}, fmt.Errorf("unable to create resource: %+v", err)
		}
	}

	return ctrl.Result{}, nil

}

func setCertStatusCondition(certObj *certsv1.Certificate, conditionType string, reason, message string) {

	certObj.Status.Condition.LastTransitionTime = metav1.Now()
	certObj.Status.Condition.Type = conditionType
	certObj.Status.Condition.Status = metav1.ConditionTrue
	certObj.Status.Condition.Reason = reason
	certObj.Status.Condition.Message = message
	if certObj.Status.Condition.Type == certsv1.CertificateAvailable {
		certObj.Status.SecretName = &certObj.Spec.SecretRef.Name
	}
}

func (r *CertificateReconciler) createResource(ctx context.Context, certObj *certsv1.Certificate) error {
	log := log.FromContext(ctx)
	certPEM, keyPEM, err := createSelfSignedCert(ctx, certObj)
	if err != nil {
		return err
	}
	secretObj := setSecret(certObj, certPEM, keyPEM)
	if err := r.Create(ctx, secretObj); err != nil {
		log.Error(err, "unable to create secret")
		return err
	}
	log.Info("successfully created secret", "secret", secretObj.Name)
	return nil
}

func createSelfSignedCert(ctx context.Context, certObj *certsv1.Certificate) ([]byte, []byte, error) {
	ssCert := &controller.SelfSignedCert{
		Domain:   certObj.Spec.DNSName,
		Validity: certObj.Spec.Validity,
	}
	certPEM, keyPEM, err := ssCert.Create(ctx, controller.PrivateKeyBitSize)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

func setSecret(certObj *certsv1.Certificate, certPEM, keyPEM []byte) *corev1.Secret {
	secretObj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certObj.Spec.SecretRef.Name,
			Namespace: certObj.Namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}
	return secretObj
}

func validateResource(ctx context.Context, secretObj *corev1.Secret, certObj *certsv1.Certificate) (bool, error) {
	// Read certificate
	ssCert := &controller.SelfSignedCert{}
	if err := ssCert.Read(ctx, secretObj); err != nil {
		return false, err
	}
	// validate actual state vs desired state
	if (ssCert.Domain == certObj.Spec.DNSName) && (ssCert.Validity == certObj.Spec.Validity) {
		return true, nil
	}
	return false, nil
}

func (r *CertificateReconciler) getSecret(ctx context.Context, secretObj *corev1.Secret, namespace, name string) error {
	err := r.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, secretObj)
	return err
}

func (r *CertificateReconciler) deleteResource(ctx context.Context, secretObj *corev1.Secret) error {
	if err := r.Delete(ctx, secretObj); err != nil {
		return err
	}
	return nil
}

func (r *CertificateReconciler) updateResource(ctx context.Context, secretObj *corev1.Secret, certObj *certsv1.Certificate) error {
	log := log.FromContext(ctx)
	certPEM, keyPEM, err := createSelfSignedCert(ctx, certObj)
	if err != nil {
		return err
	}

	secretObj.Data[corev1.TLSCertKey] = certPEM
	secretObj.Data[corev1.TLSPrivateKeyKey] = keyPEM
	if err := r.Update(ctx, secretObj); err != nil {
		log.Error(err, "unable to update secret")
		return err
	}
	log.Info("successfully updated secret", "secret", secretObj.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certsv1.Certificate{}).
		Complete(r)
}
