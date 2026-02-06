import { createMiddleware } from 'hono/factory'
import type { AppEnv, LogContext } from '../types.js'

/**
 * Wide-event logging middleware
 *
 * Initializes log context before handler, emits single JSON log after response.
 * Context is accumulated throughout request lifecycle via c.var.logCtx.
 */
export const loggerMiddleware = createMiddleware<AppEnv>(async (c, next) => {
  const start = Date.now()

  // Initialize log context
  const logCtx: LogContext = {
    requestId: '', // Will be set by requestIdMiddleware after this runs
    cfRay: c.req.header('cf-ray') ?? '',
    method: c.req.method,
    path: c.req.path,
    timestamp: new Date().toISOString(),
  }

  c.set('logCtx', logCtx)

  await next()

  // Update log context with response info
  logCtx.requestId = c.var.requestId ?? logCtx.requestId
  logCtx.durationMs = Date.now() - start
  logCtx.status = c.res.status

  // Emit single JSON log line
  console.log(JSON.stringify(logCtx))
})
