// Package e2e contains end-to-end tests for cfgate.
// These tests run against real Cloudflare API and real Kubernetes clusters.
// NO mocks, NO envtest - real APIs only.
package e2e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	cloudflare "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zero_trust"
	"github.com/cloudflare/cloudflare-go/v6/zones"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	"cfgate.io/cfgate/internal/cloudflared"
	"cfgate.io/cfgate/internal/controller"
	"cfgate.io/cfgate/internal/controller/features"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// Environment variables for E2E tests.
const (
	EnvCloudflareAPIToken  = "CLOUDFLARE_API_TOKEN"
	EnvCloudflareAccountID = "CLOUDFLARE_ACCOUNT_ID"
	EnvCloudflareZoneName  = "CLOUDFLARE_ZONE_NAME"
	EnvCloudflareIdPID     = "CLOUDFLARE_IDP_ID"
	EnvCloudflareTestEmail = "CLOUDFLARE_TEST_EMAIL"
	EnvCloudflareTestGroup = "CLOUDFLARE_TEST_GROUP"
	EnvSkipCleanup         = "E2E_SKIP_CLEANUP"
	EnvUseExistingCluster  = "E2E_USE_EXISTING_CLUSTER"
	EnvKubeconfig          = "KUBECONFIG"
)

var (
	// testEnv holds E2E test environment configuration.
	testEnv *E2ETestEnv

	// k8sClient is the Kubernetes client for test operations.
	k8sClient client.Client

	// k8sClientset is the typed Kubernetes clientset.
	k8sClientset *kubernetes.Clientset

	// cfg is the Kubernetes REST config.
	cfg *rest.Config

	// scheme is the runtime scheme with all types registered.
	scheme = runtime.NewScheme()

	// ctx is the test context.
	ctx context.Context

	// cancel is the context cancel function.
	cancel context.CancelFunc

	// projectRoot is the root directory of the cfgate project.
	projectRoot string

	// mgr is the controller manager running in-process.
	mgr manager.Manager

	// mgrCancel cancels the manager context.
	mgrCancel context.CancelFunc

	// featureGates tracks optional Gateway API CRD availability.
	featureGates *features.FeatureGates
)

// E2ETestEnv holds the E2E test environment configuration.
type E2ETestEnv struct {
	// CloudflareAPIToken is the Cloudflare API token.
	CloudflareAPIToken string

	// CloudflareAccountID is the Cloudflare account ID.
	CloudflareAccountID string

	// CloudflareZoneName is the Cloudflare zone for DNS tests.
	CloudflareZoneName string

	// CloudflareIdPID is the Cloudflare Identity Provider ID for IdP-dependent tests.
	CloudflareIdPID string

	// CloudflareTestEmail is a test email for email rule verification.
	CloudflareTestEmail string

	// CloudflareTestGroup is a test group for GSuite group rule verification.
	CloudflareTestGroup string

	// SkipCleanup skips resource cleanup for debugging.
	SkipCleanup bool

	// UseExistingCluster uses an existing cluster instead of creating kind.
	UseExistingCluster bool

	// KindClusterName is the name of the kind cluster (if created).
	KindClusterName string
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger for controller-runtime (@P-LOG-GO-001: logr interface).
	// Using zap backend with development config for readable test output.
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	zapLogger, err := zapConfig.Build()
	Expect(err).NotTo(HaveOccurred())
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseFlagOptions(&ctrlzap.Options{
		Development: true,
		ZapOpts:     []zap.Option{zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return zapLogger.Core() })},
	})))

	// Initialize schemes.
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(cfgatev1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(gatewayv1.Install(scheme)).To(Succeed())
	Expect(gatewayv1b1.Install(scheme)).To(Succeed())

	// Load environment configuration.
	testEnv = loadTestEnv()

	// Find project root for CRD paths.
	projectRoot = findProjectRoot()
	Expect(projectRoot).NotTo(BeEmpty(), "Could not find project root")

	// Set up Kubernetes cluster.
	if testEnv.UseExistingCluster {
		setupExistingCluster()
	} else {
		setupKindCluster()
	}

	// Install CRDs.
	installCRDs()

	// Start controller manager in-process.
	startController()

	By("E2E test environment ready")
})

