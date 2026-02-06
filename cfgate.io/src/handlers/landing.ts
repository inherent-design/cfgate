import type { Context } from 'hono'
import type { AppEnv } from '../types.js'

/**
 * Landing page handler
 *
 * Returns simple "Hello World" for browser requests to root.
 * Future placeholder for SPA.
 */
export function landingHandler(c: Context<AppEnv>): Response {
  c.var.logCtx.handler = 'landing'

  return c.text('Hello World')
}
