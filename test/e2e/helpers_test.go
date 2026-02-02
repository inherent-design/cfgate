// Package e2e contains end-to-end tests for cfgate.
package e2e_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	cloudflare "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zero_trust"
	"github.com/cloudflare/cloudflare-go/v6/zones"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Timeouts for E2E tests.
const (
	// DefaultTimeout is the default timeout for waiting operations.
	DefaultTimeout = 5 * time.Minute

	// DefaultInterval is the default polling interval.
	DefaultInterval = 5 * time.Second

	// ShortTimeout is for quick operations.
	ShortTimeout = 30 * time.Second

	// LongTimeout is for operations that may take longer.
	LongTimeout = 10 * time.Minute
)

// getCloudflareClient creates a real Cloudflare client from environment variables.
func getCloudflareClient() *cloudflare.Client {
	client := cloudflare.NewClient(
		option.WithAPIToken(testEnv.CloudflareAPIToken),
	)
	return client
}

// generateUniqueName generates a unique name for test resources.
func generateUniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1000000)
}

// CloudflareTunnelInfo holds information about a Cloudflare tunnel.
type CloudflareTunnelInfo struct {
	ID     string
	Name   string
	Status string
}

// getTunnelFromCloudflare fetches a tunnel directly from the Cloudflare API.
func getTunnelFromCloudflare(ctx context.Context, cfClient *cloudflare.Client, accountID, tunnelName string) (*CloudflareTunnelInfo, error) {
	// List tunnels and find by name.
	tunnels, err := cfClient.ZeroTrust.Tunnels.Cloudflared.List(ctx, zero_trust.TunnelCloudflaredListParams{
		AccountID: cloudflare.F(accountID),
		Name:      cloudflare.F(tunnelName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tunnels: %w", err)
	}

	// Find the tunnel with matching name.
	for _, tunnel := range tunnels.Result {
		if tunnel.Name == tunnelName {
			return &CloudflareTunnelInfo{
				ID:     tunnel.ID,
				Name:   tunnel.Name,
				Status: string(tunnel.Status),
			}, nil
		}
	}

	return nil, nil // Not found.
}

// getTunnelByIDFromCloudflare fetches a tunnel by ID from the Cloudflare API.
func getTunnelByIDFromCloudflare(ctx context.Context, cfClient *cloudflare.Client, accountID, tunnelID string) (*CloudflareTunnelInfo, error) {
	tunnel, err := cfClient.ZeroTrust.Tunnels.Cloudflared.Get(ctx, tunnelID, zero_trust.TunnelCloudflaredGetParams{
		AccountID: cloudflare.F(accountID),
	})
	if err != nil {
		// Check if tunnel not found.
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tunnel: %w", err)
	}

	return &CloudflareTunnelInfo{
		ID:     tunnel.ID,
		Name:   tunnel.Name,
		Status: string(tunnel.Status),
	}, nil
}

// createTunnelInCloudflare creates a tunnel directly via the Cloudflare API.
func createTunnelInCloudflare(ctx context.Context, cfClient *cloudflare.Client, accountID, tunnelName string) (*CloudflareTunnelInfo, error) {
	// Generate a random tunnel secret (base64 encoded 32 bytes).
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate tunnel secret: %w", err)
	}
	tunnelSecret := base64.StdEncoding.EncodeToString(secretBytes)

	tunnel, err := cfClient.ZeroTrust.Tunnels.Cloudflared.New(ctx, zero_trust.TunnelCloudflaredNewParams{
		AccountID:    cloudflare.F(accountID),
		Name:         cloudflare.F(tunnelName),
		TunnelSecret: cloudflare.F(tunnelSecret),
		ConfigSrc:    cloudflare.F(zero_trust.TunnelCloudflaredNewParamsConfigSrcCloudflare),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel: %w", err)
	}

	return &CloudflareTunnelInfo{
		ID:     tunnel.ID,
		Name:   tunnel.Name,
		Status: string(tunnel.Status),
	}, nil
}

// waitForTunnelReady waits for a CloudflareTunnel to have Ready=True condition.
func waitForTunnelReady(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) *cfgatev1alpha1.CloudflareTunnel {
	var tunnel cfgatev1alpha1.CloudflareTunnel

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &tunnel)
		if err != nil {
			return false
		}
		for _, cond := range tunnel.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, DefaultInterval).Should(BeTrue(), "Tunnel did not become ready")

	return &tunnel
}

