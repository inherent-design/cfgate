# cfgate

[![Latest Release](https://img.shields.io/github/v/release/inherent-design/cfgate?style=flat)](https://github.com/inherent-design/cfgate/releases/latest) [![Image](https://img.shields.io/github/v/release/inherent-design/cfgate?style=flat&label=image&logo=docker&logoColor=white&color=2496ED)](https://github.com/orgs/inherent-design/packages/container/package/cfgate) [![Helm Chart](https://img.shields.io/badge/chart-GHCR-0F1689?style=flat&logo=helm&logoColor=white)](https://github.com/orgs/inherent-design/packages/container/package/charts%2Fcfgate)

[![Build Status](https://img.shields.io/github/actions/workflow/status/inherent-design/cfgate/ci.yml?style=flat)](https://github.com/inherent-design/cfgate/actions/workflows/ci.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/inherent-design/cfgate)](https://goreportcard.com/report/github.com/inherent-design/cfgate) [![Go Reference](https://pkg.go.dev/badge/github.com/inherent-design/cfgate.svg)](https://pkg.go.dev/cfgate.io/cfgate/)

Gateway API-native Kubernetes operator for Cloudflare Tunnel, DNS, and Access management.

cfgate replaces legacy Ingress-based Cloudflare Tunnel operators with a modern, composable architecture built on [Gateway API](https://gateway-api.sigs.k8s.io/).

## Features

- **CloudflareTunnel**: Tunnel lifecycle management, cloudflared deployment, credential handling
- **CloudflareDNS**: DNS record sync from Gateway API routes or explicit hostnames, multi-zone support, ownership tracking
- **CloudflareAccessPolicy**: Zero-trust Access application and policy configuration
- **Gateway API**: Native GatewayClass/Gateway/HTTPRoute support with per-route annotations

## Install

### Kustomize

```bash
# Gateway API CRDs (required)
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml

# cfgate controller + CRDs
kubectl apply -f https://github.com/inherent-design/cfgate/releases/latest/download/install.yaml
```

### Helm

```bash
helm install cfgate oci://ghcr.io/inherent-design/charts/cfgate \
  --namespace cfgate-system --create-namespace
```

## Quick Start

### 1. Create credentials

```bash
kubectl create secret generic cloudflare-credentials \
  -n cfgate-system \
  --from-literal=CLOUDFLARE_API_TOKEN=<your-token>
```

### 2. Create a tunnel

```yaml
apiVersion: cfgate.io/v1alpha1
kind: CloudflareTunnel
metadata:
  name: my-tunnel
  namespace: cfgate-system
spec:
  tunnel:
    name: my-tunnel
  cloudflare:
    accountId: "<account-id>"
    secretRef:
      name: cloudflare-credentials
  cloudflared:
    replicas: 2
```

### 3. Create GatewayClass and Gateway

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: cfgate
spec:
  controllerName: cfgate.io/cloudflare-tunnel-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: cloudflare-tunnel
  namespace: cfgate-system
  annotations:
    cfgate.io/tunnel-ref: cfgate-system/my-tunnel
spec:
  gatewayClassName: cfgate
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
```

### 4. Set up DNS sync

```yaml
apiVersion: cfgate.io/v1alpha1
kind: CloudflareDNS
metadata:
  name: my-dns
  namespace: cfgate-system
spec:
  tunnelRef:
    name: my-tunnel
  zones:
    - name: example.com
  source:
    gatewayRoutes:
      enabled: true
```

### 5. Expose a service

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app
  namespace: default
spec:
  parentRefs:
    - name: cloudflare-tunnel
      namespace: cfgate-system
  hostnames:
    - app.example.com
  rules:
    - backendRefs:
        - name: my-service
          port: 80
```

cfgate automatically:
- Creates a CNAME record `app.example.com` -> `{tunnelId}.cfargotunnel.com`
- Adds a cloudflared ingress rule routing `app.example.com` -> `http://my-service.default.svc:80`
- Manages ownership TXT records for safe multi-cluster deployments

## CRDs

### CloudflareTunnel

Manages tunnel lifecycle and cloudflared deployment. Zone-agnostic: a single tunnel can serve any number of domains.

| Field                       | Description                                    |
| --------------------------- | ---------------------------------------------- |
| `spec.tunnel.name`          | Tunnel name (idempotent: creates or adopts)    |
| `spec.cloudflare.accountId` | Cloudflare account ID                          |
| `spec.cloudflare.secretRef` | Secret containing API token                    |
| `spec.cloudflared.replicas` | Number of cloudflared pods (1-10)              |
| `spec.cloudflared.protocol` | Transport protocol: `auto`, `quic`, `http2`    |
| `spec.originDefaults`       | Default origin connection settings             |
| `spec.fallbackTarget`       | Catch-all service (default: `http_status:404`) |

### CloudflareDNS

Syncs DNS records independently from tunnel lifecycle. Supports multiple zones per resource.

| Field                       | Description                                             |
| --------------------------- | ------------------------------------------------------- |
| `spec.tunnelRef`            | Reference to CloudflareTunnel (CNAME target)            |
| `spec.externalTarget`       | Alternative: non-tunnel DNS target                      |
| `spec.zones[]`              | Zones to manage (by name or explicit ID)                |
| `spec.source.gatewayRoutes` | Auto-discover hostnames from HTTPRoutes                 |
| `spec.source.explicit[]`    | Explicit hostname list                                  |
| `spec.policy`               | Lifecycle policy: `sync`, `upsert-only`, `create-only`  |
| `spec.ownership`            | TXT record ownership tracking (external-dns compatible) |
| `spec.defaults`             | Default TTL and proxied settings                        |

### CloudflareAccessPolicy

Manages Cloudflare Access applications and policies for zero-trust authentication.

| Field                 | Description                                 |
| --------------------- | ------------------------------------------- |
| `spec.targetRef`      | Gateway API resource to protect             |
| `spec.hostnames[]`    | Explicit hostnames for Access application   |
| `spec.tunnelRef`      | Tunnel reference for credential inheritance |
| `spec.authentication` | Identity provider configuration             |
| `spec.rules[]`        | Access rules (allow, deny, bypass)          |

#### Access Application Settings

| Field                                          | Type       | Description                                                                                              |
| ---------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------- |
| `spec.application.allowedIdps`                 | `[]string` | Restrict identity providers by UUID. Without this, all configured IdPs (including OTP) are shown.        |
| `spec.application.autoRedirectToIdentity`      | `bool`     | Skip IdP selection page when a single IdP is in `allowedIdps`.                                           |
| `spec.application.appLauncherVisible`          | `*bool`    | Show application in Cloudflare App Launcher. Defaults to `true`.                                         |
| `spec.application.corsHeaders`                 | object     | CORS configuration (see below). Mutually exclusive with `optionsPreflightBypass`.                        |
| `spec.application.optionsPreflightBypass`      | `bool`     | Allow OPTIONS preflight requests to bypass Access auth. Mutually exclusive with `corsHeaders`.           |
| `spec.application.pathCookieAttribute`         | `bool`     | Scope Access JWT cookie to the application path instead of hostname.                                     |
| `spec.application.serviceAuth401Redirect`      | `bool`     | Return HTTP 401 instead of redirect for service auth blocks.                                             |
| `spec.application.customNonIdentityDenyUrl`    | `string`   | Redirect URL for non-identity (service token) denials.                                                   |
| `spec.application.readServiceTokensFromHeader` | `string`   | Read service token from a single header (JSON with `cf-access-client-id` and `cf-access-client-secret`). |

#### CORS Headers

```yaml
spec:
  application:
    corsHeaders:
      allowAllHeaders: false
      allowAllMethods: false
      allowAllOrigins: false
      allowCredentials: true
      allowedHeaders: ["Authorization", "Content-Type"]
      allowedMethods: ["GET", "POST", "OPTIONS"]
      allowedOrigins: ["https://app.example.com"]
      maxAge: 86400
```

### HTTPRoute Annotations

Per-route configuration via annotations on Gateway API routes:

| Annotation                          | Values              | Default    | Description           |
| ----------------------------------- | ------------------- | ---------- | --------------------- |
| `cfgate.io/origin-protocol`         | `http`, `https`     | `http`     | Backend protocol      |
| `cfgate.io/origin-ssl-verify`       | `true`, `false`     | `true`     | TLS verification      |
| `cfgate.io/origin-connect-timeout`  | duration            | `30s`      | Connection timeout    |
| `cfgate.io/origin-http-host-header` | hostname            | --         | Host header override  |
| `cfgate.io/origin-server-name`      | hostname            | --         | TLS SNI server name   |
| `cfgate.io/origin-ca-pool`          | path                | --         | CA certificate pool   |
| `cfgate.io/origin-http2`            | `true`, `false`     | `false`    | HTTP/2 to origin      |
| `cfgate.io/ttl`                     | `1`-`86400`         | `1` (auto) | DNS record TTL        |
| `cfgate.io/cloudflare-proxied`      | `true`, `false`     | `true`     | Cloudflare proxy      |
| `cfgate.io/access-policy`           | `name` or `ns/name` | --         | Access policy ref     |
| `cfgate.io/hostname`                | RFC 1123 hostname   | --         | Override/set hostname |

## Requirements

### Cloudflare API Token

| Scope   | Permission                      | Used By                              |
| ------- | ------------------------------- | ------------------------------------ |
| Account | Cloudflare Tunnel: Edit         | CloudflareTunnel                     |
| Account | Access: Apps and Policies: Edit | CloudflareAccessPolicy               |
| Account | Access: Service Tokens: Edit    | CloudflareAccessPolicy               |
| Account | Account Settings: Read          | CloudflareTunnel (accountName only)* |
| Zone    | DNS: Edit                       | CloudflareDNS                        |

*Only required when using `spec.cloudflare.accountName` instead of `accountId`.

### Kubernetes

- Kubernetes 1.26+
- Gateway API v1.4.1+ CRDs installed
- cluster-admin access for CRD installation

## Multi-Zone Support

cfgate natively supports multiple zones and domains. cloudflared is zone-agnostic: it connects via tunnel UUID and routes any hostname that resolves to the tunnel domain. Zone management is handled entirely by the CloudflareDNS CRD:

```yaml
spec:
  zones:
    - name: example.com
    - name: example.org
    - name: staging.co.uk
      id: "optional-zone-id"  # skips API lookup
```

The controller extracts the zone from each hostname using the public suffix list, matches it against configured zones, and syncs records to the correct zone. Token permissions determine which zones are accessible.

## Examples

| Example                                 | Description                                    |
| --------------------------------------- | ---------------------------------------------- |
| [basic](examples/basic)                 | Single tunnel + gateway + DNS sync             |
| [multi-service](examples/multi-service) | Multiple services, one tunnel, Access policies |
| [with-rancher](examples/with-rancher)   | Rancher 2.14+ integration                      |

## Service Mesh Integration

cfgate works alongside service mesh implementations that use Gateway API (Istio, Envoy Gateway, etc.). Each mesh manages its own GatewayClass while cfgate manages `cfgate.io/cloudflare-tunnel-controller`.

### Kiali

[Kiali](https://kiali.io/) only recognizes Istio GatewayClasses by default. If you use Kiali for observability alongside cfgate, add the `cfgate` GatewayClass to Kiali's configuration to suppress KIA1504 validation warnings:

**Kiali CR:**

```yaml
spec:
  external_services:
    istio:
      gateway_api_classes:
        - class_name: "istio"
          name: "Istio"
        - class_name: "cfgate"
          name: "cfgate"
```

**Kiali ConfigMap** (if not using the Kiali Operator):

```yaml
external_services:
  istio:
    gateway_api_classes:
      - class_name: "istio"
        name: "Istio"
      - class_name: "cfgate"
        name: "cfgate"
```

> **Note:** Setting `gateway_api_classes` explicitly replaces Kiali's auto-discovery. Include all GatewayClasses you want Kiali to recognize (e.g., `istio`, `istio-remote`, `cfgate`).

See the [Kiali CR Reference](https://kiali.io/docs/configuration/kialis.kiali.io/) for all configuration options.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, secrets configuration, and contribution guidelines.

See [docs/TESTING.md](docs/TESTING.md) for E2E test strategy, environment variables, and test execution.

See [CHANGELOG.md](CHANGELOG.md) for release history (generated via [git-cliff](https://git-cliff.org/)).

```bash
brew install mise
mise install       # Install toolchain
mise tasks         # List available tasks
```

## License

[Apache 2.0](LICENSE)
