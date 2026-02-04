// Package cloudflare provides a wrapper around cloudflare-go for cfgate's needs.
package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

// AccessService handles Access-specific operations.
// It wraps the cloudflare-go client with cfgate-specific logic.
type AccessService struct {
	client Client
	log    logr.Logger
}

// NewAccessService creates a new AccessService.
func NewAccessService(client Client, log logr.Logger) *AccessService {
	return &AccessService{
		client: client,
		log:    log.WithName("access-service"),
	}
}

// SecretWriter is an interface for storing secrets.
type SecretWriter interface {
	WriteSecret(ctx context.Context, name string, data map[string][]byte) error
}

// AccessApplication represents a Cloudflare Access Application.
type AccessApplication struct {
	// ID is the unique application identifier.
	ID string

	// AUD is the Application Audience Tag for JWT validation.
	AUD string

	// Name is the application display name.
	Name string

	// Domain is the protected domain.
	Domain string

	// Type is the application type (self_hosted, saas, ssh, vnc, etc.).
	Type string

	// SessionDuration is the session cookie lifetime (e.g., "24h").
	SessionDuration string

	// AllowedIdps is the list of allowed identity provider IDs.
	AllowedIdps []string

	// AutoRedirectToIdentity auto-redirects to IdP if single provider.
	AutoRedirectToIdentity bool

	// EnableBindingCookie enables session binding cookies.
	EnableBindingCookie bool

	// HttpOnlyCookieAttribute sets HttpOnly flag on session cookies.
	HttpOnlyCookieAttribute bool

	// SameSiteCookieAttribute sets SameSite cookie attribute.
	SameSiteCookieAttribute string

	// SkipInterstitial skips Access login page for API requests.
	SkipInterstitial bool

	// LogoURL is the application logo URL.
	LogoURL string

	// AppLauncherVisible shows the app in App Launcher.
	AppLauncherVisible bool

	// CustomDenyMessage is the custom denial message.
	CustomDenyMessage string

	// CustomDenyURL is the custom denial redirect URL.
	CustomDenyURL string

	// CustomNonIdentityDenyURL is the denial URL for non-identity requests.
	CustomNonIdentityDenyURL string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time
}

// CreateApplicationParams contains parameters for creating an application.
type CreateApplicationParams struct {
	// Name is the application display name.
	Name string

	// Domain is the protected domain.
	Domain string

	// Type is the application type. Defaults to self_hosted.
	Type string

	// SessionDuration is the session lifetime. Defaults to "24h".
	SessionDuration string

	// AllowedIdps is the list of allowed identity provider IDs.
	AllowedIdps []string

	// AutoRedirectToIdentity auto-redirects if single IdP.
	AutoRedirectToIdentity bool

	// EnableBindingCookie enables sticky sessions.
	EnableBindingCookie bool

	// HttpOnlyCookieAttribute sets HttpOnly flag. Defaults to true.
	HttpOnlyCookieAttribute *bool

	// SameSiteCookieAttribute sets SameSite attribute. Defaults to "lax".
	SameSiteCookieAttribute string

	// SkipInterstitial skips login page for APIs.
	SkipInterstitial bool

	// LogoURL is the logo URL.
	LogoURL string

	// AppLauncherVisible shows in App Launcher.
	AppLauncherVisible bool

	// CustomDenyMessage is the denial message.
	CustomDenyMessage string

	// CustomDenyURL is the denial redirect.
	CustomDenyURL string
}

