# Testing

cfgate uses an E2E-only test strategy. All tests run against the live Cloudflare API -- there are no unit tests, integration tests, mocks, or fixtures.

## Philosophy

- **Real API only.** Every test creates and verifies actual Cloudflare resources (tunnels, DNS records, Access applications, service tokens).
- **E2E-only coverage.** Controller reconciliation patterns are incompatible with VCR/cassette approaches (attempted and removed). The controller runs in-process during tests against a real kind cluster.
- **API state verification.** Tests verify that Kubernetes CRD state and Cloudflare API state converge correctly.

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

> **Note:** `CLOUDFLARE_ZONE_NAME` is a test-only variable. The cfgate controller does not use it -- zones are configured per CloudflareDNS resource via `spec.zones[]`.

### Optional

| Variable | Purpose |
|----------|---------|
| `CLOUDFLARE_IDP_ID` | Identity Provider ID for IdP-dependent Access rule tests |
| `CLOUDFLARE_TEST_EMAIL` | Test email address for email rule verification |
| `CLOUDFLARE_TEST_GROUP` | Test group name for GSuite group rule verification |
| `E2E_SKIP_CLEANUP` | Set to `true` to skip resource cleanup after tests (for debugging) |
| `E2E_USE_EXISTING_CLUSTER` | Set to `true` to use existing kubeconfig cluster instead of creating kind |

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
- Ginkgo parallel execution across Describe blocks
- Race detection enabled
- JSON report output to `out/e2e-run.json`
- Coverage profile to `out/coverage.out`
- Progress polling after 15s silence

### Run Specific Tests

```bash
# Run only DNS tests
ginkgo -vv --focus "CloudflareDNS" ./test/e2e

# Run only tunnel tests
ginkgo -vv --focus "CloudflareTunnel" ./test/e2e

# Run only Access tests
ginkgo -vv --focus "CloudflareAccessPolicy" ./test/e2e

# Run only annotation tests
ginkgo -vv --focus "HTTPRoute Annotations" ./test/e2e
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
  e2e_suite_test.go    # Suite setup, environment loading, cleanup helpers
  helpers_test.go      # Wait functions, CF API verification helpers
  tunnel_test.go       # CloudflareTunnel lifecycle tests
  dns_test.go          # CloudflareDNS sync, policies, ownership tests
  access_test.go       # CloudflareAccessPolicy tests
  annotations_test.go  # HTTPRoute annotation tests (origin config, DNS config)
  combined_test.go     # Multi-CRD interaction tests
  validation_test.go   # CEL validation tests (no Cloudflare API needed)
```

### Test Naming Convention

Resources created during tests follow the pattern:

```
e2e-{type}-{ginkgo-node}-{unix-timestamp}
```

This ensures parallel test nodes don't collide and orphaned resources are identifiable.

### Test Output

After running `mise run e2e`:

| File | Contents |
|------|----------|
| `out/e2e-run.json` | Ginkgo JSON report with pass/fail per spec |
| `out/coverage.out` | Go coverage profile |

## Skipped Tests

Some tests are skipped when optional environment variables are missing:

| Missing Variable | Skipped Tests |
|-----------------|---------------|
| `CLOUDFLARE_ZONE_NAME` | All DNS tests, Access tests with hostnames, annotation tests |
| `CLOUDFLARE_IDP_ID` | IdP-dependent Access rule tests (email, group) |
| `CLOUDFLARE_TEST_EMAIL` | Email-based Access rule tests |
| `CLOUDFLARE_TEST_GROUP` | GSuite group-based Access rule tests |
