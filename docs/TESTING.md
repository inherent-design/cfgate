# Testing

cfgate uses an E2E-only test strategy. All tests run against the live Cloudflare API. There are no unit tests, integration tests, mocks, or fixtures.

## Philosophy

- **Real API only.** Every test creates and verifies actual Cloudflare resources (tunnels, DNS records, Access applications, service tokens).
- **E2E-only coverage.** Controller reconciliation patterns are incompatible with VCR/cassette approaches (attempted and removed). The controller runs in-process during tests against a real kind cluster.
- **API state verification.** Tests verify that Kubernetes CRD state and Cloudflare API state converge correctly.
- **94 specs across 6 test files.** Full lifecycle coverage for all CRDs, annotations, cross-resource interactions, CEL validation, and edge cases.

## Environment Variables

All variables are injected via `mise` from `secrets.enc.yaml` and `.env`. See [CONTRIBUTING.md](../CONTRIBUTING.md) for secrets setup.

### Required

| Variable | Purpose |
|----------|---------|
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token with required permissions |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account ID for tunnel and Access operations |

### Required for DNS and Access Tests

| Variable | Purpose |
|----------|---------|
| `CLOUDFLARE_ZONE_NAME` | Zone domain name for test DNS records (e.g., `example.com`) |

Tests construct hostnames as `e2e-{type}-{node}-{timestamp}.{CLOUDFLARE_ZONE_NAME}`. Without this variable, DNS and Access test suites are skipped.

> **Note:** `CLOUDFLARE_ZONE_NAME` is a test-only variable. The cfgate controller does not use it. Zones are configured per CloudflareDNS resource via `spec.zones[]`.

### Optional

| Variable | Purpose |
|----------|---------|
| `CLOUDFLARE_IDP_ID` | Identity Provider ID for IdP-dependent Access rule tests |
| `CLOUDFLARE_TEST_EMAIL` | Test email address for email rule verification |
| `CLOUDFLARE_TEST_GROUP` | Test group name for GSuite group rule verification |
| `E2E_SKIP_CLEANUP` | Set to `true` to skip resource cleanup after tests (for debugging) |
| `E2E_USE_EXISTING_CLUSTER` | Set to `true` to use existing kubeconfig cluster instead of creating kind |
| `E2E_PROCS` | Ginkgo parallel process count (default: 4) |

### API Token Permissions

The test token needs the same permissions as a production token:

| Scope | Permission | Required For |
|-------|------------|--------------|
| Account | Cloudflare Tunnel: Edit | Tunnel lifecycle tests |
| Account | Access: Apps and Policies: Edit | Access policy tests |
| Account | Access: Service Tokens: Edit | Service token rule tests |
| Zone | DNS: Edit | DNS record sync tests |

## Running Tests

### Prerequisites

1. Install toolchain:

```bash
brew install mise
mise install
```

