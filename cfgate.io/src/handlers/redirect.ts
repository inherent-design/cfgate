import type { Context } from 'hono'
import type { AppEnv } from '../types.js'

/**
 * Browser redirect handler
 *
 * Redirects browser requests (non go-get) to pkg.go.dev documentation.
 * Preserves path suffix in redirect target.
 */
export function redirectHandler(c: Context<AppEnv>): Response {
  c.var.logCtx.handler = 'redirect'

  const path = c.req.path
  // /cfgate -> cfgate.io/cfgate
  // /cfgate/api/v1alpha1 -> cfgate.io/cfgate/api/v1alpha1
  const pkgPath = path.startsWith('/cfgate') ? path : '/cfgate'
  const url = `https://pkg.go.dev/cfgate.io${pkgPath}`

  return c.redirect(url, 302)
}