var _ = AfterSuite(func() {
	By("Tearing down E2E test environment")

	// Clean orphaned test namespaces BEFORE stopping manager.
	// This allows finalizers to be processed while controller is still running.
	if k8sClient != nil && testEnv != nil && !testEnv.SkipCleanup {
		cleanOrphanedTestNamespaces()
	}

	// Stop controller manager.
	if mgrCancel != nil {
		mgrCancel()
	}

	// Final cleanup: delete all orphaned E2E resources from Cloudflare.
	// This is the primary cleanup mechanism - handles tunnels AND DNS records.
	if testEnv != nil && !testEnv.SkipCleanup && testEnv.CloudflareAPIToken != "" {
		cleanOrphanedE2EResources()
	}

	if testEnv != nil && !testEnv.SkipCleanup && !testEnv.UseExistingCluster {
		teardownKindCluster()
	}

	cancel()
})

// testID generates a deterministic resource name based on Ginkgo node for parallel safety.
// Format: e2e-{type}-{node}-{line}
// Cleanup relies on these patterns - all e2e-* resources are batch-deleted in AfterSuite.
func testID(resourceType string) string {
	specIndex := CurrentSpecReport().LeafNodeLocation.LineNumber
	return fmt.Sprintf("e2e-%s-%d-%d", resourceType, GinkgoParallelProcess(), specIndex)
}

// cleanOrphanedTestNamespaces removes any test namespaces that weren't cleaned up.
// Must be called BEFORE stopping the controller manager so finalizers can be processed.
func cleanOrphanedTestNamespaces() {
	By("Cleaning orphaned test namespaces")

	var nsList corev1.NamespaceList
	if err := k8sClient.List(ctx, &nsList, client.MatchingLabels{
		"cfgate.io/e2e-test": "true",
	}); err != nil {
		GinkgoWriter.Printf("Warning: failed to list test namespaces: %v\n", err)
		return
	}

	for _, ns := range nsList.Items {
		// Remove finalizers from cfgate resources to prevent blocking.
		removeCfgateFinalizersInNamespace(ns.Name)

		// Delete namespace.
		if err := k8sClient.Delete(ctx, &ns); err != nil && !apierrors.IsNotFound(err) {
			GinkgoWriter.Printf("Warning: failed to delete namespace %s: %v\n", ns.Name, err)
		}
	}

	// Wait for namespaces to terminate.
	for _, ns := range nsList.Items {
		Eventually(func() bool {
			var check corev1.Namespace
			err := k8sClient.Get(ctx, client.ObjectKey{Name: ns.Name}, &check)
			return apierrors.IsNotFound(err)
		}, 60*time.Second, 1*time.Second).Should(BeTrue(),
			"Namespace %s did not terminate", ns.Name)
	}
}

// removeCfgateFinalizersInNamespace removes finalizers from all cfgate resources in a namespace.
// This prevents resources from blocking namespace deletion.
func removeCfgateFinalizersInNamespace(namespace string) {
	// Remove finalizers from CloudflareTunnels.
	var tunnels cfgatev1alpha1.CloudflareTunnelList
	if err := k8sClient.List(ctx, &tunnels, client.InNamespace(namespace)); err == nil {
		for i := range tunnels.Items {
			if len(tunnels.Items[i].Finalizers) > 0 {
				tunnels.Items[i].Finalizers = nil
				if err := k8sClient.Update(ctx, &tunnels.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					GinkgoWriter.Printf("Warning: failed to remove finalizers from tunnel %s: %v\n",
						tunnels.Items[i].Name, err)
				}
			}
		}
	}

	// Remove finalizers from CloudflareDNS (alpha.3: separate CRD).
	var dnsResources cfgatev1alpha1.CloudflareDNSList
	if err := k8sClient.List(ctx, &dnsResources, client.InNamespace(namespace)); err == nil {
		for i := range dnsResources.Items {
			if len(dnsResources.Items[i].Finalizers) > 0 {
				dnsResources.Items[i].Finalizers = nil
				if err := k8sClient.Update(ctx, &dnsResources.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					GinkgoWriter.Printf("Warning: failed to remove finalizers from dns %s: %v\n",
						dnsResources.Items[i].Name, err)
				}
			}
		}
	}

	// Remove finalizers from CloudflareAccessPolicies (alpha.3).
	var policies cfgatev1alpha1.CloudflareAccessPolicyList
	if err := k8sClient.List(ctx, &policies, client.InNamespace(namespace)); err == nil {
		for i := range policies.Items {
			if len(policies.Items[i].Finalizers) > 0 {
				policies.Items[i].Finalizers = nil
				if err := k8sClient.Update(ctx, &policies.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					GinkgoWriter.Printf("Warning: failed to remove finalizers from access policy %s: %v\n",
						policies.Items[i].Name, err)
				}
			}
		}
	}
}