// waitForTunnelCondition waits for a specific condition on a CloudflareTunnel.
func waitForTunnelCondition(ctx context.Context, k8sClient client.Client, name, namespace, conditionType string, status metav1.ConditionStatus, timeout time.Duration) *cfgatev1alpha1.CloudflareTunnel {
	var tunnel cfgatev1alpha1.CloudflareTunnel

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &tunnel)
		if err != nil {
			return false
		}
		for _, cond := range tunnel.Status.Conditions {
			if cond.Type == conditionType && cond.Status == status {
				return true
			}
		}
		return false
	}, timeout, DefaultInterval).Should(BeTrue(), fmt.Sprintf("Tunnel condition %s did not become %s", conditionType, status))

	return &tunnel
}

// waitForDeploymentReady waits for a Deployment to have the expected ready replicas.
func waitForDeploymentReady(ctx context.Context, k8sClient client.Client, name, namespace string, replicas int32, timeout time.Duration) *appsv1.Deployment {
	var deployment appsv1.Deployment

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &deployment)
		if err != nil {
			return false
		}
		return deployment.Status.ReadyReplicas >= replicas
	}, timeout, DefaultInterval).Should(BeTrue(), "Deployment did not become ready")

	return &deployment
}

// waitForTunnelDeleted waits for a CloudflareTunnel to be deleted from Kubernetes.
func waitForTunnelDeleted(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) {
	Eventually(func() bool {
		var tunnel cfgatev1alpha1.CloudflareTunnel
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &tunnel)
		return client.IgnoreNotFound(err) == nil && err != nil
	}, timeout, DefaultInterval).Should(BeTrue(), "Tunnel was not deleted")
}

// waitForTunnelDeletedFromCloudflare waits for a tunnel to be deleted from Cloudflare.
func waitForTunnelDeletedFromCloudflare(ctx context.Context, cfClient *cloudflare.Client, accountID, tunnelName string, timeout time.Duration) {
	Eventually(func() bool {
		tunnel, err := getTunnelFromCloudflare(ctx, cfClient, accountID, tunnelName)
		if err != nil {
			return false
		}
		return tunnel == nil
	}, timeout, DefaultInterval).Should(BeTrue(), "Tunnel was not deleted from Cloudflare")
}

// CloudflareDNSRecordInfo holds information about a DNS record.
type CloudflareDNSRecordInfo struct {
	ID      string
	Type    string
	Name    string
	Content string
	Proxied bool
	TTL     float64
}

// getDNSRecordFromCloudflare fetches a DNS record from Cloudflare.
func getDNSRecordFromCloudflare(ctx context.Context, cfClient *cloudflare.Client, zoneID, hostname, recordType string) (*CloudflareDNSRecordInfo, error) {
	records, err := cfClient.DNS.Records.List(ctx, dns.RecordListParams{
		ZoneID: cloudflare.F(zoneID),
		Name:   cloudflare.F(dns.RecordListParamsName{Exact: cloudflare.F(hostname)}),
		Type:   cloudflare.F(dns.RecordListParamsType(recordType)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}

	for _, record := range records.Result {
		if record.Name == hostname {
			return &CloudflareDNSRecordInfo{
				ID:      record.ID,
				Type:    string(record.Type),
				Name:    record.Name,
				Content: record.Content,
				Proxied: record.Proxied,
				TTL:     float64(record.TTL),
			}, nil
		}
	}

	return nil, nil // Not found.
}

// getZoneIDByName gets the zone ID for a zone name.
func getZoneIDByName(ctx context.Context, cfClient *cloudflare.Client, zoneName string) (string, error) {
	zoneList, err := cfClient.Zones.List(ctx, zones.ZoneListParams{
		Name: cloudflare.F(zoneName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list zones: %w", err)
	}

	for _, zone := range zoneList.Result {
		if zone.Name == zoneName {
			return zone.ID, nil
		}
	}

	return "", fmt.Errorf("zone %s not found", zoneName)
}

// waitForDNSSyncReady waits for a CloudflareDNSSync to have Ready=True condition.
func waitForDNSSyncReady(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) *cfgatev1alpha1.CloudflareDNSSync {
	var dnsSync cfgatev1alpha1.CloudflareDNSSync

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &dnsSync)
		if err != nil {
			return false
		}
		for _, cond := range dnsSync.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, DefaultInterval).Should(BeTrue(), "DNSSync did not become ready")

	return &dnsSync
}

// createGatewayClass creates a GatewayClass for testing.
func createGatewayClass(ctx context.Context, k8sClient client.Client, name string) *gatewayv1.GatewayClass {
	gc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: "cfgate.io/cloudflare-tunnel-controller",
		},
	}
	Expect(k8sClient.Create(ctx, gc)).To(Succeed())
	return gc
}

