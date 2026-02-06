import { describe, it, expect } from 'vitest'
import { generateVanityHTML } from '../../../src/handlers/vanity.js'

describe('generateVanityHTML', () => {
  it('generates correct HTML for /cfgate path', () => {
    const html = generateVanityHTML('/cfgate')

    expect(html).toContain('<!DOCTYPE html>')
    expect(html).toContain(
      '<meta name="go-import" content="cfgate.io/cfgate git https://github.com/inherent-design/cfgate">'
    )
    expect(html).toContain('<meta name="go-source" content="cfgate.io/cfgate')
  })

  it('generates correct HTML for subpath', () => {
    const html = generateVanityHTML('/cfgate/api/v1alpha1')

    // go-import should always point to module root
    expect(html).toContain(
      '<meta name="go-import" content="cfgate.io/cfgate git https://github.com/inherent-design/cfgate">'
    )
  })

  it('includes go-source meta tag with correct templates', () => {
    const html = generateVanityHTML('/cfgate')

    // Check go-source components
    expect(html).toContain('cfgate.io/cfgate')
    expect(html).toContain('https://github.com/inherent-design/cfgate')
    expect(html).toContain('/tree/main{/dir}')
    expect(html).toContain('/blob/main{/dir}/{file}#L{line}')
  })

  it('returns same go-import for all subpaths', () => {
    const paths = ['/cfgate', '/cfgate/api', '/cfgate/api/v1alpha1', '/cfgate/internal/controller']

    const expectedImport =
      '<meta name="go-import" content="cfgate.io/cfgate git https://github.com/inherent-design/cfgate">'

    for (const path of paths) {
      const html = generateVanityHTML(path)
      expect(html).toContain(expectedImport)
    }
  })
})
