import type { Context } from 'hono'
import type { AppEnv } from '../types.js'

/**
 * Module configuration
 */
const MODULE_PATH = 'cfgate.io/cfgate'
const REPO_URL = 'https://github.com/inherent-design/cfgate'
const DEFAULT_BRANCH = 'main'

/**
 * Generate go-import HTML response
 *
 * Returns HTML with go-import and go-source meta tags for Go module resolution.
 * Same meta tags returned for all paths under /cfgate/* (Go protocol requirement).
 */
export function generateVanityHTML(): string {
  return `<!DOCTYPE html>
<html>
<head>
<meta name="go-import" content="${MODULE_PATH} git ${REPO_URL}">
<meta name="go-source" content="${MODULE_PATH} ${REPO_URL} ${REPO_URL}/tree/${DEFAULT_BRANCH}{/dir} ${REPO_URL}/blob/${DEFAULT_BRANCH}{/dir}/{file}#L{line}">
</head>
<body>Redirecting to docs...</body>
</html>`
}

/**
 * Vanity import handler
 *
 * Handles ?go-get=1 requests for Go module resolution.
 * Returns HTML with go-import meta tags.
 */
export function vanityHandler(c: Context<AppEnv>): Response {
  c.var.logCtx.handler = 'vanity'

  const html = generateVanityHTML()

  return c.html(html, 200, {
    'Cache-Control': 'public, max-age=86400',
  })
}

/**
 * Check if request is a go-get request
 */
export function isGoGetRequest(c: Context<AppEnv>): boolean {
  return c.req.query('go-get') === '1'
}