// UpdateApplicationParams contains parameters for updating an application.
type UpdateApplicationParams struct {
	// Name is the application display name.
	Name string

	// Domain is the protected domain.
	Domain string

	// SessionDuration is the session lifetime.
	SessionDuration string

	// AllowedIdps is the list of allowed identity provider IDs.
	AllowedIdps []string

	// AutoRedirectToIdentity auto-redirects if single IdP.
	AutoRedirectToIdentity bool

	// EnableBindingCookie enables sticky sessions.
	EnableBindingCookie bool

	// HttpOnlyCookieAttribute sets HttpOnly flag.
	HttpOnlyCookieAttribute *bool

	// SameSiteCookieAttribute sets SameSite attribute.
	SameSiteCookieAttribute string

	// SkipInterstitial skips login page for APIs.
	SkipInterstitial bool

	// LogoURL is the logo URL.
	LogoURL string

	// AppLauncherVisible shows in App Launcher.
	AppLauncherVisible bool

	// CustomDenyMessage is the denial message.
	CustomDenyMessage string

	// CustomDenyURL is the denial redirect.
	CustomDenyURL string
}

// AccessPolicy represents a Cloudflare Access Policy.
type AccessPolicy struct {
	// ID is the unique policy identifier.
	ID string

	// Name is the policy display name.
	Name string

	// Decision is the policy action (allow, deny, bypass, non_identity).
	Decision string

	// Precedence is the evaluation order (lower = first).
	Precedence int

	// Include are rules that must match (ANY).
	Include []AccessRuleParam

	// Exclude are rules that exclude (ANY).
	Exclude []AccessRuleParam

	// Require are rules that must match (ALL).
	Require []AccessRuleParam

	// SessionDuration overrides application session duration.
	SessionDuration string

	// PurposeJustificationRequired requires purpose justification.
	PurposeJustificationRequired bool

	// PurposeJustificationPrompt is the justification prompt text.
	PurposeJustificationPrompt string

	// ApprovalRequired requires manager approval.
	ApprovalRequired bool

	// ApprovalGroups is the approval configuration.
	ApprovalGroups []ApprovalGroupParam

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time
}

// CreatePolicyParams contains parameters for creating a policy.
type CreatePolicyParams struct {
	// Name is the policy display name.
	Name string

	// Decision is the policy action.
	Decision string

	// Precedence is the evaluation order.
	Precedence int

	// Include are rules that must match (ANY).
	Include []AccessRuleParam

	// Exclude are rules that exclude (ANY).
	Exclude []AccessRuleParam

	// Require are rules that must match (ALL).
	Require []AccessRuleParam

	// SessionDuration overrides application session duration.
	SessionDuration string

	// PurposeJustificationRequired requires purpose justification.
	PurposeJustificationRequired bool

	// PurposeJustificationPrompt is the justification prompt text.
	PurposeJustificationPrompt string

	// ApprovalRequired requires manager approval.
	ApprovalRequired bool

	// ApprovalGroups is the approval configuration.
	ApprovalGroups []ApprovalGroupParam
}

// UpdatePolicyParams contains parameters for updating a policy.
type UpdatePolicyParams struct {
	// Name is the policy display name.
	Name string

	// Decision is the policy action.
	Decision string

	// Precedence is the evaluation order.
	Precedence int

	// Include are rules that must match (ANY).
	Include []AccessRuleParam

	// Exclude are rules that exclude (ANY).
	Exclude []AccessRuleParam

	// Require are rules that must match (ALL).
	Require []AccessRuleParam

	// SessionDuration overrides application session duration.
	SessionDuration string

	// PurposeJustificationRequired requires purpose justification.
	PurposeJustificationRequired bool

	// PurposeJustificationPrompt is the justification prompt text.
	PurposeJustificationPrompt string

	// ApprovalRequired requires manager approval.
	ApprovalRequired bool

	// ApprovalGroups is the approval configuration.
	ApprovalGroups []ApprovalGroupParam
}

