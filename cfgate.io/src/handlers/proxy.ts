import type { Context } from 'hono'
import type { AppEnv } from '../types.js'

/**
 * GitHub release base URL
 */
const GITHUB_RELEASE_BASE = 'https://github.com/inherent-design/cfgate/releases/latest/download'

/**
 * Path mapping for proxy routes
 */
export const PROXY_PATHS: Record<string, string> = {
  '/install.yaml': '/install.yaml',
  '/crds.yaml': '/crds.yaml',
  '/crds/tunnel.yaml': '/crds/cloudflaretunnels.yaml',
  '/crds/dns.yaml': '/crds/cloudflarednses.yaml',
  '/crds/access.yaml': '/crds/cloudflareaccesspolicies.yaml',
}

/**
 * GitHub release proxy handler
 *
 * Proxies install manifests and CRDs from GitHub releases.
 * Uses cf object for edge caching with 1h TTL.
 */
export async function proxyHandler(c: Context<AppEnv>): Promise<Response> {
  c.var.logCtx.handler = 'proxy'
  c.var.logCtx.upstream = 'github'

  const path = c.req.path
  const assetPath = PROXY_PATHS[path]

  if (!assetPath) {
    // Path not in allowed proxy paths
    return c.text('Not Found', 404)
  }

  const upstreamUrl = `${GITHUB_RELEASE_BASE}${assetPath}`

  try {
    const response = await fetch(upstreamUrl, {
      cf: {
        cacheEverything: true,
        cacheTtl: 3600, // 1 hour
      },
      signal: AbortSignal.timeout(30000), // 30s timeout
    })

    if (!response.ok) {
      if (response.status === 404) {
        return c.text('Not Found', 404)
      }
      // Upstream error
      return c.text('Service Unavailable', 503)
    }

    // Stream response body with appropriate headers
    return new Response(response.body, {
      status: response.status,
      headers: {
        'Content-Type': 'application/yaml; charset=utf-8',
        'Cache-Control': 'public, max-age=3600',
      },
    })
  } catch (error) {
    // Network error or timeout
    c.var.logCtx.error = {
      name: error instanceof Error ? error.name : 'Error',
      message: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
    }
    return c.text('Service Unavailable', 503)
  }
}

/**
 * Check if path is a proxy route
 */
export function _isProxyPath(path: string): boolean {
  return path in PROXY_PATHS
}