// createGateway creates a Gateway for testing.
func createGateway(ctx context.Context, k8sClient client.Client, name, namespace, gatewayClassName, tunnelRef string) *gatewayv1.Gateway {
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"cfgate.io/tunnel-ref": tunnelRef,
				"cfgate.io/dns-sync":   "enabled",
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
			Listeners: []gatewayv1.Listener{
				{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, gw)).To(Succeed())
	return gw
}

// createHTTPRoute creates an HTTPRoute for testing.
func createHTTPRoute(ctx context.Context, k8sClient client.Client, name, namespace, gatewayName string, hostnames []string, serviceName string, servicePort int32) *gatewayv1.HTTPRoute {
	parentNS := gatewayv1.Namespace(namespace)

	hr := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"cfgate.io/dns-sync": "enabled",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:      gatewayv1.ObjectName(gatewayName),
						Namespace: &parentNS,
					},
				},
			},
			Hostnames: make([]gatewayv1.Hostname, len(hostnames)),
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(serviceName),
									Port: (*gatewayv1.PortNumber)(&servicePort),
								},
							},
						},
					},
				},
			},
		},
	}

	for i, h := range hostnames {
		hr.Spec.Hostnames[i] = gatewayv1.Hostname(h)
	}

	Expect(k8sClient.Create(ctx, hr)).To(Succeed())
	return hr
}

// createTestService creates a simple Service for testing.
func createTestService(ctx context.Context, k8sClient client.Client, name, namespace string, port int32) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: port,
				},
			},
			Selector: map[string]string{
				"app": name,
			},
		},
	}
	Expect(k8sClient.Create(ctx, svc)).To(Succeed())
	return svc
}

// createCloudflareTunnel creates a CloudflareTunnel CR for testing.
func createCloudflareTunnel(ctx context.Context, k8sClient client.Client, name, namespace, tunnelName string) *cfgatev1alpha1.CloudflareTunnel {
	tunnel := &cfgatev1alpha1.CloudflareTunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareTunnelSpec{
			Tunnel: cfgatev1alpha1.TunnelIdentity{
				Name: tunnelName,
			},
			Cloudflare: cfgatev1alpha1.CloudflareConfig{
				AccountID: testEnv.CloudflareAccountID,
				SecretRef: cfgatev1alpha1.SecretRef{
					Name: "cloudflare-credentials",
				},
			},
			Cloudflared: cfgatev1alpha1.CloudflaredConfig{
				Replicas: 1,
			},
		},
	}
	Expect(k8sClient.Create(ctx, tunnel)).To(Succeed())
	return tunnel
}

// createCloudflareTunnelWithInvalidToken creates a CloudflareTunnel with invalid credentials.
func createCloudflareTunnelWithInvalidToken(ctx context.Context, k8sClient client.Client, name, namespace, tunnelName string) *cfgatev1alpha1.CloudflareTunnel {
	// First create a secret with invalid token.
	invalidSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloudflare-invalid-credentials",
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"CLOUDFLARE_API_TOKEN": "invalid-token-for-testing",
		},
	}
	Expect(k8sClient.Create(ctx, invalidSecret)).To(Succeed())

	tunnel := &cfgatev1alpha1.CloudflareTunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareTunnelSpec{
			Tunnel: cfgatev1alpha1.TunnelIdentity{
				Name: tunnelName,
			},
			Cloudflare: cfgatev1alpha1.CloudflareConfig{
				AccountID: testEnv.CloudflareAccountID,
				SecretRef: cfgatev1alpha1.SecretRef{
					Name: "cloudflare-invalid-credentials",
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, tunnel)).To(Succeed())
	return tunnel
}

// createCloudflareDNSSync creates a CloudflareDNSSync CR for testing.
func createCloudflareDNSSync(ctx context.Context, k8sClient client.Client, name, namespace, tunnelRefName string) *cfgatev1alpha1.CloudflareDNSSync {
	dnsSync := &cfgatev1alpha1.CloudflareDNSSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareDNSSyncSpec{
			TunnelRef: cfgatev1alpha1.TunnelRef{
				Name: tunnelRefName,
			},
			Zones: []cfgatev1alpha1.ZoneConfig{
				{
					Name: testEnv.CloudflareZoneName,
				},
			},
			Source: cfgatev1alpha1.HostnameSource{
				GatewayRoutes: cfgatev1alpha1.GatewayRoutesSource{
					Enabled: true,
				},
			},
			Defaults: cfgatev1alpha1.RecordDefaults{
				Proxied: true,
			},
			Ownership: cfgatev1alpha1.OwnershipConfig{
				TXTRecord: cfgatev1alpha1.TXTRecordOwnership{
					Enabled: true,
					Prefix:  "_cfgate",
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, dnsSync)).To(Succeed())
	return dnsSync
}