// AccessRuleParam represents an access rule parameter.
// Only one field should be set per rule.
type AccessRuleParam struct {
	// ============================================================
	// P0: No IdP required (always testable)
	// ============================================================

	// IPRange matches an IP range (CIDR notation).
	IPRange *string

	// IPListID matches IPs from a Cloudflare IP List.
	IPListID *string

	// Country matches a country code (ISO 3166-1 alpha-2).
	Country *string

	// Everyone matches everyone (set to true).
	Everyone *bool

	// ServiceTokenID matches a specific service token.
	ServiceTokenID *string

	// AnyValidServiceToken matches any valid service token.
	AnyValidServiceToken *bool

	// ============================================================
	// P1: Basic IdP (Google Workspace)
	// ============================================================

	// Email matches a specific email address.
	Email *string

	// EmailListID matches emails from a Cloudflare Access list.
	EmailListID *string

	// EmailDomain matches an email domain.
	EmailDomain *string

	// OIDCClaim matches an OIDC token claim.
	OIDCClaim *OIDCClaimParam

	// ============================================================
	// P2: Google Workspace Groups
	// ============================================================

	// GSuiteGroup matches Google Workspace group membership.
	GSuiteGroup *GSuiteGroupParam

	// ============================================================
	// P3: v0.2.0 (not implemented in alpha.3)
	// ============================================================

	// Certificate requires a valid client certificate (set to true).
	Certificate *bool

	// CommonName matches certificate common name.
	CommonName *string

	// GroupID references an Access Group.
	GroupID *string
}

// OIDCClaimParam represents an OIDC claim rule parameter.
type OIDCClaimParam struct {
	IdentityProviderID string
	ClaimName          string
	ClaimValue         string
}

// GSuiteGroupParam represents a Google Workspace group rule parameter.
type GSuiteGroupParam struct {
	IdentityProviderID string
	Email              string // Group email address
}

// ApprovalGroupParam represents an approval configuration.
type ApprovalGroupParam struct {
	EmailAddresses  []string
	EmailListUUID   string
	ApprovalsNeeded int
}

// AccessGroup represents a Cloudflare Access Group.
type AccessGroup struct {
	// ID is the unique group identifier.
	ID string

	// Name is the group display name.
	Name string

	// Include are rules that must match (ANY).
	Include []AccessRuleParam

	// Exclude are rules that exclude (ANY).
	Exclude []AccessRuleParam

	// Require are rules that must match (ALL).
	Require []AccessRuleParam

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time
}

// CreateGroupParams contains parameters for creating a group.
type CreateGroupParams struct {
	// Name is the group display name.
	Name string

	// Include are rules that must match (ANY).
	Include []AccessRuleParam

	// Exclude are rules that exclude (ANY).
	Exclude []AccessRuleParam

	// Require are rules that must match (ALL).
	Require []AccessRuleParam
}

// UpdateGroupParams contains parameters for updating a group.
type UpdateGroupParams struct {
	// Name is the group display name.
	Name string

	// Include are rules that must match (ANY).
	Include []AccessRuleParam

	// Exclude are rules that exclude (ANY).
	Exclude []AccessRuleParam

	// Require are rules that must match (ALL).
	Require []AccessRuleParam
}

// ServiceToken represents a Cloudflare Access Service Token.
type ServiceToken struct {
	// ID is the unique token identifier.
	ID string

	// Name is the token display name.
	Name string

	// ClientID is the Client ID (CF-Access-Client-Id header).
	ClientID string

	// Duration is the token validity period.
	Duration string

	// ExpiresAt is the expiration timestamp.
	ExpiresAt time.Time
}

// ServiceTokenWithSecret includes the secret, returned only on create/rotate.
type ServiceTokenWithSecret struct {
	ServiceToken

	// ClientSecret is the Client Secret (CF-Access-Client-Secret header).
	// Only returned on create or rotate operations.
	ClientSecret string
}

// CreateServiceTokenParams contains parameters for creating a service token.
type CreateServiceTokenParams struct {
	// Name is the token display name.
	Name string

	// Duration is the token validity period in hours (e.g., "8760h" for 1 year).
	Duration string
}

// UpdateServiceTokenParams contains parameters for updating a service token.
type UpdateServiceTokenParams struct {
	// Name is the token display name.
	Name string

	// Duration is the token validity period.
	Duration string
}