// cleanOrphanedE2EResources deletes all E2E test resources from Cloudflare.
// This is the primary cleanup mechanism - runs after all tests complete.
// Cleans: tunnels (e2e-*, recovery-*), DNS records (e2e-*, _cfgate.e2e-*),
// Access applications (e2e-*), and service tokens (e2e-*).
func cleanOrphanedE2EResources() {
	By("Cleaning orphaned E2E resources from Cloudflare")

	cfClient := cloudflare.NewClient(option.WithAPIToken(testEnv.CloudflareAPIToken))
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Clean tunnels.
	cleanOrphanedTunnels(cleanupCtx, cfClient)

	// Clean DNS records (if zone configured).
	if testEnv.CloudflareZoneName != "" {
		cleanOrphanedDNSRecords(cleanupCtx, cfClient)
	}

	// Clean Access applications (alpha.3).
	cleanOrphanedAccessApplications(cleanupCtx, cfClient)

	// Clean service tokens (alpha.3).
	cleanOrphanedServiceTokens(cleanupCtx, cfClient)
}

// cleanOrphanedTunnels deletes e2e-* and recovery-* tunnels.
func cleanOrphanedTunnels(ctx context.Context, cfClient *cloudflare.Client) {
	iter := cfClient.ZeroTrust.Tunnels.Cloudflared.ListAutoPaging(ctx, zero_trust.TunnelCloudflaredListParams{
		AccountID: cloudflare.F(testEnv.CloudflareAccountID),
	})

	var orphaned []struct{ ID, Name string }
	for iter.Next() {
		t := iter.Current()
		if strings.HasPrefix(t.Name, "e2e-") || strings.HasPrefix(t.Name, "recovery-") {
			orphaned = append(orphaned, struct{ ID, Name string }{t.ID, t.Name})
		}
	}

	if err := iter.Err(); err != nil {
		GinkgoWriter.Printf("Warning: failed to list tunnels: %v\n", err)
		return
	}

	if len(orphaned) == 0 {
		GinkgoWriter.Printf("No orphaned tunnels found\n")
		return
	}

	GinkgoWriter.Printf("Deleting %d orphaned tunnels\n", len(orphaned))
	for _, t := range orphaned {
		_, _ = cfClient.ZeroTrust.Tunnels.Cloudflared.Connections.Delete(ctx, t.ID, zero_trust.TunnelCloudflaredConnectionDeleteParams{
			AccountID: cloudflare.F(testEnv.CloudflareAccountID),
		})
		_, err := cfClient.ZeroTrust.Tunnels.Cloudflared.Delete(ctx, t.ID, zero_trust.TunnelCloudflaredDeleteParams{
			AccountID: cloudflare.F(testEnv.CloudflareAccountID),
		})
		if err != nil && !strings.Contains(err.Error(), "not found") {
			GinkgoWriter.Printf("  Warning: %s: %v\n", t.Name, err)
		}
	}
}

