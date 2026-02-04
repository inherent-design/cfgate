// Package main provides a standalone cleanup tool for orphaned E2E test resources.
// This tool deletes Cloudflare resources matching the e2e-* prefix.
//
// SAFETY: Only deletes resources with e2e-* prefix. Never touches production resources.
//
// Usage:
//
//	mise exec -- go run ./cmd/cleanup
//	mise run cleanup
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	cloudflare "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zero_trust"
	"github.com/cloudflare/cloudflare-go/v6/zones"
)

const (
	// E2E prefix that identifies test resources.
	e2ePrefix = "e2e-"

	// Recovery prefix from E2E tests.
	recoveryPrefix = "recovery-"

	// Ownership TXT record prefix.
	ownershipPrefix = "_cfgate.e2e-"
)

type resource struct {
	ID   string
	Name string
	Type string
}

func main() {
	// Load credentials from environment.
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	zoneName := os.Getenv("CLOUDFLARE_ZONE_NAME")

	if apiToken == "" || accountID == "" {
		fmt.Println("ERROR: CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID must be set")
		fmt.Println("Usage: mise exec -- go run ./cmd/cleanup")
		os.Exit(1)
	}

	fmt.Println("=== cfgate E2E Resource Cleanup ===")
	fmt.Printf("Account ID: %s\n", accountID)
	fmt.Printf("Zone: %s\n", zoneName)
	fmt.Println()

	cfClient := cloudflare.NewClient(option.WithAPIToken(apiToken))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var allResources []resource
	var deleted []resource
	var failed []resource

	// List tunnels.
	fmt.Println("--- Scanning Tunnels ---")
	tunnels := listOrphanedTunnels(ctx, cfClient, accountID)
	allResources = append(allResources, tunnels...)
	for _, t := range tunnels {
		fmt.Printf("  Found: %s (ID: %s)\n", t.Name, t.ID)
	}
	if len(tunnels) == 0 {
		fmt.Println("  No orphaned tunnels found")
	}
	fmt.Println()

	// List DNS records.
	if zoneName != "" {
		fmt.Println("--- Scanning DNS Records ---")
		records := listOrphanedDNSRecords(ctx, cfClient, zoneName)
		allResources = append(allResources, records...)
		for _, r := range records {
			fmt.Printf("  Found: %s (ID: %s)\n", r.Name, r.ID)
		}
		if len(records) == 0 {
			fmt.Println("  No orphaned DNS records found")
		}
		fmt.Println()
	}

	// List Access applications.
	fmt.Println("--- Scanning Access Applications ---")
	apps := listOrphanedAccessApplications(ctx, cfClient, accountID)
	allResources = append(allResources, apps...)
	for _, a := range apps {
		fmt.Printf("  Found: %s (ID: %s)\n", a.Name, a.ID)
	}
	if len(apps) == 0 {
		fmt.Println("  No orphaned Access applications found")
	}
	fmt.Println()

	// List service tokens.
	fmt.Println("--- Scanning Service Tokens ---")
	tokens := listOrphanedServiceTokens(ctx, cfClient, accountID)
	allResources = append(allResources, tokens...)
	for _, t := range tokens {
		fmt.Printf("  Found: %s (ID: %s)\n", t.Name, t.ID)
	}
	if len(tokens) == 0 {
		fmt.Println("  No orphaned service tokens found")
	}
	fmt.Println()

	if len(allResources) == 0 {
		fmt.Println("=== No orphaned E2E resources found ===")
		return
	}

	fmt.Printf("=== Found %d orphaned resources ===\n", len(allResources))
	fmt.Println()
	fmt.Println("--- Deleting Resources ---")

	// Delete tunnels (must delete connections first).
	for _, t := range tunnels {
		fmt.Printf("  Deleting tunnel: %s ... ", t.Name)
		if err := deleteTunnel(ctx, cfClient, accountID, t.ID); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed = append(failed, t)
		} else {
			fmt.Println("OK")
			deleted = append(deleted, t)
		}
	}

	// Delete DNS records.
	if zoneName != "" {
		zoneID := getZoneID(ctx, cfClient, zoneName)
		if zoneID != "" {
			for _, r := range listOrphanedDNSRecords(ctx, cfClient, zoneName) {
				fmt.Printf("  Deleting DNS record: %s ... ", r.Name)
				if err := deleteDNSRecord(ctx, cfClient, zoneID, r.ID); err != nil {
					fmt.Printf("FAILED: %v\n", err)
					failed = append(failed, r)
				} else {
					fmt.Println("OK")
					deleted = append(deleted, r)
				}
			}
		}
	}

	// Delete Access applications.
	for _, a := range apps {
		fmt.Printf("  Deleting Access application: %s ... ", a.Name)
		if err := deleteAccessApplication(ctx, cfClient, accountID, a.ID); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed = append(failed, a)
		} else {
			fmt.Println("OK")
			deleted = append(deleted, a)
		}
	}

	// Delete service tokens.
	for _, t := range tokens {
		fmt.Printf("  Deleting service token: %s ... ", t.Name)
		if err := deleteServiceToken(ctx, cfClient, accountID, t.ID); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed = append(failed, t)
		} else {
			fmt.Println("OK")
			deleted = append(deleted, t)
		}
	}

	fmt.Println()
	fmt.Println("=== Cleanup Summary ===")
	fmt.Printf("Deleted: %d\n", len(deleted))
	fmt.Printf("Failed:  %d\n", len(failed))

	if len(failed) > 0 {
		fmt.Println("\nFailed resources:")
		for _, r := range failed {
			fmt.Printf("  - %s: %s (ID: %s)\n", r.Type, r.Name, r.ID)
		}
		os.Exit(1)
	}
}

