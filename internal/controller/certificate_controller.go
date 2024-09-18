package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	certsv1 "certificate-controller/api/v1"
	"certificate-controller/internal/certs"
)

var (
	// Custom metric.
	// Define a counter metric to track the number of certificates in Available/Degraded states.
	certStatus = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "certificate_status",
			Help: "Tracks status related to certificate resources available and degraded.",
		},
		[]string{"status", "name", "namespace"},
	)

	// Custom metric.
	// Define a counter metric to track creation/deletion events related to secrets.
	secretEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "secret_events",
			Help: "Tracks events related to secret creation and deletion.",
		},
		[]string{"event", "name", "namespace"},
	)
)

// CertificateReconciler reconciles a Certificate object
type CertificateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(certStatus, secretEvents)
}

// +kubebuilder:rbac:groups=certs.k8c.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certs.k8c.io,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certs.k8c.io,resources=certificates/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

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

	// Fetch secret
	err = r.getSecret(ctx, secretObj, req.Namespace, certObj.Spec.SecretRef.Name)

	switch {
	// The below case is for scenarios wherein secret is not Present.
	// Secret would be missing for new creation flow or update flow(i.e., update to ".spec.secretRef.name")
	case errors.IsNotFound(err):
		log.V(1).Info("secret not found. Creating...")
		// Checks for old secret and delete in case of update flow(i.e., update to ".spec.secretRef.name")
		if certObj.Status.Condition != nil && certObj.Status.Condition.Type == certsv1.CertificateTypeAvailable {

			secretObj := &corev1.Secret{}
			// Fetch old secret. If true delete else proceed as we avoid making multiple attempts
			// to fetch old secret and delete.
			err := r.getSecret(ctx, secretObj, req.Namespace, *certObj.Status.SecretName)
			if errors.IsNotFound(err) {
				log.V(1).Info("old secret not found. Proceeding..")
			} else if err != nil {
				log.Error(err, "unable to fetch secret")
				log.V(1).Info("Proceeding..")
			} else {
				log.V(1).Info("deleting old secret", "secret", secretObj.Name)
				err := r.deleteResource(ctx, secretObj)
				if err != nil {
					// Proceed if unable to delete secret
					log.Error(err, "unable to delete old secret..Proceeding")
				} else {
					r.Recorder.Event(certObj, certsv1.EventTypeNormal, certsv1.EventReasonDeleted, fmt.Sprintf("deleted secrets/%v", *certObj.Status.SecretName))
					// Incrementing the secretEvents counter since resource is deleted.
					secretEvents.WithLabelValues(certsv1.EventReasonDeleted, secretObj.Name, secretObj.Namespace).Inc()
				}
			}
			// Reset .status.condition to make it go through the creation flow.
			certObj.Status.Condition = &metav1.Condition{}
		}
	// The below case is for scenarios when secret is not retrievable due to etcd slowness, API server timeouts etc
	case err != nil:
		log.Error(err, "could not fetch secret")
		return ctrl.Result{}, fmt.Errorf("could not fetch secret: %+v", err)
	// The below case is for scenario when secret exists.
	default:
		log.V(1).Info("secret found", "secret", secretObj.Name)
		// Deletion flow
		if !certObj.DeletionTimestamp.IsZero() {
			log.Info("deleting secret resource")
			if err := r.deleteResource(ctx, secretObj); err != nil {
				log.Error(err, "unable to delete secret resource")
				return ctrl.Result{}, fmt.Errorf("unable to delete secret resource: %+v", err)
			}
			r.Recorder.Event(certObj, certsv1.EventTypeNormal, certsv1.EventReasonDeleted, fmt.Sprintf("deleted secrets/%v", certObj.Spec.SecretRef.Name))
			secretEvents.WithLabelValues(certsv1.EventReasonDeleted, secretObj.Name, secretObj.Namespace).Inc()
			if controllerutil.ContainsFinalizer(certObj, finalizer) {
				controllerutil.RemoveFinalizer(certObj, finalizer)
			}
			log.V(1).Info("removing finalizer")
			if err := r.Update(ctx, certObj); err != nil {
				log.Error(err, "unable to remove finalizer")
				return ctrl.Result{}, fmt.Errorf("unable to remove finalizer: %+v", err)
			}
			return ctrl.Result{}, nil
		} else {
			// Validate contents of secret w.r.t .spec of CR i.e., actual vs desired
			match, err := validateResource(ctx, secretObj, certObj)
			if err != nil {
				return ctrl.Result{}, err
			}
			// Secret contents matches .spec of CR
			if match {
				log.V(0).Info("secret contents match .spec of certificates CR")
				// Update status to available if not set else skip.
				if (certObj.Status.Condition != nil) && (certObj.Status.Condition.Type != certsv1.CertificateTypeAvailable) {
					if err := r.updateCertStatus(ctx, certObj, certsv1.CertificateTypeAvailable, certsv1.CertificateReasonAvailable, "created secret"); err != nil {
						log.Error(err, "unable to update status type Available")
						return ctrl.Result{}, fmt.Errorf("unable to update status type Available: %+v", err)
					}
					// Emitting event since resource is created and .status.condition.type is updated to Available.
					r.Recorder.Event(certObj, certsv1.EventTypeNormal, certsv1.EventReasonCreated, fmt.Sprintf("created secrets/%v", secretObj.Name))
					// Incrementing counter of certStatus for .status.condition.type Available.
					certStatus.WithLabelValues(certsv1.CertificateTypeAvailable, certObj.Name, certObj.Namespace).Inc()
				}
				return ctrl.Result{}, nil
			} else {
				// Secret contents does not match .spec of CR
				// Update secret
				log.V(1).Info("secret contents does not match spec of certificates CR")
				if err := r.updateCertStatus(ctx, certObj, certsv1.CertificateTypeProgressing, certsv1.CertificateReasonProgressing, "creating secret"); err != nil {
					log.Error(err, "could not update progress status")
					return ctrl.Result{}, fmt.Errorf("could not update progress status: %+v", err)
				}
				// Emitting event since resource is being updated and .status.condition.type is updated to Progressing.
				r.Recorder.Event(certObj, certsv1.EventTypeNormal, certsv1.EventReasonCreating, fmt.Sprintf("creating secrets/%v", certObj.Spec.SecretRef.Name))
				log.V(1).Info("updating secret")
				if err := r.updateResource(ctx, secretObj, certObj); err != nil {
					log.Error(err, "unable to update resource")
					return ctrl.Result{}, fmt.Errorf("unable to update resource: %+v", err)
				}

				return ctrl.Result{}, nil
			}
		}
	}
	// Initialize .status.condition to make it go through the creation flow.
	if certObj.Status.Condition == nil {
		certObj.Status.Condition = &metav1.Condition{}
	}

	// Creation flow
	// Updates status to InProgress and checks/remove if old finalizer is present(in case of Update)
	// Attempts to create Resource. If failed, starts another Reconcile loop.
	if certObj.Status.Condition.Type == "" {
		if err := r.updateCertStatus(ctx, certObj, certsv1.CertificateTypeProgressing, certsv1.CertificateReasonProgressing, "creating secret"); err != nil {
			log.Error(err, "unable to update status type InProgress")
			return ctrl.Result{}, fmt.Errorf("unable to update status type InProgress: %+v", err)
		}
		// Emitting event since resource is being updated.
		r.Recorder.Event(certObj, certsv1.EventTypeNormal, certsv1.EventReasonCreating, fmt.Sprintf("creating secrets/%v", certObj.Spec.SecretRef.Name))

		// Removes old finalizer if present
		if certObj.Status.SecretName != nil {
			if controllerutil.ContainsFinalizer(certObj, fmt.Sprintf("secrets/%v", *certObj.Status.SecretName)) {
				controllerutil.RemoveFinalizer(certObj, fmt.Sprintf("secrets/%v", *certObj.Status.SecretName))
			}
		}
		// Adds finalizer
		if !controllerutil.ContainsFinalizer(certObj, finalizer) {
			controllerutil.AddFinalizer(certObj, finalizer)
		}
		log.V(1).Info("adding finalizer")
		if err := r.Update(ctx, certObj); err != nil {
			log.Error(err, "unable to add finalizer")
			return ctrl.Result{}, fmt.Errorf("unable to add finalizer: %+v", err)
		}

		log.V(1).Info("creating resource")
		if err := r.createResource(ctx, certObj); err != nil {

			log.Error(err, "unable to create resource")
			return ctrl.Result{}, fmt.Errorf("unable to create resource: %+v", err)
		}
		// Since resource creation i.e., secret is successful, increment the counter secretEvents.
		secretEvents.WithLabelValues(certsv1.EventReasonCreated, certObj.Spec.SecretRef.Name, certObj.Namespace).Inc()

	} else if (certObj.Status.Condition.Type == certsv1.CertificateTypeProgressing) || (certObj.Status.Condition.Type == certsv1.CertificateTypeDegraded) {
		// Creation flow second attempt before marking status Degraded.
		// Attempts to create Resource. If failed marks status as Degraded.
		// Subsequent Reconcile loop with status Degraded will go through below block.
		log.V(1).Info("creating resource")
		err := r.createResource(ctx, certObj)
		if !errors.IsNotFound(err) {
			log.V(1).Info("secret exists")
			// Since secret exists, reconcile so that in next loop it gets marked as Available.
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "unable to create resource")
			if certObj.Status.Condition.Type != certsv1.CertificateTypeDegraded {
				if err := r.updateCertStatus(ctx, certObj, certsv1.CertificateTypeDegraded, certsv1.CertificateReasonDegraded, err.Error()); err != nil {
					log.Error(err, "unable to update status type Degraded")
					return ctrl.Result{}, fmt.Errorf("unable to update status type Degraded: %+v", err)
				}
				r.Recorder.Event(certObj, certsv1.EvenTypeWarning, certsv1.EventReasonFailed, fmt.Sprintf("failed creating secrets/%v", certObj.Spec.SecretRef.Name))
			}
			certStatus.WithLabelValues(certsv1.CertificateTypeDegraded, certObj.Name, certObj.Namespace).Inc()
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("unable to create resource: %+v", err)
		}
		// Since resource creation i.e., secret is successful, increment the counter secretEvents.
		secretEvents.WithLabelValues(certsv1.EventReasonCreated, certObj.Spec.SecretRef.Name, certObj.Namespace).Inc()
	}

	return ctrl.Result{}, nil

}

