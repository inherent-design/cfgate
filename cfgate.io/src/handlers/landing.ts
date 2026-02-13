import type { Context } from 'hono'
import type { AppEnv } from '../types.js'

export async function landingHandler(c: Context<AppEnv>): Promise<Response> {
  c.var.logCtx.handler = 'landing'

  if (c.env.ASSETS) {
    const asset = await c.env.ASSETS.fetch(c.req.raw)
    return new Response(asset.body, asset)
  }

  return c.text('cfgate', 200)
}
