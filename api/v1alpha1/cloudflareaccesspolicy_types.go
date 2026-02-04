// Package v1alpha1 contains API Schema definitions for the cfgate v1alpha1 API group.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyTargetReference identifies a target for policy attachment.
// Based on Gateway API LocalPolicyTargetReferenceWithSectionName.
// +kubebuilder:validation:XValidation:rule="self.group == 'gateway.networking.k8s.io'",message="group must be gateway.networking.k8s.io"
// +kubebuilder:validation:XValidation:rule="self.kind in ['Gateway', 'HTTPRoute', 'GRPCRoute', 'TCPRoute', 'UDPRoute']",message="kind must be Gateway, HTTPRoute, GRPCRoute, TCPRoute, or UDPRoute"
type PolicyTargetReference struct {
	// Group is the API group of the target resource.
	// +kubebuilder:default="gateway.networking.k8s.io"
	Group string `json:"group"`

	// Kind is the kind of the target resource.
	// +kubebuilder:validation:Enum=Gateway;HTTPRoute;GRPCRoute;TCPRoute;UDPRoute
	Kind string `json:"kind"`

	// Name is the name of the target resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Namespace is the namespace of the target resource.
	// Cross-namespace targeting requires ReferenceGrant.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// SectionName targets specific listener (Gateway) or rule (Route).
	// +optional
	SectionName *string `json:"sectionName,omitempty"`
}

// CloudflareSecretRef references Cloudflare credentials.
type CloudflareSecretRef struct {
	// Name of the secret containing credentials.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the secret (defaults to policy namespace).
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// AccountID is the Cloudflare account ID.
	// +optional
	AccountID string `json:"accountId,omitempty"`

	// AccountName is the Cloudflare account name (looked up via API).
	// +optional
	AccountName string `json:"accountName,omitempty"`
}

// AccessApplication defines Cloudflare Access Application settings.
type AccessApplication struct {
	// Name is the display name in Cloudflare dashboard.
	// Defaults to CR name if omitted.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Domain is the protected domain (auto-generated from routes if omitted).
	// +optional
	Domain string `json:"domain,omitempty"`

	// Path restricts protection to specific path prefix.
	// +optional
	// +kubebuilder:default="/"
	Path string `json:"path,omitempty"`

	// SessionDuration controls session cookie lifetime.
	// +optional
	// +kubebuilder:default="24h"
	// +kubebuilder:validation:Pattern=`^[0-9]+(h|m|s)$`
	SessionDuration string `json:"sessionDuration,omitempty"`

	// Type is the application type.
	// +kubebuilder:validation:Enum=self_hosted;saas;ssh;vnc;browser_isolation
	// +kubebuilder:default=self_hosted
	Type string `json:"type,omitempty"`

	// LogoURL is the application logo in dashboard.
	// +optional
	LogoURL string `json:"logoUrl,omitempty"`

	// SkipInterstitial bypasses the Access login page for API requests.
	// +optional
	// +kubebuilder:default=false
	SkipInterstitial bool `json:"skipInterstitial,omitempty"`

	// EnableBindingCookie enables binding cookies for sticky sessions.
	// +optional
	// +kubebuilder:default=false
	EnableBindingCookie bool `json:"enableBindingCookie,omitempty"`

	// HttpOnlyCookieAttribute adds HttpOnly to session cookies.
	// +optional
	// +kubebuilder:default=true
	HttpOnlyCookieAttribute bool `json:"httpOnlyCookieAttribute,omitempty"`

	// SameSiteCookieAttribute controls cross-site cookie behavior.
	// +kubebuilder:validation:Enum=strict;lax;none
	// +kubebuilder:default=lax
	SameSiteCookieAttribute string `json:"sameSiteCookieAttribute,omitempty"`

	// CustomDenyMessage shown when access is denied.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	CustomDenyMessage string `json:"customDenyMessage,omitempty"`

	// CustomDenyURL redirects to this URL when denied (instead of message).
	// +optional
	CustomDenyURL string `json:"customDenyUrl,omitempty"`
}

