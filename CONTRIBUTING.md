# Contributing to cfgate

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [mise](https://mise.jdx.dev/) (task runner and tool manager)
- [Docker](https://docs.docker.com/get-docker/) (for container builds and kind clusters)
- [sops](https://github.com/getsops/sops) + [age](https://github.com/FiloSottile/age) (for secrets management)
- A Cloudflare account with API token (see [API Token Permissions](#api-token-permissions))

## Getting Started

```bash
git clone https://github.com/inherent-design/cfgate.git
cd cfgate

# Install toolchain (Go, kind, kubectl, kustomize, golangci-lint, controller-gen, ginkgo)
mise install

# List available tasks
mise tasks

# Generate CRDs and DeepCopy
mise run codegen

# Build
mise run build

# Lint
mise run lint
```

## Task Reference

| Task | Alias | Description |
|------|-------|-------------|
| `codegen` | -- | Generate DeepCopy and CRD manifests |
| `build` | `b` | Build manager binary with version info |
| `lint` | -- | Run golangci-lint |
| `lint:fix` | -- | Run golangci-lint with auto-fix |
| `format` | `fmt` | Format and vet code |
| `manifests` | `dist` | Generate release manifests to `dist/` |
| `e2e` | -- | Run E2E tests against live Cloudflare API |
| `e2e:cleanup` | `clean` | Clean orphaned E2E resources from Cloudflare |
| `cluster:create` | -- | Create dedicated cfgate dev cluster |
| `cluster:delete` | -- | Delete cfgate dev cluster |
| `cluster:status` | -- | Check cfgate dev cluster status |
| `local:install` | -- | Install Gateway API and cfgate CRDs |
| `local:deploy` | -- | Deploy controller to current cluster (kustomize) |
| `local:helm` | -- | Deploy controller via local Helm chart |
| `local:undeploy` | -- | Remove controller from current cluster |
| `local:uninstall` | -- | Uninstall CRDs from current cluster |
| `run` | -- | Run controller locally (outside cluster) |
| `docker:build` | `db` | Build Docker image |
| `docker:push` | `dp` | Push Docker image to registry |
| `docker:buildx` | -- | Build multi-arch image (amd64 + arm64) |
| `helm:lint` | -- | Lint Helm chart |
| `helm:template` | -- | Render Helm chart templates (dry run) |

## Secrets Configuration

cfgate uses [sops](https://github.com/getsops/sops) with [age](https://github.com/FiloSottile/age) encryption for local development secrets. mise reads `secrets.enc.yaml` automatically via `[env] _.file`.

### Setting Up Your Own Secrets

1. **Generate an age keypair:**

```bash
age-keygen -o ~/.config/sops/age/keys.txt
```

This outputs your public key (starts with `age1...`). Save it. You need it for `.sops.yaml`.

2. **Configure sops to use your key:**

Edit `.sops.yaml` in the cfgate repo root to use your public key:

```yaml
creation_rules:
  - age: age1your-public-key-here
```

3. **Create `secrets.enc.yaml`:**

Write plaintext credentials, then encrypt in-place:

```bash
cat > secrets.enc.yaml <<'EOF'
CLOUDFLARE_API_TOKEN: your-api-token
CLOUDFLARE_ACCOUNT_ID: your-account-id
CLOUDFLARE_ZONE_NAME: your-zone.com
EOF

sops -e -i secrets.enc.yaml
```

### Required Keys

| Key | Purpose |
|-----|---------|
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account ID |

### Optional Keys

| Key | Purpose |
|-----|---------|
| `CLOUDFLARE_ZONE_NAME` | Zone for DNS/Access E2E tests |
| `CLOUDFLARE_IDP_ID` | Identity Provider ID for IdP-dependent tests |
| `CLOUDFLARE_TEST_EMAIL` | Email for email rule tests |
| `CLOUDFLARE_TEST_GROUP` | Group for GSuite group rule tests |

### Verifying Secrets

```bash
# Decrypt and view (does not modify the file)
sops decrypt secrets.enc.yaml

# Edit encrypted file in-place
sops secrets.enc.yaml

# Verify mise can read them
mise env | grep CLOUDFLARE
```

### API Token Permissions

Create a token at [Cloudflare Dashboard > API Tokens](https://dash.cloudflare.com/profile/api-tokens) with:

| Scope | Permission | Required For |
|-------|------------|--------------|
| Account | Cloudflare Tunnel: Edit | Tunnel tests |
| Account | Access: Apps and Policies: Edit | Access tests |
| Account | Access: Service Tokens: Edit | Service token tests |
| Zone | DNS: Edit | DNS tests |

Scope the zone permissions to the zone matching `CLOUDFLARE_ZONE_NAME`.

## Testing

See [docs/TESTING.md](docs/TESTING.md) for the full testing guide including:
- E2E test strategy and philosophy
- All environment variables
- Running and filtering tests
- Cleanup procedures
- Test structure and naming conventions

Quick start:

```bash
# Create cluster (test suite installs CRDs automatically)
mise run cluster:create

# Run E2E tests
mise run e2e

# Clean up orphaned resources
mise run e2e:cleanup
```

## Development Workflow

### Making Changes

1. Create a feature branch
2. Make changes
3. Regenerate CRDs if types changed: `mise run codegen`
4. Lint: `mise run lint`
5. Build: `mise run build`
6. Test: `mise run e2e`
7. Submit PR

### CRD Changes

When modifying files in `api/v1alpha1/`:

```bash
# Regenerate DeepCopy methods and CRD manifests
mise run codegen

# Verify CRDs are valid
mise run local:install
```

### Running the Controller Locally

```bash
# Against current kubeconfig cluster (CRDs must be installed)
mise run run
```

The controller runs outside the cluster but connects via kubeconfig. Useful for debugging with breakpoints.

## Project Structure

```
cfgate/
  api/v1alpha1/           # CRD type definitions
  cmd/
    manager/              # Controller entrypoint
    cleanup/              # E2E resource cleanup utility
  internal/
    controller/           # Reconcilers (tunnel, dns, access, gateway, httproute)
    controller/annotations/ # Annotation parsing and validation
    controller/features/  # Feature gate detection
    controller/status/    # Status condition helpers
    cloudflare/           # Cloudflare API client abstraction
    cloudflared/          # cloudflared config and deployment builders
  config/
    crd/                  # Generated CRD manifests
    default/              # Kustomize overlay for deployment
    manager/              # Controller deployment resources
    rbac/                 # RBAC resources
  charts/cfgate/          # Helm chart
  test/e2e/               # E2E test suite
  examples/               # Usage examples
  docs/                   # Documentation
  hack/                   # Build utilities
```

## Commits

We use conventional-ish prefixes: `feat:`, `fix:`, `chore:`, `ci:`, `docs:`, `test:`, `refactor:`, `perf:`, `infra:`, `build:`

No enforcement tooling. No scopes required. Write a clear subject line under 72 characters. If your change has a body, bullet points are preferred.

Scopes are optional and used when the change targets a specific subsystem:

```
fix(controller): correct DNS record drift detection
test(e2e): add multi-zone ownership verification
```

For contributor PRs, the maintainer squash-merges with a clean conventional subject line. You do not need to rewrite your branch history.

## Changelog

Release notes are generated via [git-cliff](https://git-cliff.org/) from commit history. Configuration is in `cliff.toml` at the project root. You do not need to update CHANGELOG.md manually.

## Code Style

- Follow existing patterns in the codebase
- Run `mise run lint` before submitting (golangci-lint enforces style)
- Run `mise run format` to auto-format
- Use structured logging via `logr` (controller-runtime convention)
- Doc comments on all exported types and functions

## License

By contributing, you agree that your contributions will be licensed under the [Apache 2.0 License](LICENSE).
