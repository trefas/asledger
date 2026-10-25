import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

function read(path: URL): string {
  return readFileSync(path, 'utf8')
}

describe('брендинг desktop', () => {
  it('использует общий логотип вместо буквенной заглушки', () => {
    const html = read(new URL('../src/renderer/index.html', import.meta.url))
    const css = read(
      new URL('../src/renderer/src/style.css', import.meta.url),
    )

    expect(html).toContain('id="brand-logo"')
    expect(html).toContain('src="./asledger-logo.svg"')
    expect(html).not.toContain('brand-mark')
    expect(css).toContain('.brand-logo')
    expect(css).not.toContain('.brand-mark')
  })

  it('берёт логотип из единого каталога assets/logo', () => {
    const logo = read(
      new URL('../../assets/logo/asledger-logo.svg', import.meta.url),
    )

    expect(logo).toContain('<svg')
    expect(logo).toContain('asledger-logo.svg')
  })
})
