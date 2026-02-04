# cfgate

Kubernetes controller for Cloudflare Tunnel management using Gateway API.

## Features

- **CloudflareTunnel**: Manages tunnel lifecycle, deploys cloudflared pods
- **CloudflareDNS**: Syncs DNS records from Gateway API routes or explicit hostnames
- **CloudflareAccessPolicy**: Zero-trust Access application and policy management
- **Gateway API**: Native GatewayClass/Gateway/HTTPRoute support

## Install

```bash
# Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml

# cfgate controller
kubectl apply -f https://github.com/inherent-design/cfgate/releases/latest/download/install.yaml
```

## Quick Start

```bash
# 1. Create credentials
kubectl create secret generic cloudflare-credentials \
  -n cfgate-system \
  --from-literal=CLOUDFLARE_API_TOKEN=<token>

# 2. Create tunnel
cat <<EOF | kubectl apply -f -
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
EOF

# 3. Create gateway
cat <<EOF | kubectl apply -f -
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
    cfgate.io/dns-sync: enabled
spec:
  gatewayClassName: cfgate
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
EOF

# 4. Create DNS sync
cat <<EOF | kubectl apply -f -
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
      annotationFilter: "cfgate.io/dns-sync=enabled"
EOF

# 5. Expose a service
cat <<EOF | kubectl apply -f -
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
EOF
```

## Requirements

### Cloudflare

API Token permissions:

| Scope   | Permission                      | Used By                                    |
| ------- | ------------------------------- | ------------------------------------------ |
| Account | `Cloudflare Tunnel:Edit`        | CloudflareTunnel                           |
| Account | `Access:Service Tokens:Edit`    | CloudflareAccessPolicy                     |
| Account | `Access:Apps and Policies:Edit` | CloudflareAccessPolicy                     |
| Account | `Account Settings:Read`         | CloudflareTunnel (accountName resolution)* |
| Zone    | `Zone:Edit`                     | CloudflareDNS (zone ID lookup)             |
| Zone    | `DNS:Edit`                      | CloudflareDNS                              |

*Only required if using `spec.cloudflare.accountName` instead of `accountId`.

Environment variables (for E2E tests and local development):

| Variable                | Required   | Purpose                               |
| ----------------------- | ---------- | ------------------------------------- |
| `CLOUDFLARE_API_TOKEN`  | Yes        | API authentication                    |
| `CLOUDFLARE_ACCOUNT_ID` | Yes        | Account scope                         |
| `CLOUDFLARE_ZONE_NAME`  | For DNS    | Zone for DNS records                  |
| `CLOUDFLARE_IDP_ID`     | For Access | Identity provider for Access policies |

### Kubernetes

- Gateway API v1.4.1+ CRDs
- cluster-admin access

## Examples

| Example                                 | Description                        |
| --------------------------------------- | ---------------------------------- |
| [basic](examples/basic)                 | Single tunnel + gateway + DNS sync |
| [multi-service](examples/multi-service) | Multiple services, one tunnel      |
| [with-rancher](examples/with-rancher)   | Rancher 2.14+ integration          |

## Development

```bash
brew install mise
mise install
mise tasks
```

| Task                    | Description                                    |
| ----------------------- | ---------------------------------------------- |
| `mise run generate`     | Generate CRDs and DeepCopy                     |
| `mise run lint`         | Run golangci-lint                              |
| `mise run test`         | Run all tests (e2e, requires Cloudflare creds) |
| `mise run build`        | Build binary                                   |
| `mise run docker:build` | Build container image                          |
| `mise run deploy`       | Deploy to current cluster                      |

## License

Apache 2.0
