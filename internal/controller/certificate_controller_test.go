package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	certsv1 "certificate-controller/api/v1"

	"certificate-controller/internal/certs"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Certificate Controller", func() {

	const (
		CertificateName      = "test-certificate"
		CertificateNamespace = "default"
		DNSName              = "test.k8c.io"
		Validity             = "30d"
		SecretName           = "test-secret"
		UpdatedDNSName       = "updated-test.k8c.io"
		UpdatedValidity      = "365d"
		UpdatedSecretName    = "updated-test-secret"
	)

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// Create test
	// Create a certificate CR and check if its created.
	Context("Create certificate CR successfully", func() {
		It("Should create certificate CR successfully", func() {
			By("Creating a certificate CR")

			certificate := &certsv1.Certificate{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "certs.k8c.io/v1",
					Kind:       "Certificate",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      CertificateName,
					Namespace: CertificateNamespace,
				},
				Spec: certsv1.CertificateSpec{
					DNSName:  DNSName,
					Validity: Validity,
					SecretRef: certsv1.SecretReference{
						Name: SecretName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, certificate)).Should(Succeed())

		})
	})

	// Create test.
	// Fetch the created certificate CR and check if its able to fetch.
	Context("Fetch an existing certificate CR successfully", func() {
		It("Should fetch an existing certificate CR successfully", func() {
			By("Fetching an existing certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

		})
	})

	// Create test.
	// Validate if the spec of created CR is as expected.
	Context("Check if certificate CR .spec is as expected", func() {
		It("Should validate certificate CR .spec", func() {
			By("Fetching certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Spec.DNSName
			}, "30s", "1s").Should(Equal(DNSName))

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Spec.Validity
			}, "30s", "1s").Should(Equal(Validity))

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Spec.SecretRef.Name
			}, "30s", "1s").Should(Equal(SecretName))
		})
	})

	// Create test.
	// Check if created certificate CR creates a secret too by fetching the secret.
	Context("Secret should have been successfully created by the certificate CR", func() {
		It("Should successfully be retrieved", func() {
			By("Fetching secret")

			typeNamespacedName := types.NamespacedName{
				Name:      SecretName,
				Namespace: CertificateNamespace,
			}

			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, secret)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

		})
	})

	// Create test.
	// Validate .status.condition.type of certificate CR is available.
	Context("Verify status.condition.type of existing certificate CR is Available", func() {
		It("Verify status.condition.type of certificate CR is Available", func() {
			By("Fetching an existing certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Status.Condition.Type
			}, "30s", "1s").Should(Equal(certsv1.CertificateTypeAvailable))

		})
	})

	// Update test
	// Update an existing certificate CR .spec.dnsName and .spec.validity field.
	// Check if update to certificate CR was successful.
	Context("Updating an existing certificate CR should .spec.dnsName and .spec.validity  be successful", func() {
		It("Should update certificate CR successfully", func() {
			By("Updating an existing certificate CR .spec.dnsName and .spec.validity")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			certificate.Spec.DNSName = UpdatedDNSName
			certificate.Spec.Validity = UpdatedValidity

			Expect(k8sClient.Update(ctx, certificate)).Should(Succeed())
		})
	})

	// Update test
	// Fetch existing certificate CR.
	// Check if .spec.dnsName and .spec.validity field reflects updated values.
	Context("Check if certificate CR .spec.dnsName and .spec.validity are updated successfully", func() {
		It("Should check certificate CR .spec.dnsName and .spec.validity are updated", func() {
			By("Fetching certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Spec.DNSName
			}, "30s", "1s").Should(Equal(UpdatedDNSName))

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Spec.Validity
			}, "30s", "1s").Should(Equal(UpdatedValidity))
		})
	})

	// Update test
	// Fetch secret to validate if the secret is updated.
	// The existing secret would have been updated as
	// .spec.secretRef.name of certificate CR did not change.
	Context("Secret should be updated as per .spec.dnsName and .spec.validity of certificate CR", func() {
		It("Should check if secret contents is as per .spec.dnsName and .spec.validity of certificate CR", func() {
			By("Fetching secret")

			typeCertificateNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeCertificateNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			typeSecretNamespacedName := types.NamespacedName{
				Name:      SecretName,
				Namespace: CertificateNamespace,
			}

			ssCert := &certs.SelfSignedCert{}

			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeSecretNamespacedName, secret)
				if err != nil {
					return false
				}
				err = ssCert.Read(ctx, secret)
				if err != nil {
					return false
				}
				return ssCert.Domain == certificate.Spec.DNSName && ssCert.Validity == certificate.Spec.Validity
			}, "30s", "1s").Should(BeTrue())

		})
	})

	// Update test.
	// Update an existing certificate CR .spec.secreteRef.Name field.
	// Check if update to certificate CR was successful.
	Context("Updating an existing certificate CR .spec.secretRef.name successfully", func() {
		It("Should update certificate CR", func() {
			By("Updating an existing certificate CR .spec.secretRef.name")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			certificate.Spec.SecretRef.Name = UpdatedSecretName

			Expect(k8sClient.Update(ctx, certificate)).Should(Succeed())
		})
	})

	// Update test.
	// Fetch existing certificate CR.
	// Check if .spec.secretRef.name field reflects updated secret name.
	Context("Check if certificate CR .spec.secretRef.name is updated successfully", func() {
		It("Should check certificate CR .spec.secretRef.name is updated", func() {
			By("Fetching certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return ""
				}
				return certificate.Spec.SecretRef.Name
			}, "30s", "1s").Should(Equal(UpdatedSecretName))

		})
	})

	// Update test.
	// The secret should be recreated with the updated name
	// as per .spec.secretRef.name of certificate CR.
	// The secret is recreated instead of update as it's a change of name.
	Context("Secret should be created as per .spec.secretRef.name of certificate CR", func() {
		It("Should check if secret is created as per .spec.secretRef.name of certificate CR", func() {
			By("Fetching secret")

			typeCertificateNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeCertificateNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			typeSecretNamespacedName := types.NamespacedName{
				Name:      certificate.Spec.SecretRef.Name,
				Namespace: CertificateNamespace,
			}

			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeSecretNamespacedName, secret)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

		})
	})

	// Update test.
	// The old secret should be deleted
	Context("Old secret should be deleted successfully", func() {
		It("Should check if old secret is deleted", func() {
			By("Fetching old secret")

			typeNamespacedName := types.NamespacedName{
				Name:      SecretName,
				Namespace: CertificateNamespace,
			}

			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, secret)
				if err != nil {
					return errors.IsNotFound(err)
				}
				return false
			}, "30s", "1s").Should(BeTrue())

		})
	})

	// Delete test.
	// Check if certificate CR can be deleted successfully.
	Context("Delete an existing certificate CR successfully", func() {
		It("Should delete an existing certificate CR", func() {
			By("Fetching an existing certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				return err == nil
			}, "30s", "1s").Should(BeTrue())

			Expect(k8sClient.Delete(ctx, certificate)).Should(Succeed())

		})
	})

	// Delete test.
	// The certificate CR should be deleted.
	Context("Certificate CR should be deleted successfully", func() {
		It("Should check if certificate CR is deleted", func() {
			By("Fetching the certificate CR")

			typeNamespacedName := types.NamespacedName{
				Name:      CertificateName,
				Namespace: CertificateNamespace,
			}

			certificate := &certsv1.Certificate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, certificate)
				if err != nil {
					return errors.IsNotFound(err)
				}
				return false
			}, "30s", "1s").Should(BeTrue())

		})
	})

	// Delete test.
	// The secret should be deleted
	Context("Secret should be deleted successfully", func() {
		It("Should check if secret is deleted", func() {
			By("Fetching the secret")

			typeNamespacedName := types.NamespacedName{
				Name:      UpdatedSecretName,
				Namespace: CertificateNamespace,
			}

			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, secret)
				if err != nil {
					return errors.IsNotFound(err)
				}
				return false
			}, "30s", "1s").Should(BeTrue())

		})
	})

})
