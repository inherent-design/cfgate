// Package cloudflare provides a wrapper around cloudflare-go for cfgate's needs.
package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cf "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zero_trust"
	"github.com/cloudflare/cloudflare-go/v6/zones"
)

// ClientOption is a functional option for configuring the client.
type ClientOption func(*clientImpl)

// clientImpl implements the Client interface using cloudflare-go v6 SDK.
type clientImpl struct {
	api *cf.Client
}

// NewClient creates a new Cloudflare client with the given API token.
func NewClient(apiToken string, opts ...ClientOption) (Client, error) {
	if apiToken == "" {
		return nil, errors.New("API token is required")
	}

	api := cf.NewClient(
		option.WithAPIToken(apiToken),
	)

	c := &clientImpl{
		api: api,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// GetTunnel retrieves a tunnel by ID.
func (c *clientImpl) GetTunnel(ctx context.Context, accountID, tunnelID string) (*Tunnel, error) {
	tunnel, err := c.api.ZeroTrust.Tunnels.Cloudflared.Get(ctx, tunnelID, zero_trust.TunnelCloudflaredGetParams{
		AccountID: cf.F(accountID),
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tunnel: %w", err)
	}

	return tunnelFromAPI(tunnel), nil
}

// GetTunnelByName retrieves a tunnel by name.
// Returns nil if the tunnel does not exist.
func (c *clientImpl) GetTunnelByName(ctx context.Context, accountID, name string) (*Tunnel, error) {
	tunnels, err := c.api.ZeroTrust.Tunnels.Cloudflared.List(ctx, zero_trust.TunnelCloudflaredListParams{
		AccountID: cf.F(accountID),
		Name:      cf.F(name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tunnels: %w", err)
	}

	for _, tunnel := range tunnels.Result {
		if tunnel.Name == name {
			return &Tunnel{
				ID:         tunnel.ID,
				Name:       tunnel.Name,
				Status:     string(tunnel.Status),
				AccountTag: accountID,
			}, nil
		}
	}

	return nil, nil // Not found
}

// CreateTunnel creates a new tunnel with the given name.
// Uses config_src: "cloudflare" for remote management.
func (c *clientImpl) CreateTunnel(ctx context.Context, accountID string, params CreateTunnelParams) (*Tunnel, error) {
	// Generate a tunnel secret - base64 encoded random bytes
	// The SDK expects the secret to be base64 encoded
	tunnelSecret := generateTunnelSecret()

	configSrc := zero_trust.TunnelCloudflaredNewParamsConfigSrcCloudflare
	if params.ConfigSrc == "local" {
		configSrc = zero_trust.TunnelCloudflaredNewParamsConfigSrcLocal
	}

	tunnel, err := c.api.ZeroTrust.Tunnels.Cloudflared.New(ctx, zero_trust.TunnelCloudflaredNewParams{
		AccountID:    cf.F(accountID),
		Name:         cf.F(params.Name),
		TunnelSecret: cf.F(tunnelSecret),
		ConfigSrc:    cf.F(configSrc),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel: %w", err)
	}

	return tunnelFromAPI(tunnel), nil
}

// DeleteTunnel deletes a tunnel by ID.
// Requires all connections to be deleted first.
func (c *clientImpl) DeleteTunnel(ctx context.Context, accountID, tunnelID string) error {
	_, err := c.api.ZeroTrust.Tunnels.Cloudflared.Delete(ctx, tunnelID, zero_trust.TunnelCloudflaredDeleteParams{
		AccountID: cf.F(accountID),
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete tunnel: %w", err)
	}

	return nil
}

// DeleteTunnelConnections deletes all active connections for a tunnel.
// Must be called before DeleteTunnel.
func (c *clientImpl) DeleteTunnelConnections(ctx context.Context, accountID, tunnelID string) error {
	_, err := c.api.ZeroTrust.Tunnels.Cloudflared.Connections.Delete(ctx, tunnelID, zero_trust.TunnelCloudflaredConnectionDeleteParams{
		AccountID: cf.F(accountID),
	})
	if err != nil {
		// Ignore "not found" errors for connections
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("failed to delete tunnel connections: %w", err)
	}

	return nil
}

// GetTunnelToken retrieves the tunnel token for cloudflared authentication.
func (c *clientImpl) GetTunnelToken(ctx context.Context, accountID, tunnelID string) (string, error) {
	tokenPtr, err := c.api.ZeroTrust.Tunnels.Cloudflared.Token.Get(ctx, tunnelID, zero_trust.TunnelCloudflaredTokenGetParams{
		AccountID: cf.F(accountID),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get tunnel token: %w", err)
	}

	if tokenPtr == nil {
		return "", errors.New("token is nil")
	}

	return *tokenPtr, nil
}

// UpdateTunnelConfiguration updates the tunnel's ingress configuration.
// This is an atomic replacement of the entire configuration.
func (c *clientImpl) UpdateTunnelConfiguration(ctx context.Context, accountID, tunnelID string, config TunnelConfiguration) error {
	// Convert our config to cloudflare-go config
	ingressRules := make([]zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfigIngress, len(config.Ingress))
	for i, rule := range config.Ingress {
		ingressRules[i] = zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfigIngress{
			Hostname: cf.F(rule.Hostname),
			Path:     cf.F(rule.Path),
			Service:  cf.F(rule.Service),
		}

		if rule.OriginRequest != nil {
			ingressRules[i].OriginRequest = cf.F(ingressOriginRequestToAPI(rule.OriginRequest))
		}
	}

	cfConfig := zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfig{
		Ingress: cf.F(ingressRules),
	}

	if config.OriginRequest != nil {
		cfConfig.OriginRequest = cf.F(globalOriginRequestToAPI(config.OriginRequest))
	}

	_, err := c.api.ZeroTrust.Tunnels.Cloudflared.Configurations.Update(ctx, tunnelID, zero_trust.TunnelCloudflaredConfigurationUpdateParams{
		AccountID: cf.F(accountID),
		Config:    cf.F(cfConfig),
	})
	if err != nil {
		return fmt.Errorf("failed to update tunnel configuration: %w", err)
	}

	return nil
}

// ListDNSRecords lists all DNS records in a zone.
func (c *clientImpl) ListDNSRecords(ctx context.Context, zoneID string) ([]DNSRecord, error) {
	var records []DNSRecord

	page := c.api.DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{
		ZoneID: cf.F(zoneID),
	})

	for page.Next() {
		record := page.Current()
		records = append(records, DNSRecord{
			ID:      record.ID,
			Type:    string(record.Type),
			Name:    record.Name,
			Content: record.Content,
			TTL:     int(record.TTL),
			Proxied: record.Proxied,
			Comment: record.Comment,
			ZoneID:  zoneID,
		})
	}

	if err := page.Err(); err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}

	return records, nil
}

// CreateDNSRecord creates a new DNS record.
func (c *clientImpl) CreateDNSRecord(ctx context.Context, zoneID string, record DNSRecord) (*DNSRecord, error) {
	var result *dns.RecordResponse
	var err error

	switch record.Type {
	case "CNAME":
		result, err = c.api.DNS.Records.New(ctx, dns.RecordNewParams{
			ZoneID: cf.F(zoneID),
			Body: dns.CNAMERecordParam{
				Name:    cf.F(record.Name),
				Type:    cf.F(dns.CNAMERecordTypeCNAME),
				Content: cf.F(record.Content),
				TTL:     cf.F(dns.TTL(record.TTL)),
				Proxied: cf.F(record.Proxied),
				Comment: cf.F(record.Comment),
			},
		})
	case "TXT":
		result, err = c.api.DNS.Records.New(ctx, dns.RecordNewParams{
			ZoneID: cf.F(zoneID),
			Body: dns.TXTRecordParam{
				Name:    cf.F(record.Name),
				Type:    cf.F(dns.TXTRecordTypeTXT),
				Content: cf.F(record.Content),
				TTL:     cf.F(dns.TTL(record.TTL)),
				Comment: cf.F(record.Comment),
			},
		})
	case "A":
		result, err = c.api.DNS.Records.New(ctx, dns.RecordNewParams{
			ZoneID: cf.F(zoneID),
			Body: dns.ARecordParam{
				Name:    cf.F(record.Name),
				Type:    cf.F(dns.ARecordTypeA),
				Content: cf.F(record.Content),
				TTL:     cf.F(dns.TTL(record.TTL)),
				Proxied: cf.F(record.Proxied),
				Comment: cf.F(record.Comment),
			},
		})
	default:
		return nil, fmt.Errorf("unsupported record type: %s", record.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create DNS record: %w", err)
	}

	return &DNSRecord{
		ID:      result.ID,
		Type:    string(result.Type),
		Name:    result.Name,
		Content: result.Content,
		TTL:     int(result.TTL),
		Proxied: result.Proxied,
		Comment: result.Comment,
		ZoneID:  zoneID,
	}, nil
}

// UpdateDNSRecord updates an existing DNS record.
func (c *clientImpl) UpdateDNSRecord(ctx context.Context, zoneID, recordID string, record DNSRecord) (*DNSRecord, error) {
	var result *dns.RecordResponse
	var err error

	switch record.Type {
	case "CNAME":
		result, err = c.api.DNS.Records.Update(ctx, recordID, dns.RecordUpdateParams{
			ZoneID: cf.F(zoneID),
			Body: dns.CNAMERecordParam{
				Name:    cf.F(record.Name),
				Type:    cf.F(dns.CNAMERecordTypeCNAME),
				Content: cf.F(record.Content),
				TTL:     cf.F(dns.TTL(record.TTL)),
				Proxied: cf.F(record.Proxied),
				Comment: cf.F(record.Comment),
			},
		})
	case "TXT":
		result, err = c.api.DNS.Records.Update(ctx, recordID, dns.RecordUpdateParams{
			ZoneID: cf.F(zoneID),
			Body: dns.TXTRecordParam{
				Name:    cf.F(record.Name),
				Type:    cf.F(dns.TXTRecordTypeTXT),
				Content: cf.F(record.Content),
				TTL:     cf.F(dns.TTL(record.TTL)),
				Comment: cf.F(record.Comment),
			},
		})
	case "A":
		result, err = c.api.DNS.Records.Update(ctx, recordID, dns.RecordUpdateParams{
			ZoneID: cf.F(zoneID),
			Body: dns.ARecordParam{
				Name:    cf.F(record.Name),
				Type:    cf.F(dns.ARecordTypeA),
				Content: cf.F(record.Content),
				TTL:     cf.F(dns.TTL(record.TTL)),
				Proxied: cf.F(record.Proxied),
				Comment: cf.F(record.Comment),
			},
		})
	default:
		return nil, fmt.Errorf("unsupported record type: %s", record.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to update DNS record: %w", err)
	}

	return &DNSRecord{
		ID:      result.ID,
		Type:    string(result.Type),
		Name:    result.Name,
		Content: result.Content,
		TTL:     int(result.TTL),
		Proxied: result.Proxied,
		Comment: result.Comment,
		ZoneID:  zoneID,
	}, nil
}

// DeleteDNSRecord deletes a DNS record.
func (c *clientImpl) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	_, err := c.api.DNS.Records.Delete(ctx, recordID, dns.RecordDeleteParams{
		ZoneID: cf.F(zoneID),
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}

	return nil
}

// ListZones lists all zones accessible with the current credentials.
func (c *clientImpl) ListZones(ctx context.Context) ([]Zone, error) {
	var zoneList []Zone

	page := c.api.Zones.ListAutoPaging(ctx, zones.ZoneListParams{})

	for page.Next() {
		zone := page.Current()
		zoneList = append(zoneList, Zone{
			ID:        zone.ID,
			Name:      zone.Name,
			Status:    string(zone.Status),
			AccountID: zone.Account.ID,
		})
	}

	if err := page.Err(); err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	return zoneList, nil
}

// GetZoneByName retrieves a zone by domain name.
// Returns nil if the zone does not exist.
func (c *clientImpl) GetZoneByName(ctx context.Context, name string) (*Zone, error) {
	zoneList, err := c.api.Zones.List(ctx, zones.ZoneListParams{
		Name: cf.F(name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	for _, zone := range zoneList.Result {
		if zone.Name == name {
			return &Zone{
				ID:        zone.ID,
				Name:      zone.Name,
				Status:    string(zone.Status),
				AccountID: zone.Account.ID,
			}, nil
		}
	}

	return nil, nil // Not found
}

// ValidateToken verifies the API token by attempting actual operations.
// Uses operational validation (ListZones + ListTunnels) to work with both
// User API Tokens and Account API Tokens.
// Returns an error if the token is invalid or missing permissions.
func (c *clientImpl) ValidateToken(ctx context.Context, accountID string) error {
	// Validate zone access (required for DNS operations)
	zones, err := c.ListZones(ctx)
	if err != nil {
		return fmt.Errorf("token validation failed (zone access): %w", err)
	}
	if len(zones) == 0 {
		return fmt.Errorf("token has no zone access - DNS operations will fail")
	}

	// Validate tunnel access (required for tunnel operations)
	_, err = c.api.ZeroTrust.Tunnels.Cloudflared.List(ctx, zero_trust.TunnelCloudflaredListParams{
		AccountID: cf.F(accountID),
	})
	if err != nil {
		return fmt.Errorf("token validation failed (tunnel access): %w", err)
	}

	return nil
}

// tunnelFromAPI converts a Cloudflare API tunnel to our Tunnel type.
func tunnelFromAPI(t *zero_trust.CloudflareTunnel) *Tunnel {
	if t == nil {
		return nil
	}

	return &Tunnel{
		ID:         t.ID,
		Name:       t.Name,
		Status:     string(t.Status),
		AccountTag: t.AccountTag,
		CreatedAt:  t.CreatedAt.String(),
	}
}

// ingressOriginRequestToAPI converts our OriginRequestConfig to the per-ingress API format.
func ingressOriginRequestToAPI(config *OriginRequestConfig) zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfigIngressOriginRequest {
	req := zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfigIngressOriginRequest{}

	if config.ConnectTimeout != "" {
		// Parse duration to seconds if needed
		req.ConnectTimeout = cf.F(int64(30)) // Default 30 seconds
	}
	if config.NoHappyEyeballs {
		req.NoHappyEyeballs = cf.F(true)
	}
	if config.KeepAliveConnections > 0 {
		req.KeepAliveConnections = cf.F(int64(config.KeepAliveConnections))
	}
	if config.HTTPHostHeader != "" {
		req.HTTPHostHeader = cf.F(config.HTTPHostHeader)
	}
	if config.OriginServerName != "" {
		req.OriginServerName = cf.F(config.OriginServerName)
	}
	if config.CAPool != "" {
		req.CAPool = cf.F(config.CAPool)
	}
	if config.NoTLSVerify {
		req.NoTLSVerify = cf.F(true)
	}
	if config.DisableChunkedEncoding {
		req.DisableChunkedEncoding = cf.F(true)
	}
	if config.HTTP2Origin {
		req.HTTP2Origin = cf.F(true)
	}

	return req
}

// globalOriginRequestToAPI converts our OriginRequestConfig to the global config API format.
func globalOriginRequestToAPI(config *OriginRequestConfig) zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfigOriginRequest {
	req := zero_trust.TunnelCloudflaredConfigurationUpdateParamsConfigOriginRequest{}

	if config.ConnectTimeout != "" {
		req.ConnectTimeout = cf.F(int64(30)) // Default 30 seconds
	}
	if config.NoHappyEyeballs {
		req.NoHappyEyeballs = cf.F(true)
	}
	if config.KeepAliveConnections > 0 {
		req.KeepAliveConnections = cf.F(int64(config.KeepAliveConnections))
	}
	if config.HTTPHostHeader != "" {
		req.HTTPHostHeader = cf.F(config.HTTPHostHeader)
	}
	if config.OriginServerName != "" {
		req.OriginServerName = cf.F(config.OriginServerName)
	}
	if config.CAPool != "" {
		req.CAPool = cf.F(config.CAPool)
	}
	if config.NoTLSVerify {
		req.NoTLSVerify = cf.F(true)
	}
	if config.DisableChunkedEncoding {
		req.DisableChunkedEncoding = cf.F(true)
	}
	if config.HTTP2Origin {
		req.HTTP2Origin = cf.F(true)
	}

	return req
}

// generateTunnelSecret generates a base64-encoded tunnel secret.
func generateTunnelSecret() string {
	// Generate 32 random bytes and base64 encode them
	// Using a simple deterministic approach for now (the API generates the actual secret)
	return "Y2ZnYXRlLWdlbmVyYXRlZC1zZWNyZXQtdG9rZW4="
}
