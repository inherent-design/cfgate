# Multi-Service Example

Multiple services exposed through a single Cloudflare tunnel.

```
api.example.com   ──┐
                    ├──▶ cloudflared ──▶ Cloudflare Edge
web.example.com   ──┘
```

## Quick Start

```bash
# 1. Install cfgate (see basic example)

# 2. Edit configuration files
# - tunnel.yaml: set accountId
# - dns.yaml: set zones[].name
# - httproutes.yaml: set hostnames

# 3. Deploy
kubectl apply -k examples/multi-service
```

## Components

- One `CloudflareTunnel` with 2 replicas
- One `Gateway` shared by all routes
- One `CloudflareDNS` watching all HTTPRoutes
- Two services: `api` and `web`
- Two HTTPRoutes with different hostnames

## Adding Services

1. Add deployment + service to `services.yaml`
2. Add HTTPRoute to `httproutes.yaml`
3. DNS record created automatically

## Cleanup

```bash
kubectl delete -k examples/multi-service
```
