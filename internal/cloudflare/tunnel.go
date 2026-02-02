// Package cloudflare provides a wrapper around cloudflare-go for cfgate's needs.
package cloudflare

import (
	"context"
	"fmt"
)

// TunnelService handles tunnel-specific operations.
// It wraps the cloudflare-go client with cfgate-specific logic.
type TunnelService struct {
	// client is the underlying Cloudflare client.
	client Client
}

// NewTunnelService creates a new TunnelService.
func NewTunnelService(client Client) *TunnelService {
	return &TunnelService{
		client: client,
	}
}

// EnsureTunnel ensures a tunnel exists with the given name.
// If a tunnel with the name exists, it is adopted. Otherwise, a new tunnel is created.
// Returns the tunnel and whether it was created (vs adopted).
func (s *TunnelService) EnsureTunnel(ctx context.Context, accountID, name string) (*Tunnel, bool, error) {
	// Try to find existing tunnel by name
	existing, err := s.client.GetTunnelByName(ctx, accountID, name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check for existing tunnel: %w", err)
	}

	// Tunnel exists, adopt it
	if existing != nil {
		return existing, false, nil
	}

	// Create new tunnel with remote management
	tunnel, err := s.client.CreateTunnel(ctx, accountID, CreateTunnelParams{
		Name:      name,
		ConfigSrc: "cloudflare", // Remote management
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to create tunnel: %w", err)
	}

	return tunnel, true, nil
}

// GetToken retrieves the tunnel token for cloudflared authentication.
func (s *TunnelService) GetToken(ctx context.Context, accountID, tunnelID string) (string, error) {
	token, err := s.client.GetTunnelToken(ctx, accountID, tunnelID)
	if err != nil {
		return "", fmt.Errorf("failed to get tunnel token: %w", err)
	}

	return token, nil
}

// UpdateConfiguration updates the tunnel's ingress configuration.
// It performs an atomic replacement of the entire configuration.
func (s *TunnelService) UpdateConfiguration(ctx context.Context, accountID, tunnelID string, config TunnelConfiguration) error {
	// Ensure catch-all rule exists
	config = ensureCatchAllRule(config)

	err := s.client.UpdateTunnelConfiguration(ctx, accountID, tunnelID, config)
	if err != nil {
		return fmt.Errorf("failed to update tunnel configuration: %w", err)
	}

	return nil
}

// Delete deletes a tunnel and all its connections.
// It first deletes all connections, then deletes the tunnel.
func (s *TunnelService) Delete(ctx context.Context, accountID, tunnelID string) error {
	// First, delete all connections
	err := s.client.DeleteTunnelConnections(ctx, accountID, tunnelID)
	if err != nil {
		return fmt.Errorf("failed to delete tunnel connections: %w", err)
	}

	// Then delete the tunnel
	err = s.client.DeleteTunnel(ctx, accountID, tunnelID)
	if err != nil {
		return fmt.Errorf("failed to delete tunnel: %w", err)
	}

	return nil
}

// IsHealthy checks if a tunnel has healthy connections.
// Health is determined by the tunnel's status field, which reflects connection state.
func (s *TunnelService) IsHealthy(ctx context.Context, accountID, tunnelID string) (bool, error) {
	tunnel, err := s.client.GetTunnel(ctx, accountID, tunnelID)
	if err != nil {
		return false, fmt.Errorf("failed to get tunnel: %w", err)
	}

	if tunnel == nil {
		return false, nil
	}

	return tunnel.Status == "healthy" || tunnel.Status == "active", nil
}

// BuildConfiguration builds a TunnelConfiguration from ingress rules.
// It adds the required catch-all rule if not present.
func BuildConfiguration(rules []IngressRule, defaults *OriginRequestConfig) TunnelConfiguration {
	config := TunnelConfiguration{
		Ingress:       rules,
		OriginRequest: defaults,
	}

	return ensureCatchAllRule(config)
}

// ensureCatchAllRule ensures the configuration has a catch-all rule at the end.
func ensureCatchAllRule(config TunnelConfiguration) TunnelConfiguration {
	if len(config.Ingress) == 0 {
		// Add a catch-all rule
		config.Ingress = append(config.Ingress, IngressRule{
			Service: "http_status:404",
		})
		return config
	}

	// Check if last rule is a catch-all (no hostname)
	lastRule := config.Ingress[len(config.Ingress)-1]
	if lastRule.Hostname == "" && lastRule.Path == "" {
		// Already has catch-all
		return config
	}

	// Add catch-all rule at the end
	config.Ingress = append(config.Ingress, IngressRule{
		Service: "http_status:404",
	})

	return config
}

// TunnelDomain returns the CNAME target domain for a tunnel.
func TunnelDomain(tunnelID string) string {
	return fmt.Sprintf("%s.cfargotunnel.com", tunnelID)
}
