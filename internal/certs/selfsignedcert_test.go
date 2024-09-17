package certs

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Certificate Controller", func() {

	const (
		DNSName           = "test.k8c.io"
		Validity          = "30d"
		PrivateKeyBitSize = 2048
	)
	var (
		cert *SelfSignedCert
		ctx  context.Context
	)

	BeforeEach(func() {
		cert = &SelfSignedCert{Domain: DNSName, Validity: Validity}
		ctx = context.Background()
	})

	// Use Create method of SelfSignedCert object to create a
	// Self Signed Certificate and Private Key.
	// Decode, Parse Certificate and Private key to check if any error occurs.
	// Validate the parsed certificate Subject name and NotBefore/After with
	// DNSName and Validity provided as input during certificate creation.
	Context("Running create method creates valid Self Signed Certificate and Private Key", func() {
		It("Should create valid Self Signed Certificate and Private Key", func() {
			By("Running create method")

			certPEM, keyPEM, err := cert.Create(ctx, PrivateKeyBitSize)
			Expect(err).NotTo(HaveOccurred())
			Expect(certPEM).NotTo(BeNil())
			Expect(keyPEM).NotTo(BeNil())

			block, _ := pem.Decode(certPEM)
			Expect(block).NotTo(BeNil())
			Expect(block.Type).To(Equal("CERTIFICATE"))

			x509Cert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			block, _ = pem.Decode(keyPEM)
			Expect(block).NotTo(BeNil())
			Expect(block.Type).To(Equal("PRIVATE KEY"))

			_, err = x509.ParsePKCS8PrivateKey(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			certValidDays := (x509Cert.NotAfter.Sub(x509Cert.NotBefore).Hours()) / 24
			certValidity := fmt.Sprintf("%vd", int(certValidDays))

			Expect(x509Cert.Subject.CommonName).To(Equal(DNSName))

			Expect(certValidity).To(Equal(Validity))

		})
	})

	// A Self Signed Certificate and Private Key is created and
	// passed to read method of SelfSignedCert object by including it in a secret object.
	// The secret object is read, parsed and data extracted is initialized to
	// Domain and Validity fields of the SelfSignedCert object. This is validated against
	// DNSName and Validity used to create Self Signed Certificate and Private Key.
	Context("Running create method creates valid Self Signed Certificate and Private Key", func() {
		It("Should read Certificate and Private Key successfully", func() {
			By("Running read method")

			certPEM, keyPEM, err := cert.Create(ctx, PrivateKeyBitSize)
			Expect(err).NotTo(HaveOccurred())
			Expect(certPEM).NotTo(BeNil())
			Expect(keyPEM).NotTo(BeNil())

			secret := &corev1.Secret{
				Data: map[string][]byte{
					"tls.crt": certPEM,
					"tls.key": keyPEM,
				},
			}

			err = cert.Read(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			Expect(cert.Domain).To(Equal(DNSName))
			Expect(cert.Validity).To(Equal(Validity))

		})

	})
})