func (r *CertificateReconciler) updateCertStatus(ctx context.Context, certObj *certsv1.Certificate, conditionType string, reason, message string) error {
	log := log.FromContext(ctx)
	certObj.Status.Condition.LastTransitionTime = metav1.Now()
	certObj.Status.Condition.Type = conditionType
	certObj.Status.Condition.Status = metav1.ConditionTrue
	certObj.Status.Condition.Reason = reason
	certObj.Status.Condition.Message = message
	if certObj.Status.Condition.Type == certsv1.CertificateTypeAvailable {
		certObj.Status.SecretName = &certObj.Spec.SecretRef.Name
	}
	log.V(1).Info("updating status", "status", certObj.Status)
	if err := r.Status().Update(ctx, certObj); err != nil {
		return err
	}
	log.V(1).Info("status updated successfully")
	return nil
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
	log.V(1).Info("created secret successfully", "secret", secretObj.Name)
	return nil
}

// Generates a Self Signed Certificate and Private key based on .spec.dnsName and .spec.validity of CR
func createSelfSignedCert(ctx context.Context, certObj *certsv1.Certificate) ([]byte, []byte, error) {
	ssCertObj := &certs.SelfSignedCert{
		Domain:   certObj.Spec.DNSName,
		Validity: certObj.Spec.Validity,
	}
	certPEM, keyPEM, err := ssCertObj.Create(ctx, certs.PrivateKeyBitSize)
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
			Annotations: map[string]string{
				"certs.k8c.io/managed-by": certObj.Name,
			},
			Labels: map[string]string{
				"certs.k8c.io/name": certObj.Spec.SecretRef.Name,
			},
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
	ssCert := &certs.SelfSignedCert{}
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
	log := log.FromContext(ctx)
	if err := r.Delete(ctx, secretObj); err != nil {
		return err
	}
	log.V(1).Info("deleted secret successfully", "secret", secretObj.Name)
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
	log.V(1).Info("successfully updated secret", "secret", secretObj.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certsv1.Certificate{}).
		Complete(r)
}