// AccessPolicyRule defines an access allow/deny rule.
type AccessPolicyRule struct {
	// Name is a human-readable identifier.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Decision is the policy action.
	// +kubebuilder:validation:Enum=allow;deny;bypass;non_identity
	// +kubebuilder:default=allow
	Decision string `json:"decision"`

	// Precedence determines rule evaluation order (lower = first).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=9999
	// +optional
	Precedence *int `json:"precedence,omitempty"`

	// Include rules (ANY must match for rule to apply).
	// +optional
	// +kubebuilder:validation:MaxItems=25
	Include []AccessRule `json:"include,omitempty"`

	// Exclude rules (if ANY match, rule does not apply).
	// +optional
	// +kubebuilder:validation:MaxItems=25
	Exclude []AccessRule `json:"exclude,omitempty"`

	// Require rules (ALL must match for rule to apply).
	// +optional
	// +kubebuilder:validation:MaxItems=25
	Require []AccessRule `json:"require,omitempty"`

	// SessionDuration overrides application session duration for this rule.
	// +optional
	SessionDuration string `json:"sessionDuration,omitempty"`

	// PurposeJustificationRequired requires user to provide justification.
	// +optional
	// +kubebuilder:default=false
	PurposeJustificationRequired bool `json:"purposeJustificationRequired,omitempty"`

	// PurposeJustificationPrompt is the prompt shown to user.
	// +optional
	PurposeJustificationPrompt string `json:"purposeJustificationPrompt,omitempty"`

	// ApprovalRequired requires approval from specific users.
	// +optional
	// +kubebuilder:default=false
	ApprovalRequired bool `json:"approvalRequired,omitempty"`

	// ApprovalGroups defines who can approve access.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	ApprovalGroups []ApprovalGroup `json:"approvalGroups,omitempty"`
}

// AccessRule defines identity matching criteria.
// SDK types: IPRule, IPListRule, CountryRule, EveryoneRule, ServiceTokenRule, AnyValidServiceTokenRule,
// EmailRule, DomainRule, EmailListRule, AccessOIDCClaimRule, GSuiteGroupRule.
//
// +kubebuilder:validation:XValidation:rule="[has(self.ip), has(self.ipList), has(self.country), has(self.everyone), has(self.serviceToken), has(self.anyValidServiceToken), has(self.email), has(self.emailList), has(self.emailDomain), has(self.oidcClaim), has(self.gsuiteGroup)].exists(x, x)",message="at least one rule type must be specified"
type AccessRule struct {
	// ============================================================
	// P0: No IdP Required
	// ============================================================

	// IP matches source IP CIDR ranges.
	// SDK: IPRule
	// +optional
	IP *AccessIPRule `json:"ip,omitempty"`

	// IPList references a Cloudflare IP List.
	// SDK: IPListRule
	// +optional
	IPList *AccessIPListRule `json:"ipList,omitempty"`

	// Country matches source country codes (ISO 3166-1 alpha-2).
	// SDK: CountryRule
	// +optional
	Country *AccessCountryRule `json:"country,omitempty"`

	// Everyone matches all users (use with caution).
	// SDK: EveryoneRule
	// +optional
	Everyone *bool `json:"everyone,omitempty"`

	// ServiceToken matches a specific service token by ID.
	// SDK: ServiceTokenRule
	// +optional
	ServiceToken *AccessServiceTokenRule `json:"serviceToken,omitempty"`

	// AnyValidServiceToken matches any valid service token.
	// SDK: AnyValidServiceTokenRule
	// +optional
	AnyValidServiceToken *bool `json:"anyValidServiceToken,omitempty"`

	// ============================================================
	// P1: Basic IdP Required (Google Workspace)
	// ============================================================

	// Email matches specific email addresses.
	// SDK: EmailRule
	// +optional
	Email *AccessEmailRule `json:"email,omitempty"`

	// EmailList references a Cloudflare Access email list.
	// SDK: EmailListRule
	// +optional
	EmailList *AccessEmailListRule `json:"emailList,omitempty"`

	// EmailDomain matches email domain suffix.
	// SDK: DomainRule
	// +optional
	EmailDomain *AccessEmailDomainRule `json:"emailDomain,omitempty"`

	// OIDCClaim matches OIDC token claims.
	// SDK: AccessOIDCClaimRule
	// +optional
	OIDCClaim *AccessOIDCClaimRule `json:"oidcClaim,omitempty"`

	// ============================================================
	// P2: Google Workspace Groups
	// ============================================================

	// GSuiteGroup matches Google Workspace groups.
	// SDK: GSuiteGroupRule
	// +optional
	GSuiteGroup *AccessGSuiteGroupRule `json:"gsuiteGroup,omitempty"`

	// ============================================================
	// P3: Deferred to v0.2.0
	// ============================================================
	// The following rule types are NOT included in alpha.3:
	// - Certificate (CertificateRule) - mTLS client cert
	// - CommonName (AccessCommonNameRule) - mTLS CN matching
	// - Group (GroupRule) - Access Groups
	// - GitHub (GitHubOrganizationRule) - GitHub org/team
	// - Azure (AzureGroupRule) - Azure AD groups
	// - Okta (OktaGroupRule) - Okta groups
	// - SAML (SAMLGroupRule) - SAML attributes
	// - AuthenticationMethod (AuthenticationMethodRule) - MFA enforcement
	// - DevicePosture (AccessDevicePostureRule) - Device compliance
	// - ExternalEvaluation (ExternalEvaluationRule) - External eval
	// - LoginMethod (AccessLoginMethodRule) - Login method
}

