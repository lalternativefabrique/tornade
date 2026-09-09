import { describe, it, expect, vi, afterEach } from 'vitest'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

async function gate() {
  vi.resetModules()
  return (await import('./admin-setup-gate')).checkAdminSetupGate
}

describe('checkAdminSetupGate', () => {
  it('allows an unconfigured gate outside production', async () => {
    vi.stubEnv('NODE_ENV', 'development')
    expect((await gate())('anyone@example.com', undefined)).toBeNull()
  })

  it('refuses an unconfigured gate in production', async () => {
    vi.stubEnv('NODE_ENV', 'production')
    expect((await gate())('anyone@example.com', undefined)).toBe('Admin setup is not configured')
  })

  it('rejects a missing or wrong token', async () => {
    vi.stubEnv('ADMIN_SETUP_TOKEN', 'secret')
    const check = await gate()
    expect(check('anyone@example.com', undefined)).toBe('Invalid or missing setup token')
    expect(check('anyone@example.com', 'wrong')).toBe('Invalid or missing setup token')
    expect(check('anyone@example.com', 'secre')).toBe('Invalid or missing setup token')
  })

  it('accepts the configured token', async () => {
    vi.stubEnv('ADMIN_SETUP_TOKEN', 'secret')
    expect((await gate())('anyone@example.com', 'secret')).toBeNull()
  })

  it('restricts the email when an allowlist is configured', async () => {
    vi.stubEnv('ADMIN_ALLOWED_EMAILS', ' Ops@Example.com , other@example.com ')
    const check = await gate()
    expect(check('  ops@example.com  ', undefined)).toBeNull()
    expect(check('OPS@EXAMPLE.COM', undefined)).toBeNull()
    expect(check('stranger@example.com', undefined)).toBe(
      'This email is not authorized to create the admin account',
    )
  })

  it('applies both checks when both are configured', async () => {
    vi.stubEnv('ADMIN_SETUP_TOKEN', 'secret')
    vi.stubEnv('ADMIN_ALLOWED_EMAILS', 'ops@example.com')
    const check = await gate()
    expect(check('ops@example.com', 'secret')).toBeNull()
    expect(check('ops@example.com', 'wrong')).toBe('Invalid or missing setup token')
    expect(check('other@example.com', 'secret')).toBe(
      'This email is not authorized to create the admin account',
    )
  })
})