// cleanOrphanedDNSRecords deletes e2e-* and _cfgate.e2e-* DNS records.
func cleanOrphanedDNSRecords(ctx context.Context, cfClient *cloudflare.Client) {
	// Get zone ID.
	zoneList, err := cfClient.Zones.List(ctx, zones.ZoneListParams{
		Name: cloudflare.F(testEnv.CloudflareZoneName),
	})
	if err != nil || len(zoneList.Result) == 0 {
		GinkgoWriter.Printf("Warning: failed to get zone ID: %v\n", err)
		return
	}
	zoneID := zoneList.Result[0].ID

	// List all records and filter for E2E patterns.
	iter := cfClient.DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{
		ZoneID: cloudflare.F(zoneID),
	})

	var orphaned []struct{ ID, Name string }
	for iter.Next() {
		r := iter.Current()
		// Match e2e-* hostnames and _cfgate.e2e-* ownership TXT records.
		if strings.Contains(r.Name, "e2e-") || strings.HasPrefix(r.Name, "_cfgate.e2e-") {
			orphaned = append(orphaned, struct{ ID, Name string }{r.ID, r.Name})
		}
	}

	if err := iter.Err(); err != nil {
		GinkgoWriter.Printf("Warning: failed to list DNS records: %v\n", err)
		return
	}

	if len(orphaned) == 0 {
		GinkgoWriter.Printf("No orphaned DNS records found\n")
		return
	}

	GinkgoWriter.Printf("Deleting %d orphaned DNS records\n", len(orphaned))
	for _, r := range orphaned {
		_, err := cfClient.DNS.Records.Delete(ctx, r.ID, dns.RecordDeleteParams{
			ZoneID: cloudflare.F(zoneID),
		})
		if err != nil && !strings.Contains(err.Error(), "not found") {
			GinkgoWriter.Printf("  Warning: %s: %v\n", r.Name, err)
		}
	}
}

// cleanOrphanedAccessApplications deletes e2e-* Access applications.
func cleanOrphanedAccessApplications(ctx context.Context, cfClient *cloudflare.Client) {
	iter := cfClient.ZeroTrust.Access.Applications.ListAutoPaging(ctx, zero_trust.AccessApplicationListParams{
		AccountID: cloudflare.F(testEnv.CloudflareAccountID),
	})

	var orphaned []struct{ ID, Name string }
	for iter.Next() {
		app := iter.Current()
		// Match e2e-* application names.
		if strings.HasPrefix(app.Name, "e2e-") {
			orphaned = append(orphaned, struct{ ID, Name string }{app.ID, app.Name})
		}
	}

	if err := iter.Err(); err != nil {
		GinkgoWriter.Printf("Warning: failed to list Access applications: %v\n", err)
		return
	}

	if len(orphaned) == 0 {
		GinkgoWriter.Printf("No orphaned Access applications found\n")
		return
	}

	GinkgoWriter.Printf("Deleting %d orphaned Access applications\n", len(orphaned))
	for _, app := range orphaned {
		_, err := cfClient.ZeroTrust.Access.Applications.Delete(ctx, app.ID, zero_trust.AccessApplicationDeleteParams{
			AccountID: cloudflare.F(testEnv.CloudflareAccountID),
		})
		if err != nil && !strings.Contains(err.Error(), "not found") {
			GinkgoWriter.Printf("  Warning: %s: %v\n", app.Name, err)
		}
	}
}