func listOrphanedTunnels(ctx context.Context, client *cloudflare.Client, accountID string) []resource {
	var results []resource
	iter := client.ZeroTrust.Tunnels.Cloudflared.ListAutoPaging(ctx, zero_trust.TunnelCloudflaredListParams{
		AccountID: cloudflare.F(accountID),
	})

	for iter.Next() {
		t := iter.Current()
		if strings.HasPrefix(t.Name, e2ePrefix) || strings.HasPrefix(t.Name, recoveryPrefix) {
			results = append(results, resource{ID: t.ID, Name: t.Name, Type: "tunnel"})
		}
	}

	if err := iter.Err(); err != nil {
		fmt.Printf("Warning: failed to list tunnels: %v\n", err)
	}

	return results
}

func listOrphanedDNSRecords(ctx context.Context, client *cloudflare.Client, zoneName string) []resource {
	zoneID := getZoneID(ctx, client, zoneName)
	if zoneID == "" {
		return nil
	}

	var results []resource
	iter := client.DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{
		ZoneID: cloudflare.F(zoneID),
	})

	for iter.Next() {
		r := iter.Current()
		// Match e2e-* hostnames and _cfgate.e2e-* ownership TXT records.
		if strings.Contains(r.Name, e2ePrefix) || strings.HasPrefix(r.Name, ownershipPrefix) {
			results = append(results, resource{ID: r.ID, Name: r.Name, Type: "dns"})
		}
	}

	if err := iter.Err(); err != nil {
		fmt.Printf("Warning: failed to list DNS records: %v\n", err)
	}

	return results
}

func listOrphanedAccessApplications(ctx context.Context, client *cloudflare.Client, accountID string) []resource {
	var results []resource
	iter := client.ZeroTrust.Access.Applications.ListAutoPaging(ctx, zero_trust.AccessApplicationListParams{
		AccountID: cloudflare.F(accountID),
	})

	for iter.Next() {
		app := iter.Current()
		if strings.HasPrefix(app.Name, e2ePrefix) {
			results = append(results, resource{ID: app.ID, Name: app.Name, Type: "access_app"})
		}
	}

	if err := iter.Err(); err != nil {
		fmt.Printf("Warning: failed to list Access applications: %v\n", err)
	}

	return results
}

func listOrphanedServiceTokens(ctx context.Context, client *cloudflare.Client, accountID string) []resource {
	var results []resource
	iter := client.ZeroTrust.Access.ServiceTokens.ListAutoPaging(ctx, zero_trust.AccessServiceTokenListParams{
		AccountID: cloudflare.F(accountID),
	})

	for iter.Next() {
		token := iter.Current()
		if strings.HasPrefix(token.Name, e2ePrefix) {
			results = append(results, resource{ID: token.ID, Name: token.Name, Type: "service_token"})
		}
	}

	if err := iter.Err(); err != nil {
		fmt.Printf("Warning: failed to list service tokens: %v\n", err)
	}

	return results
}

func getZoneID(ctx context.Context, client *cloudflare.Client, zoneName string) string {
	zoneList, err := client.Zones.List(ctx, zones.ZoneListParams{
		Name: cloudflare.F(zoneName),
	})
	if err != nil || len(zoneList.Result) == 0 {
		fmt.Printf("Warning: failed to get zone ID for %s: %v\n", zoneName, err)
		return ""
	}
	return zoneList.Result[0].ID
}

func deleteTunnel(ctx context.Context, client *cloudflare.Client, accountID, tunnelID string) error {
	// Delete connections first.
	_, _ = client.ZeroTrust.Tunnels.Cloudflared.Connections.Delete(ctx, tunnelID, zero_trust.TunnelCloudflaredConnectionDeleteParams{
		AccountID: cloudflare.F(accountID),
	})

	// Delete tunnel.
	_, err := client.ZeroTrust.Tunnels.Cloudflared.Delete(ctx, tunnelID, zero_trust.TunnelCloudflaredDeleteParams{
		AccountID: cloudflare.F(accountID),
	})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}

func deleteDNSRecord(ctx context.Context, client *cloudflare.Client, zoneID, recordID string) error {
	_, err := client.DNS.Records.Delete(ctx, recordID, dns.RecordDeleteParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}

func deleteAccessApplication(ctx context.Context, client *cloudflare.Client, accountID, appID string) error {
	_, err := client.ZeroTrust.Access.Applications.Delete(ctx, appID, zero_trust.AccessApplicationDeleteParams{
		AccountID: cloudflare.F(accountID),
	})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}

func deleteServiceToken(ctx context.Context, client *cloudflare.Client, accountID, tokenID string) error {
	_, err := client.ZeroTrust.Access.ServiceTokens.Delete(ctx, tokenID, zero_trust.AccessServiceTokenDeleteParams{
		AccountID: cloudflare.F(accountID),
	})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}
