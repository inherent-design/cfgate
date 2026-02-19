> ⚠️ MOVED: https://github.com/cfgate/cfgate

# cfgate

[![Latest Release](https://img.shields.io/github/v/release/inherent-design/cfgate?style=flat)](https://github.com/inherent-design/cfgate/releases/latest) [![Image](https://img.shields.io/github/v/release/inherent-design/cfgate?style=flat&label=image&logo=docker&logoColor=white&color=2496ED)](https://github.com/orgs/inherent-design/packages/container/package/cfgate) [![Helm Chart](https://img.shields.io/badge/chart-GHCR-0F1689?style=flat&logo=helm&logoColor=white)](https://github.com/orgs/inherent-design/packages/container/package/charts%2Fcfgate)

[![Build Status](https://img.shields.io/github/actions/workflow/status/inherent-design/cfgate/ci.yml?style=flat)](https://github.com/inherent-design/cfgate/actions/workflows/ci.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/inherent-design/cfgate)](https://goreportcard.com/report/github.com/inherent-design/cfgate) [![Go Reference](https://pkg.go.dev/badge/github.com/inherent-design/cfgate.svg)](https://pkg.go.dev/cfgate.io/cfgate/)

Gateway API-native Kubernetes operator for Cloudflare Tunnel, DNS, and Access management.

> **Gateway API** is the Kubernetes successor to Ingress, providing a role-oriented, portable, and expressive API for service networking. cfgate replaces legacy Ingress-based Cloudflare operators with a modern, composable architecture built on [Gateway API](https://gateway-api.sigs.k8s.io/). See [Gateway API Primer](docs/gateway-api-primer.md) for an introduction.

## Why cfgate?

Cloudflare Tunnels create outbound-only connections from your cluster to Cloudflare's edge network. Kubernetes workloads are never exposed via public IP addresses, load balancers, or traditional ingress controllers. cfgate manages this infrastructure declaratively:

- **No public IP exposure** - All traffic routes through Cloudflare Tunnels (outbound-only connections from cluster to edge)
- **Policy-as-code** - Zero-trust access policies defined in version-controlled Kubernetes CRDs
- **Gateway API native** - Built on the CNCF standard replacing Ingress, not a proprietary abstraction
- **Unified operator** - Tunnel lifecycle, DNS records, and access policies managed through a single controller

## Features

| Feature             | Status | Description                                                                 |
| ------------------- | ------ | --------------------------------------------------------------------------- |
| Tunnel lifecycle    | Stable | CRD-driven tunnel creation, credential management, cloudflared deployment   |
| DNS sync            | Stable | Multi-zone DNS record sync from Gateway API routes or explicit hostnames    |
| Access policies     | Stable | Zero-trust Cloudflare Access application and policy management              |
| Gateway API         | Stable | Native GatewayClass, Gateway, HTTPRoute, GRPCRoute support                  |
| TCPRoute / UDPRoute | Stub   | Controllers registered, implementation planned for v0.2.0                   |
| Per-route config    | Stable | Origin protocol, TLS, timeouts, DNS settings via route annotations          |
| Multi-zone DNS      | Stable | Single CloudflareDNS resource manages records across multiple zones         |
| Service mesh compat | Stable | Works alongside Istio, Envoy Gateway, and other Gateway API implementations |

## CRDs

### CloudflareTunnel

Manages tunnel lifecycle and cloudflared deployment. Zone-agnostic: a single tunnel serves any number of domains.

| Field                         | Description                                                |
| ----------------------------- | ---------------------------------------------------------- |
| `spec.tunnel.name`            | Tunnel name (idempotent: creates or adopts)                |
| `spec.cloudflare.accountId`   | Cloudflare account ID                                      |
| `spec.cloudflare.accountName` | Alternative: account name (looked up via API)              |
| `spec.cloudflare.secretRef`   | Secret containing API token                                |
| `spec.cloudflared.replicas`   | Number of cloudflared pods (default: 2)                    |
| `spec.cloudflared.protocol`   | Transport: `auto` (default), `quic`, `http2`               |
| `spec.cloudflared.image`      | Container image (default: `cloudflare/cloudflared:latest`) |
| `spec.originDefaults`         | Default origin connection settings                         |
| `spec.fallbackTarget`         | Catch-all (default: `http_status:404`)                     |
| `spec.fallbackCredentialsRef` | Fallback credentials for deletion cleanup                  |

> Full reference: [docs/cloudflare-tunnel.md](docs/cloudflare-tunnel.md)

### CloudflareDNS

Syncs DNS records independently from tunnel lifecycle. Supports multiple zones per resource.

| Field                                        | Description                                                                                         |
| -------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `spec.tunnelRef`                             | Reference to CloudflareTunnel (CNAME target)                                                        |
| `spec.externalTarget`                        | Alternative: non-tunnel DNS target (CNAME/A/AAAA)                                                   |
| `spec.zones[]`                               | Zones to manage (by name or explicit ID)                                                            |
| `spec.source.gatewayRoutes`                  | Auto-discover hostnames from routes                                                                 |
| `spec.source.gatewayRoutes.annotationFilter` | Filter routes by annotation (user-defined, see [docs](docs/cloudflare-dns.md#annotation-filtering)) |
| `spec.source.explicit[]`                     | Explicit hostname list                                                                              |
| `spec.policy`                                | `sync` (default), `upsert-only`, `create-only`                                                      |
| `spec.cleanupPolicy`                         | Deletion behavior configuration                                                                     |
| `spec.ownership`                             | TXT record ownership tracking (external-dns compatible)                                             |

> Full reference: [docs/cloudflare-dns.md](docs/cloudflare-dns.md)

### CloudflareAccessPolicy

Manages Cloudflare Access applications and policies for zero-trust authentication.

| Field                | Description                                                  |
| -------------------- | ------------------------------------------------------------ |
| `spec.targetRef`     | Single Gateway API resource to protect                       |
| `spec.targetRefs[]`  | Multiple targets (mutually exclusive with targetRef, max 16) |
| `spec.cloudflareRef` | Cloudflare credentials (inherits from tunnel if omitted)     |
| `spec.application`   | Access Application settings (name, domain, IdP config, CORS) |
| `spec.policies[]`    | Access rules: allow, deny, bypass, non_identity (max 50)     |
| `spec.groupRefs[]`   | Reference Cloudflare Access Groups (by `cloudflareId`)       |
| `spec.serviceTokens` | Machine-to-machine auth tokens                               |
| `spec.mtls`          | Certificate-based authentication                             |

> Full reference: [docs/cloudflare-access-policy.md](docs/cloudflare-access-policy.md)

## Route Annotations

Per-route configuration via annotations on Gateway API route resources (HTTPRoute, TCPRoute, UDPRoute, GRPCRoute).

| Annotation                          | Values                            | Default         | Description                          |
| ----------------------------------- | --------------------------------- | --------------- | ------------------------------------ |
| `cfgate.io/origin-protocol`         | `http`, `https`, `tcp`\*, `udp`\* | route-dependent | Backend protocol                     |
| `cfgate.io/origin-ssl-verify`       | `true`, `false`                   | `true`          | TLS certificate verification         |
| `cfgate.io/origin-connect-timeout`  | duration                          | `30s`           | Connection timeout                   |
| `cfgate.io/origin-http-host-header` | hostname                          | --              | Host header override                 |
| `cfgate.io/origin-server-name`      | hostname                          | --              | TLS SNI server name                  |
| `cfgate.io/origin-ca-pool`          | path                              | --              | CA certificate pool                  |
| `cfgate.io/origin-http2`            | `true`, `false`                   | `false`         | HTTP/2 to origin                     |
| `cfgate.io/ttl`                     | `1`-`86400`                       | `1` (auto)      | DNS record TTL                       |
| `cfgate.io/cloudflare-proxied`      | `true`, `false`                   | `true`          | Cloudflare proxy                     |
| `cfgate.io/access-policy`           | `name` or `ns/name`               | --              | Access policy reference              |
| `cfgate.io/hostname`                | RFC 1123 hostname                 | --              | Override/set hostname**              |
| `cfgate.io/deletion-policy`         | `orphan`                          | --              | Skip Cloudflare cleanup on delete*** |

\* TCPRoute/UDPRoute controllers are stubs (v0.2.0). Default protocol is route-type dependent: `http` for HTTPRoute, `tcp` for TCPRoute, `udp` for UDPRoute.

\*\* Required for TCPRoute and UDPRoute (Gateway API has no hostnames field for these route types). Optional for HTTPRoute/GRPCRoute (overrides spec.hostnames).

\*\*\* Applies to CloudflareTunnel and CloudflareAccessPolicy resources, not routes.

> Full reference: [docs/annotations.md](docs/annotations.md)

## Credential Resolution

CloudflareAccessPolicy resolves Cloudflare API credentials through two paths:

1. **Explicit `cloudflareRef`**: Set `spec.cloudflareRef` with a secret reference and account ID.
2. **Inherited from tunnel**: Gateway target -> `cfgate.io/tunnel-ref` -> CloudflareTunnel -> `spec.cloudflare.secretRef`. For HTTPRoute targets: HTTPRoute -> parentRef -> Gateway -> same chain.

If neither path resolves, reconciliation fails with `CredentialsInvalid`.

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

### The cfgate-system Namespace

Helm creates the `cfgate-system` namespace via `--create-namespace`. For kustomize, the install manifest includes the namespace. CloudflareTunnel and CloudflareDNS resources typically live here. Routes and services can be in any namespace.

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

> **GatewayClass** defines which controller manages Gateways of this class. cfgate registers `cfgate.io/cloudflare-tunnel-controller` automatically. **Gateway** is the runtime instance that binds to a specific CloudflareTunnel via the `cfgate.io/tunnel-ref` annotation. You need both: the class (driver) and the instance (endpoint).

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

The Quick Start uses `gatewayRoutes.enabled: true` WITHOUT annotationFilter, which syncs ALL routes attached to the referenced tunnel's Gateways. To selectively sync specific routes, use the `annotationFilter` field. See [CloudflareDNS reference](docs/cloudflare-dns.md#annotation-filtering).

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

## Documentation

| Document                                                   | Description                                           |
| ---------------------------------------------------------- | ----------------------------------------------------- |
| [Gateway API Primer](docs/gateway-api-primer.md)           | Gateway API concepts for Ingress users                |
| [CloudflareTunnel](docs/cloudflare-tunnel.md)              | Full CRD reference                                    |
| [CloudflareDNS](docs/cloudflare-dns.md)                    | Full CRD reference, annotationFilter, ownership       |
| [CloudflareAccessPolicy](docs/cloudflare-access-policy.md) | Full CRD reference, rule types, credential resolution |
| [Annotations](docs/annotations.md)                         | Complete annotation reference                         |
| [Troubleshooting](docs/troubleshooting.md)                 | Diagnostic steps and solutions                        |
| [Testing](docs/TESTING.md)                                 | E2E test strategy                                     |
| [Contributing](CONTRIBUTING.md)                            | Development setup                                     |
| [Changelog](CHANGELOG.md)                                  | Release history                                       |

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

## Examples

| Example                                 | Description                                    |
| --------------------------------------- | ---------------------------------------------- |
| [basic](examples/basic)                 | Single tunnel + gateway + DNS sync             |
| [multi-service](examples/multi-service) | Multiple services, one tunnel, Access policies |
| [with-rancher](examples/with-rancher)   | Rancher 2.14+ integration                      |

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

## Troubleshooting

For detailed troubleshooting steps with diagnostic commands and expected outputs, see [docs/troubleshooting.md](docs/troubleshooting.md).

Quick reference:
- DNS not syncing -> Check CloudflareDNS status, tunnel readiness, annotationFilter
- GatewayClass not Accepted -> Verify controllerName is `cfgate.io/cloudflare-tunnel-controller`
- CredentialsInvalid -> Check credential resolution chain
- Stuck finalizers -> Use `cfgate.io/deletion-policy: orphan` or remove finalizer manually

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, secrets configuration, and contribution guidelines.

See [docs/TESTING.md](docs/TESTING.md) for E2E test strategy, environment variables, and test execution.

```bash
brew install mise
mise install       # Install toolchain
mise tasks         # List available tasks
```

## License

[Apache 2.0](LICENSE)