// cleanOrphanedServiceTokens deletes e2e-* service tokens.
func cleanOrphanedServiceTokens(ctx context.Context, cfClient *cloudflare.Client) {
	iter := cfClient.ZeroTrust.Access.ServiceTokens.ListAutoPaging(ctx, zero_trust.AccessServiceTokenListParams{
		AccountID: cloudflare.F(testEnv.CloudflareAccountID),
	})

	var orphaned []struct{ ID, Name string }
	for iter.Next() {
		token := iter.Current()
		// Match e2e-* token names.
		if strings.HasPrefix(token.Name, "e2e-") {
			orphaned = append(orphaned, struct{ ID, Name string }{token.ID, token.Name})
		}
	}

	if err := iter.Err(); err != nil {
		GinkgoWriter.Printf("Warning: failed to list service tokens: %v\n", err)
		return
	}

	if len(orphaned) == 0 {
		GinkgoWriter.Printf("No orphaned service tokens found\n")
		return
	}

	GinkgoWriter.Printf("Deleting %d orphaned service tokens\n", len(orphaned))
	for _, token := range orphaned {
		_, err := cfClient.ZeroTrust.Access.ServiceTokens.Delete(ctx, token.ID, zero_trust.AccessServiceTokenDeleteParams{
			AccountID: cloudflare.F(testEnv.CloudflareAccountID),
		})
		if err != nil && !strings.Contains(err.Error(), "not found") {
			GinkgoWriter.Printf("  Warning: %s: %v\n", token.Name, err)
		}
	}
}

// loadTestEnv loads the test environment from environment variables.
func loadTestEnv() *E2ETestEnv {
	env := &E2ETestEnv{
		CloudflareAPIToken:  os.Getenv(EnvCloudflareAPIToken),
		CloudflareAccountID: os.Getenv(EnvCloudflareAccountID),
		CloudflareZoneName:  os.Getenv(EnvCloudflareZoneName),
		CloudflareIdPID:     os.Getenv(EnvCloudflareIdPID),
		CloudflareTestEmail: os.Getenv(EnvCloudflareTestEmail),
		CloudflareTestGroup: os.Getenv(EnvCloudflareTestGroup),
		SkipCleanup:         os.Getenv(EnvSkipCleanup) == "true",
		UseExistingCluster:  os.Getenv(EnvUseExistingCluster) == "true",
		KindClusterName:     fmt.Sprintf("cfgate-e2e-%d", time.Now().Unix()),
	}

	return env
}

// findProjectRoot finds the cfgate project root directory.
func findProjectRoot() string {
	// Start from the test directory and walk up to find go.mod.
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// setupKindCluster creates a kind cluster for testing.
func setupKindCluster() {
	By("Creating kind cluster: " + testEnv.KindClusterName)

	cmd := exec.CommandContext(ctx, "kind", "create", "cluster",
		"--name", testEnv.KindClusterName,
		"--wait", "5m",
	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	Expect(cmd.Run()).To(Succeed(), "Failed to create kind cluster")

	// Get kubeconfig from kind.
	kubeconfigBytes, err := exec.CommandContext(ctx, "kind", "get", "kubeconfig",
		"--name", testEnv.KindClusterName,
	).Output()
	Expect(err).NotTo(HaveOccurred(), "Failed to get kind kubeconfig")

	// Create temporary kubeconfig file.
	kubeconfigPath := filepath.Join(os.TempDir(), fmt.Sprintf("cfgate-e2e-%s.kubeconfig", testEnv.KindClusterName))
	Expect(os.WriteFile(kubeconfigPath, kubeconfigBytes, 0600)).To(Succeed())

	// Set up clients.
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to build REST config")

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), "Failed to create controller-runtime client")

	k8sClientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred(), "Failed to create clientset")

	// Wait for API server to be ready.
	Eventually(func() error {
		_, err := k8sClientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		return err
	}, 60*time.Second, 1*time.Second).Should(Succeed(), "API server not ready")
}

// setupExistingCluster sets up clients for an existing cluster.
func setupExistingCluster() {
	By("Using existing Kubernetes cluster")

	var err error

	// Load kubeconfig from default location or env.
	kubeconfigPath := os.Getenv(EnvKubeconfig)
	if kubeconfigPath == "" {
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to build REST config from kubeconfig")

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), "Failed to create controller-runtime client")

	k8sClientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred(), "Failed to create clientset")

	// Verify cluster is accessible.
	_, err = k8sClientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to connect to existing cluster")
}

