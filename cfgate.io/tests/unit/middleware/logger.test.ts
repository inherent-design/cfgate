import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Hono } from 'hono'
import { loggerMiddleware } from '../../../src/middleware/logger.js'
import type { AppEnv } from '../../../src/types.js'

describe('loggerMiddleware', () => {
  let consoleSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleSpy.mockRestore()
  })

  it('initializes log context with request info', async () => {
    const app = new Hono<AppEnv>()
    app.use('*', loggerMiddleware)
    app.get('/test', (c) => {
      // Verify logCtx is initialized
      expect(c.var.logCtx).toBeDefined()
      expect(c.var.logCtx.method).toBe('GET')
      expect(c.var.logCtx.path).toBe('/test')
      expect(c.var.logCtx.timestamp).toBeDefined()
      return c.text('ok')
    })

    await app.request('/test')
  })

  it('emits JSON log after response', async () => {
    const app = new Hono<AppEnv>()
    app.use('*', loggerMiddleware)
    app.get('/test', (c) => c.text('ok'))

    await app.request('/test')

    expect(consoleSpy).toHaveBeenCalledTimes(1)
    const logArg = consoleSpy.mock.calls[0]?.[0]
    expect(typeof logArg).toBe('string')

    const parsed = JSON.parse(logArg as string)
    expect(parsed.method).toBe('GET')
    expect(parsed.path).toBe('/test')
    expect(parsed.status).toBe(200)
    expect(typeof parsed.durationMs).toBe('number')
  })

  it('captures cf-ray header', async () => {
    const app = new Hono<AppEnv>()
    app.use('*', loggerMiddleware)
    app.get('/test', (c) => c.text('ok'))

    await app.request('/test', {
      headers: { 'cf-ray': 'abc123-LAX' },
    })

    const logArg = consoleSpy.mock.calls[0]?.[0]
    const parsed = JSON.parse(logArg as string)
    expect(parsed.cfRay).toBe('abc123-LAX')
  })
})
