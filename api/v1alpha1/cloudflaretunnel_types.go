// Package v1alpha1 contains API Schema definitions for the cfgate v1alpha1 API group.
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TunnelIdentity defines the tunnel identification configuration.
// Uses a single idempotent pathway: resolve by name, create if not exists.
type TunnelIdentity struct {
	// Name is the tunnel name in Cloudflare. If tunnel with this name exists, adopt it.
	// If not, create it. Tunnel ID is stored in status after resolution/creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
}

// CloudflareConfig defines the Cloudflare API credentials configuration.
// +kubebuilder:validation:XValidation:rule="has(self.accountId) || has(self.accountName)",message="either accountId or accountName must be specified"
type CloudflareConfig struct {
	// AccountID is the Cloudflare Account ID.
	// +optional
	// +kubebuilder:validation:MaxLength=32
	AccountID string `json:"accountId,omitempty"`

	// AccountName is the Cloudflare Account name. Will be looked up via API.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	AccountName string `json:"accountName,omitempty"`

	// SecretRef references the Secret containing Cloudflare API credentials.
	// The secret must contain an API token (not tunnel token).
	// +kubebuilder:validation:Required
	SecretRef SecretRef `json:"secretRef"`

	// SecretKeys defines the key mappings within the secret.
	// +optional
	SecretKeys SecretKeys `json:"secretKeys,omitempty"`
}

// SecretRef references a Kubernetes Secret.
type SecretRef struct {
	// Name of the secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Namespace of the secret. Defaults to the tunnel's namespace.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace,omitempty"`
}

// SecretReference references a secret in a namespace.
// Used for fallback credentials which may be in a different namespace.
type SecretReference struct {
	// Name of the secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Namespace of the secret. Defaults to the resource's namespace if empty.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace,omitempty"`
}

// SecretKeys defines the key mappings within the credentials secret.
type SecretKeys struct {
	// APIToken is the key name for the Cloudflare API token.
	// +kubebuilder:default=CLOUDFLARE_API_TOKEN
	// +kubebuilder:validation:MaxLength=253
	APIToken string `json:"apiToken,omitempty"`
}

// CloudflaredConfig defines the cloudflared deployment configuration.
type CloudflaredConfig struct {
	// Replicas is the number of cloudflared replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=2
	Replicas int32 `json:"replicas,omitempty"`

	// Image is the cloudflared container image.
	// +kubebuilder:default="cloudflare/cloudflared:latest"
	// +kubebuilder:validation:MaxLength=255
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the pull policy for the cloudflared image.
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	// +kubebuilder:default=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Protocol is the tunnel transport protocol: auto, quic, http2.
	// +kubebuilder:validation:Enum=auto;quic;http2
	// +kubebuilder:default=auto
	Protocol string `json:"protocol,omitempty"`

	// Resources are the resource requirements for cloudflared containers.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// NodeSelector is a selector for nodes to run cloudflared on.
	// +optional
	// +kubebuilder:validation:MaxProperties=50
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are tolerations for the cloudflared pods.
	// +optional
	// +kubebuilder:validation:MaxItems=20
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// PodAnnotations are annotations to add to cloudflared pods.
	// +optional
	// +kubebuilder:validation:MaxProperties=50
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// ExtraArgs are additional arguments to pass to cloudflared.
	// +optional
	// +kubebuilder:validation:MaxItems=20
	ExtraArgs []string `json:"extraArgs,omitempty"`

	// Metrics configures the cloudflared metrics endpoint.
	// +optional
	Metrics MetricsConfig `json:"metrics,omitempty"`
}

// MetricsConfig defines the metrics endpoint configuration.
type MetricsConfig struct {
	// Enabled enables the metrics endpoint.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Port is the port for the metrics endpoint.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=44483
	Port int32 `json:"port,omitempty"`
}