// ============================================================
// P0 Rule Types (No IdP Required)
// ============================================================

// AccessIPRule matches source IP CIDR ranges.
// Maps to SDK: IPRule
type AccessIPRule struct {
	// Ranges are CIDR blocks (IPv4 or IPv6).
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	Ranges []string `json:"ranges"`
}

// AccessIPListRule references a Cloudflare IP List.
// Maps to SDK: IPListRule
// +kubebuilder:validation:XValidation:rule="has(self.id) || has(self.name)",message="either id or name must be specified"
type AccessIPListRule struct {
	// ID of the IP list in Cloudflare.
	// +optional
	// +kubebuilder:validation:MaxLength=36
	ID string `json:"id,omitempty"`

	// Name of the IP list (looked up via API).
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`
}

// AccessCountryRule matches source country codes.
// Maps to SDK: CountryRule
type AccessCountryRule struct {
	// Codes are ISO 3166-1 alpha-2 country codes.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	Codes []string `json:"codes"`
}

// AccessServiceTokenRule matches a specific service token.
// Maps to SDK: ServiceTokenRule
type AccessServiceTokenRule struct {
	// TokenID is the Cloudflare service token ID.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=36
	TokenID string `json:"tokenId"`
}

// ============================================================
// P1 Rule Types (Basic IdP Required)
// ============================================================

// AccessEmailRule matches specific email addresses.
// Maps to SDK: EmailRule
type AccessEmailRule struct {
	// Addresses to match.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	Addresses []string `json:"addresses"`
}

// AccessEmailListRule references a Cloudflare Access email list.
// Maps to SDK: EmailListRule
// +kubebuilder:validation:XValidation:rule="has(self.id) || has(self.name)",message="either id or name must be specified"
type AccessEmailListRule struct {
	// ID of the Access list in Cloudflare.
	// +optional
	// +kubebuilder:validation:MaxLength=36
	ID string `json:"id,omitempty"`

	// Name of the Access list (looked up via API).
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`
}

// AccessEmailDomainRule matches email domain suffix.
// Maps to SDK: DomainRule
type AccessEmailDomainRule struct {
	// Domain suffix (e.g., "example.com").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Domain string `json:"domain"`
}

// AccessOIDCClaimRule matches OIDC token claims.
// Maps to SDK: AccessOIDCClaimRule
type AccessOIDCClaimRule struct {
	// IdentityProviderID in Cloudflare.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=36
	IdentityProviderID string `json:"identityProviderId"`

	// ClaimName is the OIDC claim to match.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ClaimName string `json:"claimName"`

	// ClaimValue is the expected value.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ClaimValue string `json:"claimValue"`
}

// ============================================================
// P2 Rule Types (Google Workspace Groups)
// ============================================================

// AccessGSuiteGroupRule matches Google Workspace groups.
// Maps to SDK: GSuiteGroupRule
type AccessGSuiteGroupRule struct {
	// IdentityProviderID in Cloudflare.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=36
	IdentityProviderID string `json:"identityProviderId"`

	// Email is the Google Workspace group email.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=320
	Email string `json:"email"`
}

// ============================================================
// Supporting Types
// ============================================================

// AccessGroupRef references an AccessGroup CR or Cloudflare group.
// +kubebuilder:validation:XValidation:rule="has(self.name) || has(self.cloudflareId)",message="either name or cloudflareId must be specified"
type AccessGroupRef struct {
	// Name of AccessGroup CR in same namespace.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// CloudflareID of group in Cloudflare (bypasses CR lookup).
	// +optional
	// +kubebuilder:validation:MaxLength=36
	CloudflareID string `json:"cloudflareId,omitempty"`
}

// ApprovalGroup defines who can approve access requests.
// +kubebuilder:validation:XValidation:rule="size(self.emails) > 0 || has(self.emailDomain)",message="at least one approver (emails or emailDomain) must be specified"
type ApprovalGroup struct {
	// Emails of approvers.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	Emails []string `json:"emails,omitempty"`

	// EmailDomain allows any user from domain to approve.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	EmailDomain string `json:"emailDomain,omitempty"`

	// ApprovalsNeeded is number of approvals required.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	ApprovalsNeeded int `json:"approvalsNeeded,omitempty"`
}

// ServiceTokenConfig defines machine-to-machine authentication.
type ServiceTokenConfig struct {
	// Name is the token display name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Duration is the token validity period using Go duration format.
	// Only hours (h) supported by Cloudflare API. Use "8760h" for 1 year.
	// +kubebuilder:validation:Pattern=`^[0-9]+h$`
	// +kubebuilder:default="8760h"
	Duration string `json:"duration,omitempty"`

	// SecretRef stores the generated token credentials.
	// +kubebuilder:validation:Required
	SecretRef ServiceTokenSecretRef `json:"secretRef"`
}