// teardownKindCluster deletes the kind cluster.
func teardownKindCluster() {
	if testEnv.KindClusterName == "" {
		return
	}

	By("Deleting kind cluster: " + testEnv.KindClusterName)

	cmd := exec.CommandContext(context.Background(), "kind", "delete", "cluster",
		"--name", testEnv.KindClusterName,
	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	// Ignore errors on cleanup.
	_ = cmd.Run()
}

// installCRDs installs cfgate and Gateway API CRDs.
func installCRDs() {
	By("Installing CRDs")

	// Install cfgate CRDs.
	crdDir := filepath.Join(projectRoot, "config", "crd", "bases")
	files, err := os.ReadDir(crdDir)
	Expect(err).NotTo(HaveOccurred(), "Failed to read CRD directory")

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".yaml" {
			continue
		}

		crdPath := filepath.Join(crdDir, file.Name())
		cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", crdPath)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter
		Expect(cmd.Run()).To(Succeed(), "Failed to install CRD: "+file.Name())
	}

	// Install Gateway API CRDs (standard channel).
	gatewayAPICRDs := "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml"
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", gatewayAPICRDs)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	Expect(cmd.Run()).To(Succeed(), "Failed to install Gateway API CRDs")

	// Wait for ALL cfgate CRDs to be established.
	// This prevents race conditions where the controller starts before
	// CRD API discovery is complete.
	Eventually(func() bool {
		// Check CloudflareTunnel API registration
		tunnel := &cfgatev1alpha1.CloudflareTunnel{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: "test", Namespace: "default"}, tunnel); err != nil {
			if !apierrors.IsNotFound(err) {
				return false // API not ready (kind not registered)
			}
		}

		// Check CloudflareDNS API registration
		dns := &cfgatev1alpha1.CloudflareDNS{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: "test", Namespace: "default"}, dns); err != nil {
			if !apierrors.IsNotFound(err) {
				return false // API not ready (kind not registered)
			}
		}

		// Check CloudflareAccessPolicy API registration
		policy := &cfgatev1alpha1.CloudflareAccessPolicy{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: "test", Namespace: "default"}, policy); err != nil {
			if !apierrors.IsNotFound(err) {
				return false // API not ready (kind not registered)
			}
		}

		return true
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "cfgate CRDs not fully registered in API server")
}

// startController starts the controller manager in-process.
func startController() {
	By("Starting controller manager in-process")

	var err error
	var mgrCtx context.Context
	mgrCtx, mgrCancel = context.WithCancel(context.Background())

	// Create manager.
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         false,
		HealthProbeBindAddress: "0",                                     // Disable health probe
		Metrics:                metricsserver.Options{BindAddress: "0"}, // Disable metrics
	})
	Expect(err).NotTo(HaveOccurred(), "Failed to create manager")

	// Detect feature gates (optional Gateway API CRDs).
	featureGates, err = features.DetectFeatures(k8sClientset.Discovery())
	Expect(err).NotTo(HaveOccurred(), "Failed to detect feature gates")
	featureGates.LogFeatures(ctrl.Log.WithName("e2e"))

	// Set up CloudflareTunnel controller.
	tunnelReconciler := &controller.CloudflareTunnelReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("cloudflaretunnel-controller"),
		Builder:  cloudflared.NewBuilder(),
	}
	Expect(tunnelReconciler.SetupWithManager(mgr)).To(Succeed(), "Failed to setup tunnel controller")

	// CloudflareDNS controller (alpha.3: separate CRD).
	dnsReconciler := &controller.CloudflareDNSReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("cloudflaredns-controller"),
	}
	Expect(dnsReconciler.SetupWithManager(mgr)).To(Succeed(), "Failed to setup DNS controller")

	// CloudflareAccessPolicy controller.
	accessReconciler := &controller.CloudflareAccessPolicyReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorder("cloudflareaccesspolicy-controller"),
		FeatureGates: featureGates,
	}
	Expect(accessReconciler.SetupWithManager(mgr)).To(Succeed(), "Failed to setup access policy controller")

	// HTTPRoute controller.
	httpRouteReconciler := &controller.HTTPRouteReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("httproute-controller"),
	}
	Expect(httpRouteReconciler.SetupWithManager(mgr)).To(Succeed(), "Failed to setup HTTPRoute controller")

	// Start manager in background.
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(mgrCtx)
		if err != nil && mgrCtx.Err() == nil {
			Fail(fmt.Sprintf("Manager failed to start: %v", err))
		}
	}()

	// Wait for manager caches to sync.
	Eventually(func() bool {
		return mgr.GetCache().WaitForCacheSync(mgrCtx)
	}, 30*time.Second, 100*time.Millisecond).Should(BeTrue(), "Manager caches did not sync")
}