2. Configure secrets (see [CONTRIBUTING.md](../CONTRIBUTING.md#secrets-configuration))

3. Ensure a Kubernetes cluster is available:

```bash
# Option A: Create a dedicated kind cluster
mise run cluster:create

# Option B: Use existing cluster
export E2E_USE_EXISTING_CLUSTER=true
```

The test suite installs CRDs and Gateway API resources automatically if not already present.

### Run E2E Tests

```bash
mise run e2e
```

This runs the full suite with:
- Ginkgo parallel execution (4 procs by default, configurable via `E2E_PROCS`)
- Race detection enabled
- JSON report output to `out/run.json`
- Coverage profile to `out/coverage.out`
- Progress polling after 15s silence

### Run Specific Tests

```bash
# By CRD type
ginkgo -vv --focus "CloudflareTunnel" ./test/e2e
ginkgo -vv --focus "CloudflareDNS" ./test/e2e
ginkgo -vv --focus "CloudflareAccessPolicy" ./test/e2e

# Annotations
ginkgo -vv --focus "HTTPRoute Annotations" ./test/e2e

# Multi-CRD interactions
ginkgo -vv --focus "Combined" ./test/e2e

# CEL validation (no Cloudflare API needed)
ginkgo -vv --focus "CEL Validation" ./test/e2e
```

### Adjust Parallelism

```bash
# Single process (useful for debugging ordering issues)
E2E_PROCS=1 mise run e2e

# Higher parallelism (if your API token rate limits allow)
E2E_PROCS=8 mise run e2e
```

### Cleanup Orphaned Resources

If tests fail or `E2E_SKIP_CLEANUP=true` was set, resources may be left in Cloudflare. The cleanup utility removes them:

```bash
mise run e2e:cleanup
```

This scans for and deletes:
- Tunnels with `e2e-` or `recovery-` name prefix
- DNS records containing `e2e-` or `_cfgate.e2e-` in the name
- Access applications with `e2e-` name prefix
- Service tokens with `e2e-` name prefix

Run cleanup before E2E tests to ensure a clean slate if previous runs left orphans.

## Test Structure

```
test/e2e/
  e2e_suite_test.go     # Suite setup, framework init, cleanup helpers
  helpers_test.go       # Wait functions, resource creators, CF API verifiers
  tunnel_test.go        # CloudflareTunnel lifecycle (17 specs)
  dns_test.go           # CloudflareDNS sync, policies, ownership (18 specs)
  access_test.go        # CloudflareAccessPolicy rules and applications (26 specs)
  annotations_test.go   # HTTPRoute annotation parsing and propagation (16 specs)
  combined_test.go      # Multi-CRD interaction and cross-resource tests (7 specs)
  validation_test.go    # CEL validation rules, no Cloudflare API needed (10 specs)
```

### Test Naming Convention

Resources created during tests follow the pattern:

```
e2e-{type}-{ginkgo-node}-{unix-timestamp}
```

This ensures parallel test nodes do not collide and orphaned resources are identifiable.

## Test Patterns

### SpecTimeout

Every spec that calls the Cloudflare API uses `SpecTimeout` to prevent hangs:

```go
It("creates CNAME record pointing to tunnel domain", SpecTimeout(6*time.Minute), func(ctx SpecContext) {
    // ctx is cancelled when SpecTimeout fires
})
```

Typical timeouts:
- Tunnel operations: 3-5 minutes (tunnel creation is the slowest API call)
- DNS operations: 6 minutes (propagation verification)
- Access operations: 3-5 minutes
- Validation-only specs: no timeout needed (no API calls)

### Conflict Retry (Eventually + Get/Update)

When updating a resource that the controller may also be reconciling, use `Eventually` with a fresh `Get` inside the assertion to retry on conflict:

```go
Eventually(func(g Gomega) {
    var current cfgatev1alpha1.CloudflareTunnel
    g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(tunnel), &current)).To(Succeed())
    current.Spec.Cloudflared.Image = "cloudflare/cloudflared:2025.1.0"
    g.Expect(k8sClient.Update(ctx, &current)).To(Succeed())
}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
```

This pattern prevents test flakes from Kubernetes optimistic concurrency conflicts.

### Wait Helpers

`helpers_test.go` provides typed wait functions for all resources:

| Helper | Waits For |
|--------|-----------|
| `waitForTunnelReady` | Tunnel status Ready=True |
| `waitForTunnelCondition` | Specific condition on tunnel |
| `waitForTunnelDeleted` | Tunnel removed from K8s |
| `waitForTunnelDeletedFromCloudflare` | Tunnel removed from Cloudflare API |
| `waitForDeploymentReady` | cloudflared Deployment available |
| `waitForDNSReady` | DNS status Ready=True |
| `waitForDNSRecordInCloudflare` | DNS record exists in Cloudflare API |
| `waitForDNSRecordDeletedFromCloudflare` | DNS record removed from Cloudflare API |
| `waitForAccessPolicyReady` | Access policy status Ready=True |
| `waitForAccessPolicyDeleted` | Access policy removed from K8s |
| `waitForAccessApplicationDeletedFromCloudflare` | Access app removed from Cloudflare API |
| `waitForServiceTokenSecretCreated` | Service token Secret created in K8s |

### Resource Creators

`helpers_test.go` provides typed factory functions that create resources with sensible defaults:

| Creator | Creates |
|---------|---------|
| `createCloudflareTunnel` | CloudflareTunnel with standard config |
| `createCloudflareDNSWithGatewayRoutes` | CloudflareDNS with gateway route discovery |
| `createCloudflareAccessPolicy` | Basic Access policy |
| `createCloudflareAccessPolicyWith*` | Access policy with specific rule type (IP, country, email, OIDC, GSuite) |
| `createGatewayClass` | GatewayClass for cfgate |
| `createGateway` | Gateway with tunnel reference |
| `createHTTPRoute` | HTTPRoute with hostname and backend |
| `createTestService` | ClusterIP Service for backends |

## Skipped Tests

Some tests are skipped when optional environment variables are missing:

| Missing Variable | Skipped Tests |
|-----------------|---------------|
| `CLOUDFLARE_ZONE_NAME` | All DNS tests, Access tests with hostnames, annotation tests |
| `CLOUDFLARE_IDP_ID` | IdP-dependent Access rule tests (OIDC claims, GSuite groups) |
| `CLOUDFLARE_TEST_EMAIL` | Email-based Access rule tests |
| `CLOUDFLARE_TEST_GROUP` | GSuite group-based Access rule tests |

## Test Output

After running `mise run e2e`:

| File | Contents |
|------|----------|
| `out/run.json` | Ginkgo JSON report with pass/fail per spec |
| `out/coverage.out` | Go coverage profile |