// ServiceTokenSecretRef references a Kubernetes Secret for service token storage.
type ServiceTokenSecretRef struct {
	// Name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// MTLSConfig defines certificate-based authentication.
type MTLSConfig struct {
	// Enabled activates mTLS requirement.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// RootCASecretRef references the CA certificate(s) for validation.
	// +optional
	RootCASecretRef *CASecretRef `json:"rootCaSecretRef,omitempty"`

	// AssociatedHostnames limits mTLS to specific hostnames.
	// +optional
	// +kubebuilder:validation:MaxItems=25
	AssociatedHostnames []string `json:"associatedHostnames,omitempty"`

	// RuleName is the name of the mTLS rule in Cloudflare.
	// Defaults to CR name if omitted.
	// +optional
	RuleName string `json:"ruleName,omitempty"`
}

// CASecretRef references a CA certificate Secret.
type CASecretRef struct {
	// Name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key within Secret (defaults to ca.crt).
	// +kubebuilder:default="ca.crt"
	Key string `json:"key,omitempty"`
}

// CloudflareAccessPolicySpec defines the desired state of CloudflareAccessPolicy.
// +kubebuilder:validation:XValidation:rule="has(self.targetRef) || has(self.targetRefs)",message="either targetRef or targetRefs must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.targetRef) && has(self.targetRefs))",message="targetRef and targetRefs are mutually exclusive"
type CloudflareAccessPolicySpec struct {
	// TargetRef identifies a single target for policy attachment.
	// +optional
	TargetRef *PolicyTargetReference `json:"targetRef,omitempty"`

	// TargetRefs identifies multiple targets for policy attachment.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []PolicyTargetReference `json:"targetRefs,omitempty"`

	// CloudflareRef references Cloudflare credentials (inherits from tunnel if omitted).
	// +optional
	CloudflareRef *CloudflareSecretRef `json:"cloudflareRef,omitempty"`

	// Application defines the Access Application settings.
	Application AccessApplication `json:"application"`

	// Policies define access rules (evaluated in order).
	// +optional
	// +kubebuilder:validation:MaxItems=50
	Policies []AccessPolicyRule `json:"policies,omitempty"`

	// GroupRefs reference reusable identity rules.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	GroupRefs []AccessGroupRef `json:"groupRefs,omitempty"`

	// ServiceTokens for machine-to-machine authentication.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	ServiceTokens []ServiceTokenConfig `json:"serviceTokens,omitempty"`

	// MTLS configures certificate-based authentication.
	// +optional
	MTLS *MTLSConfig `json:"mtls,omitempty"`
}

// PolicyAncestorStatus describes attachment status per target.
// Follows Gateway API PolicyAncestorStatus pattern.
type PolicyAncestorStatus struct {
	// AncestorRef identifies the target.
	AncestorRef PolicyTargetReference `json:"ancestorRef"`

	// ControllerName identifies the controller managing this attachment.
	ControllerName string `json:"controllerName"`

	// Conditions for this specific target.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// CloudflareAccessPolicyStatus defines the observed state of CloudflareAccessPolicy.
type CloudflareAccessPolicyStatus struct {
	// ApplicationID is the Cloudflare Access Application ID.
	ApplicationID string `json:"applicationId,omitempty"`

	// ApplicationAUD is the Application Audience Tag.
	ApplicationAUD string `json:"applicationAud,omitempty"`

	// AttachedTargets is the count of successfully attached targets.
	AttachedTargets int32 `json:"attachedTargets,omitempty"`

	// ServiceTokenIDs maps token names to Cloudflare IDs.
	ServiceTokenIDs map[string]string `json:"serviceTokenIds,omitempty"`

	// MTLSRuleID is the Cloudflare mTLS rule ID.
	MTLSRuleID string `json:"mtlsRuleId,omitempty"`

	// ObservedGeneration is the last generation processed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe current state.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Ancestors contains status for each targetRef (Gateway API PolicyStatus).
	// +optional
	Ancestors []PolicyAncestorStatus `json:"ancestors,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfap;cfaccess
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Application",type="string",JSONPath=".status.applicationId"
// +kubebuilder:printcolumn:name="Targets",type="integer",JSONPath=".status.attachedTargets"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudflareAccessPolicy is the Schema for the cloudflareaccespolicies API.
// It manages Cloudflare Access Applications and Policies for zero-trust access control.
type CloudflareAccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareAccessPolicySpec   `json:"spec,omitempty"`
	Status CloudflareAccessPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareAccessPolicyList contains a list of CloudflareAccessPolicy.
type CloudflareAccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareAccessPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareAccessPolicy{}, &CloudflareAccessPolicyList{})
}
