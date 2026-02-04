// Package v1alpha1 contains API Schema definitions for the cfgate v1alpha1 API group.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSTunnelRef references a CloudflareTunnel resource for DNS CNAME target resolution.
// When specified, DNS records target the tunnel's domain ({tunnelId}.cfargotunnel.com).
type DNSTunnelRef struct {
	// Name is the name of the CloudflareTunnel.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// Namespace is the namespace of the CloudflareTunnel.
	// Defaults to the CloudflareDNS's namespace.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace,omitempty"`
}

// RecordType represents DNS record types supported by CloudflareDNS.
// +kubebuilder:validation:Enum=CNAME;A;AAAA
type RecordType string

const (
	// RecordTypeCNAME is a CNAME record type.
	RecordTypeCNAME RecordType = "CNAME"
	// RecordTypeA is an A record type.
	RecordTypeA RecordType = "A"
	// RecordTypeAAAA is an AAAA record type.
	RecordTypeAAAA RecordType = "AAAA"
)

// DNSPolicy defines the DNS record lifecycle policy.
// +kubebuilder:validation:Enum=sync;upsert-only;create-only
type DNSPolicy string

const (
	// DNSPolicySync creates, updates, and deletes records (default).
	DNSPolicySync DNSPolicy = "sync"
	// DNSPolicyUpsertOnly creates and updates only, never deletes.
	DNSPolicyUpsertOnly DNSPolicy = "upsert-only"
	// DNSPolicyCreateOnly creates only, never updates or deletes.
	DNSPolicyCreateOnly DNSPolicy = "create-only"
)

// ExternalTarget defines a non-tunnel DNS target for external CNAME, A, or AAAA records.
// +kubebuilder:validation:XValidation:rule="self.type == 'CNAME' || self.type == 'A' || self.type == 'AAAA'",message="type must be CNAME, A, or AAAA"
type ExternalTarget struct {
	// Type is the DNS record type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=CNAME;A;AAAA
	Type RecordType `json:"type"`

	// Value is the target value (domain for CNAME, IP for A/AAAA).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Value string `json:"value"`
}

// DNSZoneConfig defines a DNS zone to manage.
type DNSZoneConfig struct {
	// Name is the zone domain name (e.g., example.com).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// ID is the optional explicit zone ID (skips API lookup).
	// +optional
	// +kubebuilder:validation:MaxLength=32
	ID string `json:"id,omitempty"`

	// Proxied sets the default proxied setting for this zone.
	// nil inherits from spec.defaults.proxied.
	// +optional
	Proxied *bool `json:"proxied,omitempty"`
}

// DNSNamespaceSelector limits route discovery to specific namespaces.
// +kubebuilder:validation:XValidation:rule="has(self.matchLabels) || has(self.matchNames)",message="at least one selector must be specified"
type DNSNamespaceSelector struct {
	// MatchLabels selects namespaces with matching labels.
	// +optional
	// +kubebuilder:validation:MaxProperties=10
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchNames selects namespaces by name.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	MatchNames []string `json:"matchNames,omitempty"`
}

// DNSGatewayRoutesSource configures watching Gateway API routes for hostnames.
type DNSGatewayRoutesSource struct {
	// Enabled enables watching Gateway API routes.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// AnnotationFilter only syncs routes with this annotation.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	AnnotationFilter string `json:"annotationFilter,omitempty"`

	// NamespaceSelector limits route discovery to specific namespaces.
	// +optional
	NamespaceSelector *DNSNamespaceSelector `json:"namespaceSelector,omitempty"`
}

// DNSExplicitHostname defines an explicit hostname to sync.
// +kubebuilder:validation:XValidation:rule="!has(self.ttl) || self.ttl == 1 || (self.ttl >= 60 && self.ttl <= 86400)",message="TTL must be 1 (auto) or between 60 and 86400 seconds"
type DNSExplicitHostname struct {
	// Hostname is the DNS hostname to create.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Hostname string `json:"hostname"`

	// Target is the CNAME target. Supports template variable {{ .TunnelDomain }}.
	// Defaults to tunnel domain when tunnelRef is specified.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Target string `json:"target,omitempty"`

	// Proxied enables Cloudflare proxy for this record.
	// nil inherits from zone or defaults.
	// +optional
	Proxied *bool `json:"proxied,omitempty"`

	// TTL is the DNS record TTL in seconds. 1 means auto (Cloudflare managed).
	// Valid values: 1 (auto) or 60-86400 (explicit).
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=86400
	// +kubebuilder:default=1
	TTL int32 `json:"ttl,omitempty"`
}

