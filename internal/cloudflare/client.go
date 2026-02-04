// Package cloudflare provides a wrapper around cloudflare-go for cfgate's needs.
// It abstracts Cloudflare API operations for tunnels, DNS, and zones.
package cloudflare

import (
	"context"
	"fmt"
)

// Client wraps cloudflare-go for cfgate's needs.
// It provides a high-level interface for tunnel and DNS operations.
type Client interface {
	// Tunnel operations

	// GetTunnel retrieves a tunnel by ID.
	GetTunnel(ctx context.Context, accountID, tunnelID string) (*Tunnel, error)

	// GetTunnelByName retrieves a tunnel by name.
	// Returns nil if the tunnel does not exist.
	GetTunnelByName(ctx context.Context, accountID, name string) (*Tunnel, error)

	// CreateTunnel creates a new tunnel with the given name.
	// Uses config_src: "cloudflare" for remote management.
	CreateTunnel(ctx context.Context, accountID string, params CreateTunnelParams) (*Tunnel, error)

	// DeleteTunnel deletes a tunnel by ID.
	// Requires all connections to be deleted first.
	DeleteTunnel(ctx context.Context, accountID, tunnelID string) error

	// DeleteTunnelConnections deletes all active connections for a tunnel.
	// Must be called before DeleteTunnel.
	DeleteTunnelConnections(ctx context.Context, accountID, tunnelID string) error

	// GetTunnelToken retrieves the tunnel token for cloudflared authentication.
	GetTunnelToken(ctx context.Context, accountID, tunnelID string) (string, error)

	// UpdateTunnelConfiguration updates the tunnel's ingress configuration.
	// This is an atomic replacement of the entire configuration.
	UpdateTunnelConfiguration(ctx context.Context, accountID, tunnelID string, config TunnelConfiguration) error

	// DNS operations

	// ListDNSRecords lists all DNS records in a zone.
	ListDNSRecords(ctx context.Context, zoneID string) ([]DNSRecord, error)

	// CreateDNSRecord creates a new DNS record.
	CreateDNSRecord(ctx context.Context, zoneID string, record DNSRecord) (*DNSRecord, error)

	// UpdateDNSRecord updates an existing DNS record.
	UpdateDNSRecord(ctx context.Context, zoneID, recordID string, record DNSRecord) (*DNSRecord, error)

	// DeleteDNSRecord deletes a DNS record.
	DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error

	// Zone operations

	// ListZones lists all zones accessible with the current credentials.
	ListZones(ctx context.Context) ([]Zone, error)

	// GetZoneByName retrieves a zone by domain name.
	// Returns nil if the zone does not exist.
	GetZoneByName(ctx context.Context, name string) (*Zone, error)

	// Token validation

	// ValidateToken verifies the API token by attempting actual operations.
	// Uses operational validation (ListZones + ListTunnels) to work with both
	// User API Tokens and Account API Tokens.
	// Returns an error if the token is invalid or missing permissions.
	ValidateToken(ctx context.Context, accountID string) error

	// Account operations

	// ListAccounts lists all accounts accessible with the current credentials.
	ListAccounts(ctx context.Context) ([]Account, error)

	// GetAccountByName retrieves an account by name.
	// Returns nil if the account does not exist.
	GetAccountByName(ctx context.Context, name string) (*Account, error)

	// Access Application operations

	// CreateAccessApplication creates a new Access application.
	CreateAccessApplication(ctx context.Context, accountID string, params CreateApplicationParams) (*AccessApplication, error)

	// GetAccessApplication retrieves an Access application by ID.
	GetAccessApplication(ctx context.Context, accountID, appID string) (*AccessApplication, error)

	// UpdateAccessApplication updates an existing Access application.
	UpdateAccessApplication(ctx context.Context, accountID, appID string, params UpdateApplicationParams) (*AccessApplication, error)

	// DeleteAccessApplication deletes an Access application.
	DeleteAccessApplication(ctx context.Context, accountID, appID string) error

	// ListAccessApplications lists all Access applications.
	ListAccessApplications(ctx context.Context, accountID string) ([]AccessApplication, error)

	// GetAccessApplicationByName retrieves an Access application by name.
	// Returns nil if the application does not exist.
	GetAccessApplicationByName(ctx context.Context, accountID, name string) (*AccessApplication, error)

	// Access Policy operations (application-scoped)

	// CreateAccessPolicy creates a new Access policy for an application.
	CreateAccessPolicy(ctx context.Context, accountID, appID string, params CreatePolicyParams) (*AccessPolicy, error)

	// GetAccessPolicy retrieves an Access policy by ID.
	GetAccessPolicy(ctx context.Context, accountID, appID, policyID string) (*AccessPolicy, error)

	// UpdateAccessPolicy updates an existing Access policy.
	UpdateAccessPolicy(ctx context.Context, accountID, appID, policyID string, params UpdatePolicyParams) (*AccessPolicy, error)

	// DeleteAccessPolicy deletes an Access policy.
	DeleteAccessPolicy(ctx context.Context, accountID, appID, policyID string) error

	// ListAccessPolicies lists all Access policies for an application.
	ListAccessPolicies(ctx context.Context, accountID, appID string) ([]AccessPolicy, error)

	// Access Group operations

	// CreateAccessGroup creates a new Access group.
	CreateAccessGroup(ctx context.Context, accountID string, params CreateGroupParams) (*AccessGroup, error)

	// GetAccessGroup retrieves an Access group by ID.
	GetAccessGroup(ctx context.Context, accountID, groupID string) (*AccessGroup, error)

	// UpdateAccessGroup updates an existing Access group.
	UpdateAccessGroup(ctx context.Context, accountID, groupID string, params UpdateGroupParams) (*AccessGroup, error)

	// DeleteAccessGroup deletes an Access group.
	DeleteAccessGroup(ctx context.Context, accountID, groupID string) error

	// ListAccessGroups lists all Access groups.
	ListAccessGroups(ctx context.Context, accountID string) ([]AccessGroup, error)

	// GetAccessGroupByName retrieves an Access group by name.
	// Returns nil if the group does not exist.
	GetAccessGroupByName(ctx context.Context, accountID, name string) (*AccessGroup, error)

	// Service Token operations

	// CreateServiceToken creates a new service token.
	// The returned token includes the client secret, which is only available at creation time.
	CreateServiceToken(ctx context.Context, accountID string, params CreateServiceTokenParams) (*ServiceTokenWithSecret, error)

	// GetServiceToken retrieves a service token by ID.
	GetServiceToken(ctx context.Context, accountID, tokenID string) (*ServiceToken, error)

	// UpdateServiceToken updates an existing service token.
	UpdateServiceToken(ctx context.Context, accountID, tokenID string, params UpdateServiceTokenParams) (*ServiceToken, error)

	// DeleteServiceToken deletes a service token.
	DeleteServiceToken(ctx context.Context, accountID, tokenID string) error

	// ListServiceTokens lists all service tokens.
	ListServiceTokens(ctx context.Context, accountID string) ([]ServiceToken, error)

	// RotateServiceToken rotates a service token.
	// The returned token includes the new client secret.
	RotateServiceToken(ctx context.Context, accountID, tokenID string) (*ServiceTokenWithSecret, error)

	// RefreshServiceToken refreshes a service token's expiration.
	RefreshServiceToken(ctx context.Context, accountID, tokenID string) (*ServiceToken, error)

	// mTLS Certificate operations

	// CreateMTLSCertificate creates a new mTLS certificate.
	CreateMTLSCertificate(ctx context.Context, accountID string, params CreateCertificateParams) (*MTLSCertificate, error)

	// GetMTLSCertificate retrieves an mTLS certificate by ID.
	GetMTLSCertificate(ctx context.Context, accountID, certID string) (*MTLSCertificate, error)

	// UpdateMTLSCertificate updates an existing mTLS certificate.
	UpdateMTLSCertificate(ctx context.Context, accountID, certID string, params UpdateCertificateParams) (*MTLSCertificate, error)

	// DeleteMTLSCertificate deletes an mTLS certificate.
	DeleteMTLSCertificate(ctx context.Context, accountID, certID string) error

	// ListMTLSCertificates lists all mTLS certificates.
	ListMTLSCertificates(ctx context.Context, accountID string) ([]MTLSCertificate, error)

	// mTLS Certificate Settings (hostname association)

	// GetMTLSCertificateSettings retrieves mTLS certificate settings.
	GetMTLSCertificateSettings(ctx context.Context, accountID string) ([]CertificateSettings, error)

	// UpdateMTLSCertificateSettings updates mTLS certificate settings.
	UpdateMTLSCertificateSettings(ctx context.Context, accountID string, settings []CertificateSettingsParam) ([]CertificateSettings, error)
}