// createTestNamespace creates a unique namespace for a test.
func createTestNamespace(prefix string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix + "-",
			Labels: map[string]string{
				"cfgate.io/e2e-test": "true",
			},
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns
}

// deleteTestNamespace deletes a test namespace and waits for termination.
// Explicitly deletes CloudflareTunnel CRs first so finalizers can access credentials.
func deleteTestNamespace(ns *corev1.Namespace) {
	if testEnv.SkipCleanup {
		return
	}

	// Delete CloudflareTunnels first - finalizers need the Secret which lives in namespace.
	var tunnels cfgatev1alpha1.CloudflareTunnelList
	if err := k8sClient.List(ctx, &tunnels, client.InNamespace(ns.Name)); err == nil {
		for i := range tunnels.Items {
			_ = k8sClient.Delete(ctx, &tunnels.Items[i])
		}
		// Wait for tunnels to be fully deleted (finalizers complete).
		Eventually(func() bool {
			var check cfgatev1alpha1.CloudflareTunnelList
			if err := k8sClient.List(ctx, &check, client.InNamespace(ns.Name)); err != nil {
				return true
			}
			return len(check.Items) == 0
		}, 60*time.Second, 1*time.Second).Should(BeTrue(),
			"CloudflareTunnels in namespace %s did not terminate", ns.Name)
	}

	// Delete namespace after CRs are gone.
	Expect(k8sClient.Delete(ctx, ns)).To(Succeed())

	// Wait for namespace to terminate.
	Eventually(func() bool {
		var check corev1.Namespace
		err := k8sClient.Get(ctx, client.ObjectKey{Name: ns.Name}, &check)
		return apierrors.IsNotFound(err)
	}, 120*time.Second, 1*time.Second).Should(BeTrue(),
		"Namespace %s did not terminate", ns.Name)
}

// createCloudflareCredentialsSecret creates the Cloudflare credentials secret.
func createCloudflareCredentialsSecret(namespace string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloudflare-credentials",
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"CLOUDFLARE_API_TOKEN": testEnv.CloudflareAPIToken,
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	return secret
}

// skipIfNoCredentials skips the test if Cloudflare credentials are not available.
func skipIfNoCredentials() {
	if testEnv.CloudflareAPIToken == "" {
		Skip("CLOUDFLARE_API_TOKEN not set - skipping E2E test")
	}
	if testEnv.CloudflareAccountID == "" {
		Skip("CLOUDFLARE_ACCOUNT_ID not set - skipping E2E test")
	}
}

// skipIfNoZone skips the test if Cloudflare zone is not configured.
func skipIfNoZone() {
	skipIfNoCredentials()
	if testEnv.CloudflareZoneName == "" {
		Skip("CLOUDFLARE_ZONE_NAME not set - skipping DNS E2E test")
	}
}

// skipIfNoIdP skips the test if Identity Provider is not configured.
// Required for P1 tests: email, emailDomain, oidcClaim rules.
func skipIfNoIdP() {
	skipIfNoZone()
	if testEnv.CloudflareIdPID == "" {
		Skip("CLOUDFLARE_IDP_ID not set - required for IdP-dependent Access tests")
	}
}

// skipIfNoGSuiteGroup skips the test if GSuite group testing is not configured.
// Required for P2 tests: gsuiteGroup rules.
func skipIfNoGSuiteGroup() {
	skipIfNoIdP()
	if testEnv.CloudflareTestGroup == "" {
		Skip("CLOUDFLARE_TEST_GROUP not set - required for GSuite group tests")
	}
}