// DNSHostnameSource defines sources for hostnames to sync.
type DNSHostnameSource struct {
	// GatewayRoutes configures watching Gateway API routes.
	// +optional
	GatewayRoutes DNSGatewayRoutesSource `json:"gatewayRoutes,omitempty"`

	// Explicit defines explicit hostnames to sync.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Explicit []DNSExplicitHostname `json:"explicit,omitempty"`
}

// DNSRecordDefaults defines default settings for DNS records.
// +kubebuilder:validation:XValidation:rule="!has(self.ttl) || self.ttl == 1 || (self.ttl >= 60 && self.ttl <= 86400)",message="TTL must be 1 (auto) or between 60 and 86400 seconds"
type DNSRecordDefaults struct {
	// Proxied enables Cloudflare proxy by default.
	// +kubebuilder:default=true
	Proxied bool `json:"proxied,omitempty"`

	// TTL is the default DNS record TTL in seconds.
	// Valid values: 1 (auto) or 60-86400 (explicit).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=86400
	// +kubebuilder:default=1
	TTL int32 `json:"ttl,omitempty"`
}

// DNSTXTRecordOwnership configures TXT record-based ownership tracking.
type DNSTXTRecordOwnership struct {
	// Enabled enables TXT record ownership tracking.
	// nil defaults to true.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Prefix is the prefix for TXT record names.
	// +kubebuilder:default="_cfgate"
	// +kubebuilder:validation:MaxLength=63
	Prefix string `json:"prefix,omitempty"`
}

// DNSCommentOwnership configures comment-based ownership tracking.
type DNSCommentOwnership struct {
	// Enabled enables comment-based ownership tracking.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Template is the comment template.
	// +kubebuilder:default="managed by cfgate"
	// +kubebuilder:validation:MaxLength=255
	Template string `json:"template,omitempty"`
}

// DNSOwnershipConfig defines how to track record ownership.
type DNSOwnershipConfig struct {
	// OwnerID is the cluster/installation identifier used in TXT ownership records.
	// Used to distinguish records created by different cfgate installations.
	// Defaults to the CloudflareDNS resource's namespace/name if not specified.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(/[a-z0-9]([-a-z0-9]*[a-z0-9])?)?$`
	OwnerID string `json:"ownerId,omitempty"`

	// TXTRecord configures TXT record-based ownership.
	// +optional
	TXTRecord DNSTXTRecordOwnership `json:"txtRecord,omitempty"`

	// Comment configures comment-based ownership.
	// +optional
	Comment DNSCommentOwnership `json:"comment,omitempty"`
}

// DNSCleanupPolicy defines what to do when records are no longer needed.
// All fields use *bool to distinguish between "not set" (nil, defaults to true) and "explicitly false".
type DNSCleanupPolicy struct {
	// DeleteOnRouteRemoval deletes records when the source route is deleted.
	// nil defaults to true.
	// +optional
	DeleteOnRouteRemoval *bool `json:"deleteOnRouteRemoval,omitempty"`

	// DeleteOnResourceRemoval deletes records when CloudflareDNS resource is deleted.
	// nil defaults to true.
	// +optional
	DeleteOnResourceRemoval *bool `json:"deleteOnResourceRemoval,omitempty"`

	// OnlyManaged only deletes records that were created by cfgate (verified via ownership).
	// nil defaults to true.
	// +optional
	OnlyManaged *bool `json:"onlyManaged,omitempty"`
}

