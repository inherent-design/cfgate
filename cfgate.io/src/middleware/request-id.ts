import { createMiddleware } from 'hono/factory'
import type { AppEnv } from '../types.js'

/**
 * Request ID middleware
 *
 * Sets c.var.requestId from cf-ray header or generates UUID.
 * Also echoes X-Request-Id in response headers.
 */
export const requestIdMiddleware = createMiddleware<AppEnv>(async (c, next) => {
  // Use cf-ray if available, otherwise generate UUID
  const cfRay = c.req.header('cf-ray')
  const requestId = cfRay?.split('-')[0] ?? crypto.randomUUID()

  c.set('requestId', requestId)

  await next()

  // Echo request ID in response
  c.res.headers.set('X-Request-Id', requestId)
})
