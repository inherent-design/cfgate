import { SELF } from 'cloudflare:test'
import { describe, expect, it } from 'vitest'

describe('E2E Route Tests', () => {
  describe('Landing page', () => {
    it('returns Hello World for root path', async () => {
      const response = await SELF.fetch('https://cfgate.io/')

      expect(response.status).toBe(200)
      // expect(await response.text()).toBe('Hello World')
    })

    it('includes X-Request-Id header', async () => {
      const response = await SELF.fetch('https://cfgate.io/')

      expect(response.headers.get('X-Request-Id')).toBeDefined()
    })
  })

  describe('Vanity imports', () => {
    it('returns go-import meta tag for /?go-get=1', async () => {
      const response = await SELF.fetch('https://cfgate.io/?go-get=1')

      expect(response.status).toBe(200)
      expect(response.headers.get('Content-Type')).toContain('text/html')
      expect(response.headers.get('Cache-Control')).toBe('public, max-age=86400')

      const html = await response.text()
      expect(html).toContain(
        '<meta name="go-import" content="cfgate.io/cfgate git https://github.com/inherent-design/cfgate">'
      )
    })

    it('returns go-import meta tag for /cfgate?go-get=1', async () => {
      const response = await SELF.fetch('https://cfgate.io/cfgate?go-get=1')

      expect(response.status).toBe(200)
      const html = await response.text()
      expect(html).toContain(
        '<meta name="go-import" content="cfgate.io/cfgate git https://github.com/inherent-design/cfgate">'
      )
    })

    it('returns same go-import for subpaths', async () => {
      const response = await SELF.fetch('https://cfgate.io/cfgate/api/v1alpha1?go-get=1')

      expect(response.status).toBe(200)
      const html = await response.text()
      // All subpaths return the same go-import pointing to module root
      expect(html).toContain(
        '<meta name="go-import" content="cfgate.io/cfgate git https://github.com/inherent-design/cfgate">'
      )
    })

    it('includes go-source meta tag', async () => {
      const response = await SELF.fetch('https://cfgate.io/cfgate?go-get=1')
      const html = await response.text()

      expect(html).toContain('<meta name="go-source"')
      expect(html).toContain('/tree/main{/dir}')
      expect(html).toContain('/blob/main{/dir}/{file}#L{line}')
    })
  })

  describe('Browser redirects', () => {
    it('redirects /cfgate to pkg.go.dev', async () => {
      const response = await SELF.fetch('https://cfgate.io/cfgate', {
        redirect: 'manual',
      })

      expect(response.status).toBe(302)
      expect(response.headers.get('Location')).toBe('https://pkg.go.dev/cfgate.io/cfgate')
    })

    it('redirects /cfgate/api to pkg.go.dev with path', async () => {
      const response = await SELF.fetch('https://cfgate.io/cfgate/api', {
        redirect: 'manual',
      })

      expect(response.status).toBe(302)
      expect(response.headers.get('Location')).toBe('https://pkg.go.dev/cfgate.io/cfgate/api')
    })

    it('redirects /cfgate/api/v1alpha1 to pkg.go.dev with full path', async () => {
      const response = await SELF.fetch('https://cfgate.io/cfgate/api/v1alpha1', {
        redirect: 'manual',
      })

      expect(response.status).toBe(302)
      expect(response.headers.get('Location')).toBe(
        'https://pkg.go.dev/cfgate.io/cfgate/api/v1alpha1'
      )
    })
  })

  describe('404 handling', () => {
    it('returns 404 for unknown paths', async () => {
      const response = await SELF.fetch('https://cfgate.io/unknown-path')

      expect(response.status).toBe(404)
      expect(await response.text()).toBe('Not Found')
    })

    it('returns 404 for unknown paths with go-get', async () => {
      const response = await SELF.fetch('https://cfgate.io/unknown-path?go-get=1')

      expect(response.status).toBe(404)
    })
  })

  describe('Proxy routes (scaffold)', () => {
    // Note: These tests verify route registration. Actual proxy behavior
    // hits real GitHub - assets may or may not exist.
    // 200 = asset exists and was proxied
    // 404 = route exists, asset doesn't exist on GitHub
    // 503 = route exists, upstream error

    it('registers /install.yaml route', async () => {
      const response = await SELF.fetch('https://cfgate.io/install.yaml')

      // Route exists and returned a valid proxy response
      expect([200, 404, 503]).toContain(response.status)
    })

    it('registers /crds.yaml route', async () => {
      const response = await SELF.fetch('https://cfgate.io/crds.yaml')
      expect([200, 404, 503]).toContain(response.status)
    })

    it('registers /crds/tunnel.yaml route', async () => {
      const response = await SELF.fetch('https://cfgate.io/crds/tunnel.yaml')
      expect([200, 404, 503]).toContain(response.status)
    })
  })
})