// Tunnel represents a Cloudflare Tunnel.
type Tunnel struct {
	// ID is the unique tunnel identifier.
	ID string

	// Name is the tunnel name.
	Name string

	// Status is the tunnel status (healthy, inactive, etc.).
	Status string

	// AccountTag is the account ID owning the tunnel.
	AccountTag string

	// CreatedAt is the tunnel creation timestamp.
	CreatedAt string
}

// CreateTunnelParams contains parameters for creating a tunnel.
type CreateTunnelParams struct {
	// Name is the tunnel name.
	Name string

	// ConfigSrc should be "cloudflare" for remote management.
	ConfigSrc string
}

// TunnelConfiguration represents the tunnel's ingress configuration.
type TunnelConfiguration struct {
	// Ingress is the list of ingress rules.
	Ingress []IngressRule

	// OriginRequest contains default origin settings.
	OriginRequest *OriginRequestConfig

	// WarpRouting enables WARP routing.
	WarpRouting *WarpRoutingConfig
}

// IngressRule represents a single ingress rule.
type IngressRule struct {
	// Hostname is the hostname to match (optional for catch-all).
	Hostname string `json:"hostname,omitempty"`

	// Path is the path regex to match (optional).
	Path string `json:"path,omitempty"`

	// Service is the origin service URL.
	Service string `json:"service"`

	// OriginRequest contains per-rule origin settings.
	OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`
}

// OriginRequestConfig contains origin connection settings.
type OriginRequestConfig struct {
	// ConnectTimeout is the connection timeout.
	ConnectTimeout string `json:"connectTimeout,omitempty"`

	// TLSTimeout is the TLS handshake timeout.
	TLSTimeout string `json:"tlsTimeout,omitempty"`

	// TCPKeepAlive is the TCP keepalive interval.
	TCPKeepAlive string `json:"tcpKeepAlive,omitempty"`

	// NoHappyEyeballs disables Happy Eyeballs algorithm.
	NoHappyEyeballs bool `json:"noHappyEyeballs,omitempty"`

	// KeepAliveConnections is the max idle connections.
	KeepAliveConnections int `json:"keepAliveConnections,omitempty"`

	// KeepAliveTimeout is the idle connection timeout.
	KeepAliveTimeout string `json:"keepAliveTimeout,omitempty"`

	// HTTPHostHeader overrides the Host header.
	HTTPHostHeader string `json:"httpHostHeader,omitempty"`

	// OriginServerName is the TLS SNI hostname.
	OriginServerName string `json:"originServerName,omitempty"`

	// CAPool is the path to CA certificates.
	CAPool string `json:"caPool,omitempty"`

	// NoTLSVerify disables TLS verification.
	NoTLSVerify bool `json:"noTLSVerify,omitempty"`

	// DisableChunkedEncoding disables chunked encoding.
	DisableChunkedEncoding bool `json:"disableChunkedEncoding,omitempty"`

	// BastionMode enables bastion mode.
	BastionMode bool `json:"bastionMode,omitempty"`

	// ProxyAddress is the proxy address.
	ProxyAddress string `json:"proxyAddress,omitempty"`

	// ProxyPort is the proxy port.
	ProxyPort int `json:"proxyPort,omitempty"`

	// ProxyType is the proxy type (socks, "").
	ProxyType string `json:"proxyType,omitempty"`

	// IPRules contains IP-based access rules.
	IPRules []IPRule `json:"ipRules,omitempty"`

	// HTTP2Origin enables HTTP/2 to origin.
	HTTP2Origin bool `json:"http2Origin,omitempty"`

	// MatchSNIToHost passes SNI matching hostname to origin.
	MatchSNIToHost bool `json:"matchSniToHost,omitempty"`

	// Access contains Access policy settings.
	Access *AccessConfig `json:"access,omitempty"`
}

// IPRule represents an IP-based access rule.
type IPRule struct {
	Prefix string `json:"prefix"`
	Ports  []int  `json:"ports"`
	Allow  bool   `json:"allow"`
}

// AccessConfig contains Cloudflare Access settings.
type AccessConfig struct {
	Required bool     `json:"required"`
	TeamName string   `json:"teamName"`
	AudTag   []string `json:"audTag"`
}

// WarpRoutingConfig contains WARP routing settings.
type WarpRoutingConfig struct {
	Enabled bool `json:"enabled"`
}

// DNSRecord represents a Cloudflare DNS record.
type DNSRecord struct {
	// ID is the record ID.
	ID string

	// Type is the record type (CNAME, A, etc.).
	Type string

	// Name is the record name (hostname).
	Name string

	// Content is the record content (target for CNAME).
	Content string

	// TTL is the record TTL (1 = auto).
	TTL int

	// Proxied indicates if the record is proxied through Cloudflare.
	Proxied bool

	// Comment is the record comment (used for ownership tracking).
	Comment string

	// ZoneID is the zone ID containing this record.
	ZoneID string

	// ZoneName is the zone name.
	ZoneName string
}

// Zone represents a Cloudflare DNS zone.
type Zone struct {
	// ID is the zone ID.
	ID string

	// Name is the zone name (domain).
	Name string

	// Status is the zone status.
	Status string

	// AccountID is the account ID owning the zone.
	AccountID string
}

// Account represents a Cloudflare account.
type Account struct {
	// ID is the account ID.
	ID string

	// Name is the account name.
	Name string
}

// APIError represents a Cloudflare API error.
type APIError struct {
	// Code is the error code.
	Code int

	// Message is the error message.
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("cloudflare API error (code %d): %s", e.Code, e.Message)
}
