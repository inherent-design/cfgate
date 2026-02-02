# cfgate

Kubernetes controller for Cloudflare Tunnel management using Gateway API.

## Features

- **CloudflareTunnel**: Manages tunnel lifecycle, deploys cloudflared pods, syncs ingress configuration
- **CloudflareDNSSync**: Watches HTTPRoutes and creates CNAME records pointing to tunnel
- **Gateway API**: Native GatewayClass/Gateway/HTTPRoute support

## Requirements

### Cloudflare

API Token with permissions:
- `Zone:DNS:Edit`
- `Zone:Zone:Read`
- `Account:Cloudflare Tunnel:Edit`

### Kubernetes

- Gateway API CRDs installed
- cluster-admin access

## Install

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml
kubectl apply -f https://github.com/inherent-design/cfgate/releases/latest/download/install.yaml
```

Create credentials:

```bash
kubectl create secret generic cloudflare-credentials \
  -n cfgate-system \
  --from-literal=CLOUDFLARE_API_TOKEN=<token> \
  --from-literal=CLOUDFLARE_ACCOUNT_ID=<account-id>
```

## Usage

See [examples/](examples/) for complete configurations:

| Example | Description |
|---------|-------------|
| [basic](examples/basic) | Tunnel + gateway + DNS sync |
| [multi-service](examples/multi-service) | Multiple services, one tunnel |
| [with-rancher](examples/with-rancher) | Rancher 2.14+ integration |

### Gateway Annotations

```yaml
metadata:
  annotations:
    cfgate.io/tunnel-ref: <namespace>/<tunnel-name>
    cfgate.io/dns-sync: enabled
```

## Development

```bash
brew install mise
mise install
mise tasks
```

| Task | Description |
|------|-------------|
| `mise run generate` | Generate CRDs and DeepCopy |
| `mise run lint` | Run golangci-lint |
| `mise run test` | Run all tests (e2e, requires Cloudflare creds) |
| `mise run build` | Build binary |
| `mise run docker:build` | Build container image |
| `mise run deploy` | Deploy to current cluster |

### Local Testing

```bash
mise run cluster:create
mise run deploy
mise run test
mise run cluster:delete
```

## License

Apache 2.0