// CloudflareDNSSpec defines the desired state of CloudflareDNS.
// +kubebuilder:validation:XValidation:rule="has(self.tunnelRef) || has(self.externalTarget)",message="either tunnelRef or externalTarget must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.tunnelRef) && has(self.externalTarget))",message="tunnelRef and externalTarget are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="has(self.tunnelRef) || has(self.cloudflare)",message="cloudflare credentials required when using externalTarget"
type CloudflareDNSSpec struct {
	// TunnelRef references a CloudflareTunnel for CNAME target resolution.
	// +optional
	TunnelRef *DNSTunnelRef `json:"tunnelRef,omitempty"`

	// ExternalTarget specifies a non-tunnel DNS target.
	// +optional
	ExternalTarget *ExternalTarget `json:"externalTarget,omitempty"`

	// Zones defines the DNS zones to manage.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	Zones []DNSZoneConfig `json:"zones"`

	// Policy controls DNS record lifecycle.
	// +kubebuilder:validation:Enum=sync;upsert-only;create-only
	// +kubebuilder:default=sync
	Policy DNSPolicy `json:"policy,omitempty"`

	// Source defines where to get hostnames to sync.
	// +optional
	Source DNSHostnameSource `json:"source,omitempty"`

	// Defaults defines default settings for DNS records.
	// +optional
	Defaults DNSRecordDefaults `json:"defaults,omitempty"`

	// Ownership defines how to track record ownership.
	// +optional
	Ownership DNSOwnershipConfig `json:"ownership,omitempty"`

	// CleanupPolicy defines cleanup behavior for records.
	// +optional
	CleanupPolicy DNSCleanupPolicy `json:"cleanupPolicy,omitempty"`

	// Cloudflare API credentials (required when using externalTarget).
	// When using tunnelRef, credentials are inherited from the tunnel.
	// +optional
	Cloudflare *CloudflareConfig `json:"cloudflare,omitempty"`

	// FallbackCredentialsRef references fallback Cloudflare API credentials.
	// Used during deletion when primary credentials are unavailable.
	// +optional
	FallbackCredentialsRef *SecretReference `json:"fallbackCredentialsRef,omitempty"`
}

// DNSRecordSyncStatus represents the status of a single DNS record.
type DNSRecordSyncStatus struct {
	// Hostname is the DNS hostname.
	Hostname string `json:"hostname"`

	// Type is the DNS record type (CNAME, A, AAAA).
	Type string `json:"type"`

	// Target is the record target/content.
	Target string `json:"target"`

	// Proxied indicates if Cloudflare proxy is enabled.
	Proxied bool `json:"proxied"`

	// TTL is the record TTL.
	TTL int32 `json:"ttl,omitempty"`

	// Status is the sync status: Synced, Pending, Failed.
	Status string `json:"status"`

	// RecordID is the Cloudflare record ID.
	// +optional
	RecordID string `json:"recordId,omitempty"`

	// ZoneID is the Cloudflare zone ID where the record was created.
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// Error contains the error message if status is Failed.
	// +optional
	Error string `json:"error,omitempty"`
}

// CloudflareDNSStatus defines the observed state of CloudflareDNS.
type CloudflareDNSStatus struct {
	// SyncedRecords is the number of successfully synced records.
	SyncedRecords int32 `json:"syncedRecords,omitempty"`

	// PendingRecords is the number of records pending sync.
	PendingRecords int32 `json:"pendingRecords,omitempty"`

	// FailedRecords is the number of records that failed to sync.
	FailedRecords int32 `json:"failedRecords,omitempty"`

	// Records contains the status of individual DNS records.
	// +optional
	// +kubebuilder:validation:MaxItems=1000
	Records []DNSRecordSyncStatus `json:"records,omitempty"`

	// ResolvedTarget is the resolved CNAME target (tunnel domain or external value).
	// +optional
	ResolvedTarget string `json:"resolvedTarget,omitempty"`

	// ObservedGeneration is the generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the last time records were synced.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfdns;dns
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Synced",type="integer",JSONPath=".status.syncedRecords"
// +kubebuilder:printcolumn:name="Pending",type="integer",JSONPath=".status.pendingRecords"
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failedRecords"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudflareDNS is the Schema for the cloudflaredns API.
// It manages DNS records independently from CloudflareTunnel resources.
type CloudflareDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareDNSSpec   `json:"spec,omitempty"`
	Status CloudflareDNSStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareDNSList contains a list of CloudflareDNS.
type CloudflareDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareDNS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareDNS{}, &CloudflareDNSList{})
}