// MTLSCertificate represents a Cloudflare mTLS Certificate.
type MTLSCertificate struct {
	// ID is the unique certificate identifier.
	ID string

	// Name is the certificate display name.
	Name string

	// Fingerprint is the SHA-256 fingerprint.
	Fingerprint string

	// AssociatedHostnames are hostnames using this certificate.
	AssociatedHostnames []string

	// ExpiresOn is the certificate expiration.
	ExpiresOn time.Time
}

// CreateCertificateParams contains parameters for creating a certificate.
type CreateCertificateParams struct {
	// Name is the certificate display name.
	Name string

	// Certificate is the PEM-encoded certificate.
	Certificate string

	// AssociatedHostnames are hostnames using this certificate.
	AssociatedHostnames []string
}

// UpdateCertificateParams contains parameters for updating a certificate.
type UpdateCertificateParams struct {
	// Name is the certificate display name.
	Name string

	// AssociatedHostnames are hostnames using this certificate.
	AssociatedHostnames []string
}

// CertificateSettings represents mTLS certificate settings.
type CertificateSettings struct {
	// Hostname is the hostname for mTLS.
	Hostname string

	// ChinaNetwork enables China network mTLS.
	ChinaNetwork bool

	// ClientCertificateForwarding forwards client cert to origin.
	ClientCertificateForwarding bool
}

// CertificateSettingsParam is used for updating certificate settings.
type CertificateSettingsParam struct {
	// Hostname is the hostname for mTLS.
	Hostname string

	// ChinaNetwork enables China network mTLS.
	ChinaNetwork bool

	// ClientCertificateForwarding forwards client cert to origin.
	ClientCertificateForwarding bool
}

// EnsureApplication ensures an application exists with the given configuration.
// If an application with the domain exists, it is adopted. Otherwise, a new application is created.
// Returns the application and whether it was created (vs adopted).
func (s *AccessService) EnsureApplication(ctx context.Context, accountID string, params CreateApplicationParams) (*AccessApplication, bool, error) {
	s.log.Info("ensuring access application exists",
		"accountID", accountID,
		"domain", params.Domain,
	)

	// Try to find existing application by name
	existing, err := s.client.GetAccessApplicationByName(ctx, accountID, params.Name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check for existing application: %w", err)
	}

	// Application exists, adopt it
	if existing != nil {
		s.log.V(1).Info("access application already exists, adopting",
			"applicationId", existing.ID,
			"domain", existing.Domain,
		)
		return existing, false, nil
	}

	// Create new application
	s.log.Info("creating new access application",
		"accountID", accountID,
		"domain", params.Domain,
		"name", params.Name,
	)

	app, err := s.client.CreateAccessApplication(ctx, accountID, params)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create application: %w", err)
	}

	return app, true, nil
}

