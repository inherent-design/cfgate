# Changelog

All notable changes to cfgate are documented in this file.


## [Unreleased]

### Features

- Alpha.6 comprehensive stabilization (unreleased)

### Bug Fixes

- **(controller)** Alpha.6 reconcile, deletion, and API stabilization
- **(controller)** Logging guard and em-dash removal

### Testing

- **(e2e)** Alpha.6 coverage expansion and 94/94 stabilization

### Maintenance

- Local dev fixes (docker cache, mise tasks)
- Reset kustomization.yaml after local deploy

## [0.1.0-alpha.5] - 2026-02-06

### Features

- Helm chart v1.0.1

### Bug Fixes

- Alpha.5 controller stabilization

### Infrastructure

- Cfgate.io v0.1.2 custom_domain for auto DNS

### Maintenance

- Local dev tasks + docs

## [0.1.0-alpha.4] - 2026-02-06

### Features

- Alpha.3 implementation
- CloudflareDNS CRD (composable architecture)
- Add helm chart v1.0.0

### Bug Fixes

- SA1019 events API migration + reconciliation bugs

### Testing

- Alpha.3 E2E suite (85/85 passing)

### Documentation

- Alpha.3 samples and examples
- Godoc comments and logging audit

### Infrastructure

- Initialize cfgate.io as wrangler
- Add version injection to builds
- Use kubectl kustomize
- Cfgate.io v0.1.1 with route fix
- Alpha.4 CI/CD improvements

### Maintenance

- Pin doc2go version
- Organize mise.toml
- Fix cfgate.io bootstrap

## [0.1.0-alpha.2] - 2026-02-02

### Bug Fixes

- Use release version tag in install.yaml

### Documentation

- Update README and examples for v0.1.0-alpha.1

## [0.1.0-alpha.1] - 2026-02-02

### Features

- Initial commit
- Add Dockerfile for container builds
- Add docs, ci, mise tooling

### Bug Fixes

- Remove deprecated Connections field (SA1019)
- Add kustomize directory structure

### Documentation

- Update Gateway API version and consolidate test tasks

### CI/CD

- Separate e2e, drop mise
- Bump golangci-lint to v2.8.0
- Update workflows; remove e2e
- Add path filter to pull_request trigger
- Remove workflow_dispatch from release

### Maintenance

- Clean CI workflows and dead code
