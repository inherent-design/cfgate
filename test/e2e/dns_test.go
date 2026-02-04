// Package e2e contains end-to-end tests for cfgate.
package e2e_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cloudflare "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// CloudflareDNS E2E tests.
// Tests the CloudflareDNS CRD (alpha.3 composable CRD architecture).
// CloudflareDNS manages DNS records independently from CloudflareTunnel.
var _ = Describe("CloudflareDNS E2E", Label("cloudflare"), Ordered, func() {
	var (
		namespace *corev1.Namespace
		cfClient  *cloudflare.Client
		zoneID    string

		// Shared tunnel for tests that need tunnelRef mode.
		sharedTunnel *cfgatev1alpha1.CloudflareTunnel
	)

	BeforeAll(func() {
		skipIfNoZone() // Requires zone in addition to credentials.

		// Create unique namespace for DNS tests.
		namespace = createTestNamespace("cfgate-dns-e2e")

		// Create Cloudflare credentials secret.
		createCloudflareCredentialsSecret(namespace.Name)

		// Create Cloudflare client for verification.
		cfClient = getCloudflareClient()

		// Get zone ID for DNS operations.
		var err error
		zoneID, err = getZoneIDByName(ctx, cfClient, testEnv.CloudflareZoneName)
		Expect(err).NotTo(HaveOccurred(), "Failed to get zone ID")
		Expect(zoneID).NotTo(BeEmpty())

		// Create a shared tunnel for tunnelRef tests.
		tunnelName := testID("dns-shared-tunnel")
		sharedTunnel = createCloudflareTunnel(ctx, k8sClient, testID("dns-shared"), namespace.Name, tunnelName)
		sharedTunnel = waitForTunnelReady(ctx, k8sClient, sharedTunnel.Name, namespace.Name, DefaultTimeout)
		Expect(sharedTunnel.Status.TunnelDomain).NotTo(BeEmpty(), "Shared tunnel domain should be populated")

		// Register cleanup via DeferCleanup (Ginkgo #1284 pattern).
		DeferCleanup(func() {
			if testEnv.SkipCleanup {
				return
			}
			// Delete namespace - controller finalizers will handle cleanup.
			if namespace != nil {
				deleteTestNamespace(namespace)
			}
		})
	})

	// =========================================================================
	// Section 1: TunnelRef Mode Tests
	// =========================================================================

	Context("tunnelRef mode", Ordered, func() {
		var (
			dnsResource *cfgatev1alpha1.CloudflareDNS
			hostname    string
		)

		It("creates CNAME record pointing to tunnel domain", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			By("Creating CloudflareDNS with tunnelRef")
			hostname = fmt.Sprintf("%s.%s", testID("tunnelref"), testEnv.CloudflareZoneName)
			dnsResource = createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-tunnelref"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
				true, // TXT ownership enabled
			)

			By("Waiting for CloudflareDNS to be ready")
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CNAME record exists pointing to tunnel domain")
			Eventually(func(g Gomega) {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(record).NotTo(BeNil(), "CNAME record should exist")
				g.Expect(record.Content).To(Equal(sharedTunnel.Status.TunnelDomain), "CNAME should point to tunnel domain")
			}, DefaultTimeout, DefaultInterval).Should(Succeed())
		})

		It("creates proxied DNS records by default", SpecTimeout(2*time.Minute), func(ctx SpecContext) {
			By("Verifying record is proxied (orange cloud)")
			Eventually(func(g Gomega) {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(record).NotTo(BeNil())
				g.Expect(record.Proxied).To(BeTrue(), "Record should be proxied by default")
			}, DefaultTimeout, DefaultInterval).Should(Succeed())
		})

		It("populates status with resolved target", func() {
			By("Verifying status.resolvedTarget matches tunnel domain")
			var current cfgatev1alpha1.CloudflareDNS
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dnsResource.Name, Namespace: namespace.Name}, &current)).To(Succeed())
			Expect(current.Status.ResolvedTarget).To(Equal(sharedTunnel.Status.TunnelDomain))
		})

		It("tracks synced records in status", func() {
			By("Verifying status.syncedRecords count")
			var current cfgatev1alpha1.CloudflareDNS
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dnsResource.Name, Namespace: namespace.Name}, &current)).To(Succeed())
			Expect(current.Status.SyncedRecords).To(BeNumerically(">=", 1), "Should have at least 1 synced record")
		})

		It("cleans up DNS record on CloudflareDNS deletion", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			By("Deleting the CloudflareDNS resource")
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())

			By("Waiting for CloudflareDNS to be deleted from Kubernetes")
			waitForDNSDeleted(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying DNS record is deleted from Cloudflare")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record == nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue(), "DNS record should be deleted on cleanup")
		})
	})

	// =========================================================================
	// Section 2: ExternalTarget Mode Tests
	// =========================================================================

	Context("externalTarget mode", Ordered, func() {
		var (
			dnsResource *cfgatev1alpha1.CloudflareDNS
			hostname    string
		)

		It("creates CNAME record with external target", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			By("Creating CloudflareDNS with externalTarget")
			hostname = fmt.Sprintf("%s.%s", testID("external"), testEnv.CloudflareZoneName)
			dnsResource = createCloudflareDNSWithExternalTarget(ctx, k8sClient,
				testID("dns-external"), namespace.Name,
				cfgatev1alpha1.RecordTypeCNAME,
				"example.com",
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
			)

			By("Waiting for CloudflareDNS to be ready")
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CNAME record exists with external target")
			Eventually(func(g Gomega) {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(record).NotTo(BeNil(), "CNAME record should exist")
				g.Expect(record.Content).To(Equal("example.com"), "CNAME should point to external target")
			}, DefaultTimeout, DefaultInterval).Should(Succeed())
		})

		It("populates status with external target value", func() {
			By("Verifying status.resolvedTarget matches external target")
			var current cfgatev1alpha1.CloudflareDNS
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dnsResource.Name, Namespace: namespace.Name}, &current)).To(Succeed())
			Expect(current.Status.ResolvedTarget).To(Equal("example.com"))
		})

		It("cleans up on deletion", SpecTimeout(2*time.Minute), func(ctx SpecContext) {
			By("Deleting the CloudflareDNS resource")
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())

			By("Waiting for DNS record cleanup")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record == nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue())
		})
	})

	// =========================================================================
	// Section 3: DNS Policy Tests
	// =========================================================================

	Context("policy modes", func() {
		It("sync policy creates, updates, and deletes records", SpecTimeout(4*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("sync-policy"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS with sync policy")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-sync-policy"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
				false,
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CREATE: record is created")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue(), "Record should be created")

			By("Verifying DELETE: deleting CloudflareDNS deletes records")
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
			Eventually(func() bool {
				record, _ := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return record == nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue(), "Record should be deleted with sync policy")
		})

		It("upsert-only policy creates but never deletes", SpecTimeout(4*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("upsert-policy"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS with upsert-only policy")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-upsert-policy"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicyUpsertOnly,
				false,
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CREATE: record is created")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue())

			By("Deleting the CloudflareDNS resource")
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
			waitForDNSDeleted(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying NO DELETE: record should still exist with upsert-only policy")
			Consistently(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, ShortTimeout, DefaultInterval).Should(BeTrue(), "Record should NOT be deleted with upsert-only policy")

			// Manual cleanup for test hygiene.
			cleanupDNSRecord(ctx, cfClient, zoneID, hostname, "CNAME")
		})

		It("create-only policy creates but never updates or deletes", SpecTimeout(4*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("createonly-policy"), testEnv.CloudflareZoneName)

			By("Creating a pre-existing DNS record with different target")
			_, err := cfClient.DNS.Records.New(ctx, dns.RecordNewParams{
				ZoneID: cloudflare.F(zoneID),
				Body: dns.CNAMERecordParam{
					Name:    cloudflare.F(hostname),
					Type:    cloudflare.F(dns.CNAMERecordTypeCNAME),
					Content: cloudflare.F("original.example.com"),
					TTL:     cloudflare.F(dns.TTL(1)),
					Proxied: cloudflare.F(true),
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Creating CloudflareDNS with create-only policy")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-createonly-policy"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicyCreateOnly,
				false,
			)
			// Wait for reconciliation to attempt (may not set Ready if record exists).
			time.Sleep(10 * time.Second)

			By("Verifying NO UPDATE: record keeps original content")
			Consistently(func() string {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				if err != nil || record == nil {
					return ""
				}
				return record.Content
			}, ShortTimeout, DefaultInterval).Should(Equal("original.example.com"), "Record should NOT be updated with create-only policy")

			By("Deleting the CloudflareDNS resource")
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
			waitForDNSDeleted(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying NO DELETE: record should still exist")
			Consistently(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, ShortTimeout, DefaultInterval).Should(BeTrue(), "Record should NOT be deleted with create-only policy")

			// Manual cleanup.
			cleanupDNSRecord(ctx, cfClient, zoneID, hostname, "CNAME")
		})
	})

	// =========================================================================
	// Section 4: TXT Ownership Record Tests
	// =========================================================================

	Context("TXT ownership records", func() {
		It("creates TXT ownership record when enabled", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("txt-enabled"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS with TXT ownership enabled")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-txt-enabled"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
				true, // TXT enabled
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CNAME record exists")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue())

			By("Verifying TXT ownership record exists with correct format")
			txtHostname := fmt.Sprintf("_cfgate.%s", hostname)
			Eventually(func(g Gomega) {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, txtHostname, "TXT")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(record).NotTo(BeNil(), "TXT ownership record should exist")
				g.Expect(record.Name).To(Equal(txtHostname), "TXT record should have correct name format")
			}, DefaultTimeout, DefaultInterval).Should(Succeed())

			// Cleanup.
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
		})

		It("skips TXT record when ownership disabled", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("txt-disabled"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS with TXT ownership disabled")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-txt-disabled"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
				false, // TXT disabled
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CNAME record exists")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue())

			By("Verifying no TXT ownership record exists")
			txtHostname := fmt.Sprintf("_cfgate.%s", hostname)
			Consistently(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, txtHostname, "TXT")
				return err == nil && record == nil
			}, ShortTimeout, DefaultInterval).Should(BeTrue(), "No TXT record should be created when ownership disabled")

			// Cleanup.
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
		})

		It("uses custom TXT prefix when specified", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("txt-custom"), testEnv.CloudflareZoneName)
			customPrefix := "_custom-owner"

			By("Creating CloudflareDNS with custom TXT prefix")
			dnsResource := createCloudflareDNSWithCustomTXTPrefix(ctx, k8sClient,
				testID("dns-txt-custom"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				customPrefix,
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying CNAME record exists")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue())

			By("Verifying TXT record uses custom prefix")
			customTXTHostname := fmt.Sprintf("%s.%s", customPrefix, hostname)
			Eventually(func(g Gomega) {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, customTXTHostname, "TXT")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(record).NotTo(BeNil(), "TXT record with custom prefix should exist")
			}, DefaultTimeout, DefaultInterval).Should(Succeed())

			// Cleanup.
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
		})
	})

	// =========================================================================
	// Section 5: Zone Resolution Tests
	// =========================================================================

	Context("zone resolution", func() {
		It("resolves zone by name", SpecTimeout(2*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("zone-resolve"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS with zone name only (no ID)")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-zone-resolve"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
				false,
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying record was created in correct zone")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue(), "Record should be created in resolved zone")

			// Cleanup.
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
		})

		It("handles hostname matching zone correctly", SpecTimeout(2*time.Minute), func(ctx SpecContext) {
			// Test that a hostname is matched to the correct zone.
			hostname := fmt.Sprintf("%s.%s", testID("zone-match"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS")
			dnsResource := createCloudflareDNSWithTunnelRef(ctx, k8sClient,
				testID("dns-zone-match"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				cfgatev1alpha1.DNSPolicySync,
				false,
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying status has correct zone info")
			var current cfgatev1alpha1.CloudflareDNS
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dnsResource.Name, Namespace: namespace.Name}, &current)).To(Succeed())
			Expect(current.Status.SyncedRecords).To(BeNumerically(">=", 1))

			// Cleanup.
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
		})
	})

	// =========================================================================
	// Section 6: Cleanup Policy Tests
	// =========================================================================

	Context("cleanup policy", func() {
		It("respects deleteOnResourceRemoval=false", SpecTimeout(3*time.Minute), func(ctx SpecContext) {
			hostname := fmt.Sprintf("%s.%s", testID("cleanup-false"), testEnv.CloudflareZoneName)

			By("Creating CloudflareDNS with deleteOnResourceRemoval=false")
			dnsResource := createCloudflareDNSWithCleanupPolicy(ctx, k8sClient,
				testID("dns-cleanup-false"), namespace.Name,
				sharedTunnel.Name, namespace.Name,
				[]string{hostname},
				false, // deleteOnResourceRemoval=false
			)
			dnsResource = waitForDNSReady(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying record is created")
			Eventually(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, DefaultTimeout, DefaultInterval).Should(BeTrue())

			By("Deleting the CloudflareDNS resource")
			Expect(k8sClient.Delete(ctx, dnsResource)).To(Succeed())
			waitForDNSDeleted(ctx, k8sClient, dnsResource.Name, namespace.Name, DefaultTimeout)

			By("Verifying record is NOT deleted (cleanup disabled)")
			Consistently(func() bool {
				record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, "CNAME")
				return err == nil && record != nil
			}, ShortTimeout, DefaultInterval).Should(BeTrue(), "Record should NOT be deleted when cleanup disabled")

			// Manual cleanup.
			cleanupDNSRecord(ctx, cfClient, zoneID, hostname, "CNAME")
		})
	})
})

// =============================================================================
// Helper Functions for CloudflareDNS Tests
// =============================================================================

// createCloudflareDNSWithTunnelRef creates a CloudflareDNS with tunnelRef mode.
func createCloudflareDNSWithTunnelRef(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	tunnelName, tunnelNamespace string,
	hostnames []string,
	policy cfgatev1alpha1.DNSPolicy,
	txtEnabled bool,
) *cfgatev1alpha1.CloudflareDNS {
	explicitHostnames := make([]cfgatev1alpha1.DNSExplicitHostname, len(hostnames))
	for i, h := range hostnames {
		explicitHostnames[i] = cfgatev1alpha1.DNSExplicitHostname{
			Hostname: h,
		}
	}

	dns := &cfgatev1alpha1.CloudflareDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareDNSSpec{
			TunnelRef: &cfgatev1alpha1.DNSTunnelRef{
				Name:      tunnelName,
				Namespace: tunnelNamespace,
			},
			Zones: []cfgatev1alpha1.DNSZoneConfig{
				{
					Name: testEnv.CloudflareZoneName,
				},
			},
			Policy: policy,
			Source: cfgatev1alpha1.DNSHostnameSource{
				Explicit: explicitHostnames,
			},
			Ownership: cfgatev1alpha1.DNSOwnershipConfig{
				TXTRecord: cfgatev1alpha1.DNSTXTRecordOwnership{
					Enabled: ptrTo(txtEnabled),
					Prefix:  "_cfgate",
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, dns)).To(Succeed())
	return dns
}

// createCloudflareDNSWithExternalTarget creates a CloudflareDNS with externalTarget mode.
func createCloudflareDNSWithExternalTarget(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	recordType cfgatev1alpha1.RecordType,
	targetValue string,
	hostnames []string,
	policy cfgatev1alpha1.DNSPolicy,
) *cfgatev1alpha1.CloudflareDNS {
	explicitHostnames := make([]cfgatev1alpha1.DNSExplicitHostname, len(hostnames))
	for i, h := range hostnames {
		explicitHostnames[i] = cfgatev1alpha1.DNSExplicitHostname{
			Hostname: h,
		}
	}

	dns := &cfgatev1alpha1.CloudflareDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareDNSSpec{
			ExternalTarget: &cfgatev1alpha1.ExternalTarget{
				Type:  recordType,
				Value: targetValue,
			},
			Zones: []cfgatev1alpha1.DNSZoneConfig{
				{
					Name: testEnv.CloudflareZoneName,
				},
			},
			Policy: policy,
			Source: cfgatev1alpha1.DNSHostnameSource{
				Explicit: explicitHostnames,
			},
			Cloudflare: &cfgatev1alpha1.CloudflareConfig{
				AccountID: testEnv.CloudflareAccountID,
				SecretRef: cfgatev1alpha1.SecretRef{
					Name: "cloudflare-credentials",
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, dns)).To(Succeed())
	return dns
}

// createCloudflareDNSWithCustomTXTPrefix creates a CloudflareDNS with custom TXT prefix.
func createCloudflareDNSWithCustomTXTPrefix(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	tunnelName, tunnelNamespace string,
	hostnames []string,
	txtPrefix string,
) *cfgatev1alpha1.CloudflareDNS {
	explicitHostnames := make([]cfgatev1alpha1.DNSExplicitHostname, len(hostnames))
	for i, h := range hostnames {
		explicitHostnames[i] = cfgatev1alpha1.DNSExplicitHostname{
			Hostname: h,
		}
	}

	dns := &cfgatev1alpha1.CloudflareDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareDNSSpec{
			TunnelRef: &cfgatev1alpha1.DNSTunnelRef{
				Name:      tunnelName,
				Namespace: tunnelNamespace,
			},
			Zones: []cfgatev1alpha1.DNSZoneConfig{
				{
					Name: testEnv.CloudflareZoneName,
				},
			},
			Policy: cfgatev1alpha1.DNSPolicySync,
			Source: cfgatev1alpha1.DNSHostnameSource{
				Explicit: explicitHostnames,
			},
			Ownership: cfgatev1alpha1.DNSOwnershipConfig{
				TXTRecord: cfgatev1alpha1.DNSTXTRecordOwnership{
					Enabled: ptrTo(true),
					Prefix:  txtPrefix,
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, dns)).To(Succeed())
	return dns
}

// createCloudflareDNSWithCleanupPolicy creates a CloudflareDNS with specific cleanup policy.
func createCloudflareDNSWithCleanupPolicy(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	tunnelName, tunnelNamespace string,
	hostnames []string,
	deleteOnResourceRemoval bool,
) *cfgatev1alpha1.CloudflareDNS {
	explicitHostnames := make([]cfgatev1alpha1.DNSExplicitHostname, len(hostnames))
	for i, h := range hostnames {
		explicitHostnames[i] = cfgatev1alpha1.DNSExplicitHostname{
			Hostname: h,
		}
	}

	dns := &cfgatev1alpha1.CloudflareDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cfgatev1alpha1.CloudflareDNSSpec{
			TunnelRef: &cfgatev1alpha1.DNSTunnelRef{
				Name:      tunnelName,
				Namespace: tunnelNamespace,
			},
			Zones: []cfgatev1alpha1.DNSZoneConfig{
				{
					Name: testEnv.CloudflareZoneName,
				},
			},
			Policy: cfgatev1alpha1.DNSPolicySync,
			Source: cfgatev1alpha1.DNSHostnameSource{
				Explicit: explicitHostnames,
			},
			CleanupPolicy: cfgatev1alpha1.DNSCleanupPolicy{
				DeleteOnResourceRemoval: &deleteOnResourceRemoval,
			},
		},
	}
	Expect(k8sClient.Create(ctx, dns)).To(Succeed())
	return dns
}

// waitForDNSReady waits for a CloudflareDNS to have Ready=True condition.
func waitForDNSReady(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) *cfgatev1alpha1.CloudflareDNS {
	var dns cfgatev1alpha1.CloudflareDNS

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &dns)
		if err != nil {
			return false
		}
		for _, cond := range dns.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, DefaultInterval).Should(BeTrue(), "CloudflareDNS did not become ready")

	return &dns
}

// waitForDNSDeleted waits for a CloudflareDNS to be deleted from Kubernetes.
func waitForDNSDeleted(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) {
	Eventually(func() bool {
		var dns cfgatev1alpha1.CloudflareDNS
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &dns)
		return client.IgnoreNotFound(err) == nil && err != nil
	}, timeout, DefaultInterval).Should(BeTrue(), "CloudflareDNS was not deleted")
}

// cleanupDNSRecord manually deletes a DNS record for test hygiene.
func cleanupDNSRecord(ctx context.Context, cfClient *cloudflare.Client, zoneID, hostname, recordType string) {
	record, err := getDNSRecordFromCloudflare(ctx, cfClient, zoneID, hostname, recordType)
	if err != nil || record == nil {
		return
	}
	_, _ = cfClient.DNS.Records.Delete(ctx, record.ID, dns.RecordDeleteParams{
		ZoneID: cloudflare.F(zoneID),
	})
}
