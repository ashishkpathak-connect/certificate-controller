package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CertificateAvailable   string = "Available"
	CertificateProgressing string = "Progressing"
	CertificateDegraded    string = "Degraded"

	ReasonAvailable   string = "CertificateReady"
	ReasonProgressing string = "CertificateInProgress"
	ReasonDegraded    string = "CertificateFailed"
)

// SecretReference represents a Secret Reference.
type SecretReference struct {

	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`

	// The name of the Secret in which certificate is stored.
	// This should be a valid DNS Subdomain Name. It cannot be empty.
	Name string `json:"name"`
}

// CertificateSpec defines the desired state of Certificate
type CertificateSpec struct {

	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`

	// The DNS name for which the certificate should be issued.
	// This should be a valid DNS Subdomain Name. It cannot be empty.
	DNSName string `json:"dnsName"`

	// +kubebuilder:validation:Pattern=`^\d+d$`

	// The time until the certificate expires.
	// Accepts sequence of 1 or more digits followed by the letter 'd' which indicates days.
	Validity string `json:"validity"`

	// A reference to the Secret object in which certificate is stored.
	SecretRef SecretReference `json:"secretRef"`
}

// CertificateStatus defines the observed state of Certificate
type CertificateStatus struct {

	// Condition contains details for one aspect of the current state of this API Resource.
	// +optional
	Condition *metav1.Condition `json:"condition,omitempty"`

	// Name of the Secret object which is created.
	// +optional
	SecretName *string `json:"secretName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status", type="string", JSONPath=".status.condition.type", description="current status"

// Certificate is the Schema for the certificates API
type Certificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec CertificateSpec `json:"spec,omitempty"`

	Status CertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CertificateList contains a list of Certificate
type CertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Certificate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Certificate{}, &CertificateList{})
}
