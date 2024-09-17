package certs

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type SelfSignedCert struct {
	Domain   string
	Validity string
}

const (
	PrivateKeyBitSize int = 2048
)

func (s *SelfSignedCert) Create(ctx context.Context, keySize int) ([]byte, []byte, error) {
	log := log.FromContext(ctx)
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		log.Error(err, "unable to create private key")
		return nil, nil, err
	}

	// Create a random serial number.
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)

	if err != nil {
		log.Error(err, "unable to create serial number for certificate")
		return nil, nil, err
	}

	// Extract integer from validity string.
	re := regexp.MustCompile(`[0-9]`)
	validStr := strings.Join(re.FindAllString(s.Validity, -1), "")
	validDays, err := strconv.Atoi(validStr)
	if err != nil {
		log.Error(err, "unable to convert string to int")
		return nil, nil, err
	}

	// Create a x509 certificate template.
	x509Template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   s.Domain,
			Organization: []string{s.Domain},
		},
		NotBefore:   time.Now().UTC(),
		NotAfter:    time.Now().Add(time.Duration(validDays) * 24 * time.Hour).UTC(),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Create self signed certificate.
	certBytes, err := x509.CreateCertificate(rand.Reader, &x509Template, &x509Template, &privateKey.PublicKey, privateKey)
	if err != nil {
		log.Error(err, "unable to create certificate")
		return nil, nil, err
	}

	// Serialize Private Key
	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		log.Error(err, "unable to serialize private key")
		return nil, nil, err
	}

	certPEM := &bytes.Buffer{}
	keyPEM := &bytes.Buffer{}

	// Encode certificate and key to PEM format
	err = pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	if err != nil {
		log.Error(err, "unable to encode certificate to PEM")
		return nil, nil, err
	}
	err = pem.Encode(keyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	if err != nil {
		log.Error(err, "unable to encode privateKey to PEM")
		return nil, nil, err
	}

	return certPEM.Bytes(), keyPEM.Bytes(), nil
}

func (s *SelfSignedCert) Read(ctx context.Context, secretObj *corev1.Secret) error {
	log := log.FromContext(ctx)
	certData, status := secretObj.Data["tls.crt"]
	if !status {
		err := fmt.Errorf("%v missing", "tls.crt")
		log.Error(err, "certificate does not exist in secret")
		return err
	}
	block, _ := pem.Decode(certData)
	if block == nil {
		err := fmt.Errorf("error decoding PEM block")
		log.Error(err, "unable to decode PEM formatted block. Certificate invalid")
		return err
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Error(err, "unable to parse certificate")
		return err
	}

	s.Domain = cert.Subject.CommonName
	log.Info("domain", "domain", s.Domain)
	validDays := (cert.NotAfter.Sub(cert.NotBefore).Hours()) / 24
	s.Validity = fmt.Sprintf("%vd", int(validDays))
	log.Info("validity", "validity", s.Validity)

	return nil

}