// OriginDefaults defines default settings for origin connections.
type OriginDefaults struct {
	// ConnectTimeout is the timeout for connecting to the origin.
	// +kubebuilder:default="30s"
	// +kubebuilder:validation:Pattern=`^[0-9]+(s|m|h)$`
	ConnectTimeout string `json:"connectTimeout,omitempty"`

	// NoTLSVerify disables TLS verification for origin connections.
	// +kubebuilder:default=false
	NoTLSVerify bool `json:"noTLSVerify,omitempty"`

	// HTTP2Origin enables HTTP/2 for origin connections.
	// +kubebuilder:default=false
	HTTP2Origin bool `json:"http2Origin,omitempty"`

	// CAPoolSecretRef references a Secret containing CA certificates for origin verification.
	// +optional
	CAPoolSecretRef *CAPoolSecretRef `json:"caPoolSecretRef,omitempty"`
}

// CAPoolSecretRef references a Secret containing CA certificates.
type CAPoolSecretRef struct {
	// Name of the secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Key is the key within the secret data.
	// +kubebuilder:default="ca.crt"
	// +kubebuilder:validation:MaxLength=253
	Key string `json:"key,omitempty"`
}

// CloudflareTunnelSpec defines the desired state of CloudflareTunnel.
type CloudflareTunnelSpec struct {
	// Tunnel defines the tunnel identity configuration.
	// +kubebuilder:validation:Required
	Tunnel TunnelIdentity `json:"tunnel"`

	// Cloudflare defines the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareConfig `json:"cloudflare"`

	// Cloudflared defines the cloudflared deployment configuration.
	// +optional
	Cloudflared CloudflaredConfig `json:"cloudflared,omitempty"`

	// OriginDefaults defines default settings for origin connections.
	// +optional
	OriginDefaults OriginDefaults `json:"originDefaults,omitempty"`

	// FallbackTarget is the service for unmatched requests.
	// +kubebuilder:default="http_status:404"
	FallbackTarget string `json:"fallbackTarget,omitempty"`

	// FallbackCredentialsRef references a secret containing fallback Cloudflare API credentials.
	// Used during deletion when primary credentials (in Cloudflare.SecretRef) are unavailable.
	// This enables cleanup of Cloudflare resources even if the per-tunnel secret is deleted.
	// The secret must contain the same keys as the primary credentials secret.
	// +optional
	FallbackCredentialsRef *SecretReference `json:"fallbackCredentialsRef,omitempty"`
}

// CloudflareTunnelStatus defines the observed state of CloudflareTunnel.
type CloudflareTunnelStatus struct {
	// TunnelID is the Cloudflare tunnel ID.
	TunnelID string `json:"tunnelId,omitempty"`

	// TunnelName is the Cloudflare tunnel name.
	TunnelName string `json:"tunnelName,omitempty"`

	// TunnelDomain is the tunnel's CNAME target domain (e.g., {tunnelId}.cfargotunnel.com).
	TunnelDomain string `json:"tunnelDomain,omitempty"`

	// AccountID is the resolved Cloudflare account ID.
	AccountID string `json:"accountId,omitempty"`

	// Replicas is the total number of cloudflared replicas.
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready cloudflared replicas.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// ObservedGeneration is the generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the last time the configuration was synced to Cloudflare.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ConnectedRouteCount is the number of routes connected to this tunnel.
	ConnectedRouteCount int32 `json:"connectedRouteCount,omitempty"`

	// Conditions represent the latest available observations of the tunnel's state.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cft;cftunnel
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Tunnel ID",type="string",JSONPath=".status.tunnelId"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudflareTunnel is the Schema for the cloudflaretunnels API.
// It manages the Cloudflare Tunnel lifecycle independent of routing.
type CloudflareTunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareTunnelSpec   `json:"spec,omitempty"`
	Status CloudflareTunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareTunnelList contains a list of CloudflareTunnel.
type CloudflareTunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareTunnel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareTunnel{}, &CloudflareTunnelList{})
}