// SyncPolicies synchronizes access policies for an application.
// It deletes policies not in the desired set, updates existing policies if different,
// and creates new policies for additions.
// Returns the policy IDs after sync.
func (s *AccessService) SyncPolicies(ctx context.Context, accountID, appID string, desired []CreatePolicyParams) ([]string, error) {
	// List existing policies
	existing, err := s.client.ListAccessPolicies(ctx, accountID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing policies: %w", err)
	}

	s.log.Info("syncing access policies",
		"applicationId", appID,
		"desiredCount", len(desired),
		"existingCount", len(existing),
	)

	// Build maps for comparison
	existingByName := make(map[string]*AccessPolicy)
	for i := range existing {
		existingByName[existing[i].Name] = &existing[i]
	}

	desiredByName := make(map[string]CreatePolicyParams)
	for _, p := range desired {
		desiredByName[p.Name] = p
	}

	var resultIDs []string
	var toCreate, toUpdate, toDelete int

	// Delete policies not in desired set
	for name, policy := range existingByName {
		if _, found := desiredByName[name]; !found {
			toDelete++
			s.log.V(1).Info("policy operation",
				"applicationId", appID,
				"policyName", name,
				"operation", "delete",
			)
			if err := s.client.DeleteAccessPolicy(ctx, accountID, appID, policy.ID); err != nil {
				return nil, fmt.Errorf("failed to delete policy %s: %w", name, err)
			}
		}
	}

	// Create or update policies
	for name, params := range desiredByName {
		if existingPolicy, found := existingByName[name]; found {
			// Update if different
			if !policiesEqual(existingPolicy, &params) {
				toUpdate++
				s.log.V(1).Info("policy operation",
					"applicationId", appID,
					"policyName", name,
					"operation", "update",
				)
				updated, err := s.client.UpdateAccessPolicy(ctx, accountID, appID, existingPolicy.ID, UpdatePolicyParams(params))
				if err != nil {
					return nil, fmt.Errorf("failed to update policy %s: %w", name, err)
				}
				resultIDs = append(resultIDs, updated.ID)
			} else {
				resultIDs = append(resultIDs, existingPolicy.ID)
			}
		} else {
			// Create new policy
			toCreate++
			s.log.V(1).Info("policy operation",
				"applicationId", appID,
				"policyName", name,
				"operation", "create",
			)
			created, err := s.client.CreateAccessPolicy(ctx, accountID, appID, params)
			if err != nil {
				return nil, fmt.Errorf("failed to create policy %s: %w", name, err)
			}
			resultIDs = append(resultIDs, created.ID)
		}
	}

	s.log.Info("access policies synced",
		"applicationId", appID,
		"toCreate", toCreate,
		"toUpdate", toUpdate,
		"toDelete", toDelete,
	)

	return resultIDs, nil
}

// policiesEqual checks if a policy matches the desired params.
func policiesEqual(existing *AccessPolicy, desired *CreatePolicyParams) bool {
	if existing.Name != desired.Name {
		return false
	}
	if existing.Decision != desired.Decision {
		return false
	}
	if existing.Precedence != desired.Precedence {
		return false
	}
	// For full equality checking, we would need to compare rules deeply
	// For now, we use a simplified check
	return len(existing.Include) == len(desired.Include) &&
		len(existing.Exclude) == len(desired.Exclude) &&
		len(existing.Require) == len(desired.Require)
}

// EnsureServiceToken ensures a service token exists with the given configuration.
// If a token with the name exists and is not expired, it is returned (no secret available).
// If expired, the token is rotated and the new secret is stored.
// If not exists, a new token is created and the secret is stored.
func (s *AccessService) EnsureServiceToken(ctx context.Context, accountID string, params CreateServiceTokenParams, secretWriter SecretWriter) (*ServiceToken, error) {
	s.log.Info("ensuring service token exists",
		"accountID", accountID,
		"tokenName", params.Name,
	)

	// Try to find existing token by name
	tokens, err := s.client.ListServiceTokens(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list service tokens: %w", err)
	}

	var existing *ServiceToken
	for i := range tokens {
		if tokens[i].Name == params.Name {
			existing = &tokens[i]
			break
		}
	}

	if existing != nil {
		// Check if expired
		if time.Now().After(existing.ExpiresAt) {
			s.log.Info("service token expired, rotating",
				"tokenId", existing.ID,
				"tokenName", existing.Name,
				"expiredAt", existing.ExpiresAt,
			)

			rotated, err := s.client.RotateServiceToken(ctx, accountID, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to rotate service token: %w", err)
			}

			// Store the new secret
			if secretWriter != nil {
				if err := secretWriter.WriteSecret(ctx, params.Name, map[string][]byte{
					"CF_ACCESS_CLIENT_ID":     []byte(rotated.ClientID),
					"CF_ACCESS_CLIENT_SECRET": []byte(rotated.ClientSecret),
				}); err != nil {
					return nil, fmt.Errorf("failed to store service token secret: %w", err)
				}
				s.log.Info("service token rotated, secret stored",
					"tokenId", rotated.ID,
					"tokenName", rotated.Name,
					"expiresAt", rotated.ExpiresAt,
				)
			}

			return &rotated.ServiceToken, nil
		}

		s.log.V(1).Info("service token already exists",
			"tokenId", existing.ID,
			"tokenName", existing.Name,
			"expiresAt", existing.ExpiresAt,
		)
		return existing, nil
	}

	// Create new token
	s.log.Info("creating new service token",
		"accountID", accountID,
		"tokenName", params.Name,
	)

	created, err := s.client.CreateServiceToken(ctx, accountID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create service token: %w", err)
	}

	// Store the secret
	if secretWriter != nil {
		if err := secretWriter.WriteSecret(ctx, params.Name, map[string][]byte{
			"CF_ACCESS_CLIENT_ID":     []byte(created.ClientID),
			"CF_ACCESS_CLIENT_SECRET": []byte(created.ClientSecret),
		}); err != nil {
			return nil, fmt.Errorf("failed to store service token secret: %w", err)
		}
		s.log.Info("service token created, secret stored",
			"tokenId", created.ID,
			"tokenName", created.Name,
			"expiresAt", created.ExpiresAt,
		)
	}

	return &created.ServiceToken, nil
}

