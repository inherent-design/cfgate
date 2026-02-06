import { Hono } from 'hono'
import type { AppEnv } from './types.js'

// Middleware
import { loggerMiddleware } from './middleware/logger.js'
import { requestIdMiddleware } from './middleware/request-id.js'

// Handlers
import { errorHandler, notFoundHandler } from './handlers/error.js'
import { landingHandler } from './handlers/landing.js'
import { PROXY_PATHS, proxyHandler } from './handlers/proxy.js'
import { redirectHandler } from './handlers/redirect.js'
import { isGoGetRequest, vanityHandler } from './handlers/vanity.js'

/**
 * cfgate.io Cloudflare Worker
 *
 * Serves:
 * - Go vanity imports for cfgate.io/cfgate
 * - GitHub release proxy for install manifests and CRDs
 * - Browser redirects to pkg.go.dev
 */
const app = new Hono<AppEnv>()

// Middleware stack (order matters)
app.use('*', loggerMiddleware)
app.use('*', requestIdMiddleware)

// Error handlers
app.onError(errorHandler)
app.notFound(notFoundHandler)

// Proxy routes (explicit paths, checked first)
for (const path in PROXY_PATHS) {
  app.get(path, proxyHandler)
}

// Root path handler
app.get('/', (c) => {
  if (isGoGetRequest(c)) {
    // Go module verification request
    return vanityHandler(c)
  }
  // Browser request
  return landingHandler(c)
})

// /cfgate routes (conditional on go-get query param)
app.get('/cfgate', (c) => {
  if (isGoGetRequest(c)) {
    return vanityHandler(c)
  }
  return redirectHandler(c)
})

// /cfgate/* wildcard (conditional on go-get query param)
app.get('/cfgate/*', (c) => {
  if (isGoGetRequest(c)) {
    return vanityHandler(c)
  }
  return redirectHandler(c)
})

// Export app type for RPC (if needed in future)
export type AppType = typeof app

export default app
