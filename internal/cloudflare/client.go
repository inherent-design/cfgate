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