// EnsureMTLSCertificate ensures an mTLS certificate exists with the given configuration.
// If a certificate with the name or fingerprint exists, it is returned.
// Otherwise, a new certificate is created.
func (s *AccessService) EnsureMTLSCertificate(ctx context.Context, accountID string, params CreateCertificateParams) (*MTLSCertificate, bool, error) {
	s.log.Info("ensuring mTLS certificate exists",
		"accountID", accountID,
		"certificateName", params.Name,
	)

	// List existing certificates
	certs, err := s.client.ListMTLSCertificates(ctx, accountID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list mTLS certificates: %w", err)
	}

	// Find by name
	for i := range certs {
		if certs[i].Name == params.Name {
			s.log.V(1).Info("mTLS certificate already exists",
				"certificateId", certs[i].ID,
				"certificateName", certs[i].Name,
			)
			return &certs[i], false, nil
		}
	}

	// Create new certificate
	s.log.Info("creating new mTLS certificate",
		"accountID", accountID,
		"certificateName", params.Name,
	)

	cert, err := s.client.CreateMTLSCertificate(ctx, accountID, params)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create mTLS certificate: %w", err)
	}

	s.log.Info("mTLS certificate created",
		"certificateId", cert.ID,
		"certificateName", cert.Name,
		"fingerprint", cert.Fingerprint,
	)

	return cert, true, nil
}

// UpdateMTLSHostnames updates mTLS hostname associations.
func (s *AccessService) UpdateMTLSHostnames(ctx context.Context, accountID string, hostnames []string, enableForwarding bool) error {
	settings := make([]CertificateSettingsParam, len(hostnames))
	for i, hostname := range hostnames {
		settings[i] = CertificateSettingsParam{
			Hostname:                    hostname,
			ClientCertificateForwarding: enableForwarding,
		}
	}

	s.log.Info("updating mTLS hostname settings",
		"accountID", accountID,
		"hostnames", hostnames,
		"enableForwarding", enableForwarding,
	)

	_, err := s.client.UpdateMTLSCertificateSettings(ctx, accountID, settings)
	if err != nil {
		return fmt.Errorf("failed to update mTLS certificate settings: %w", err)
	}

	return nil
}

// DeleteApplication deletes an application and all associated resources.
func (s *AccessService) DeleteApplication(ctx context.Context, accountID, appID string) error {
	s.log.Info("deleting access application",
		"applicationId", appID,
	)

	// Delete the application (policies are deleted automatically)
	if err := s.client.DeleteAccessApplication(ctx, accountID, appID); err != nil {
		return fmt.Errorf("failed to delete access application: %w", err)
	}

	return nil
}

// Client returns the underlying Cloudflare client.
// Used for direct API operations not wrapped by AccessService.
func (s *AccessService) Client() Client {
	return s.client
}
