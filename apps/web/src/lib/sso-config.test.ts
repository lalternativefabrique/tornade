import { describe, expect, it } from 'vitest'
import { ssoFromEnv } from './sso-config'

describe('ssoFromEnv', () => {
  it('is off without a client secret, so the password form stays', () => {
    expect(
      ssoFromEnv({ URBANGATE_ISSUER_URL: 'https://id.example' }),
    ).toBeUndefined()
  })

  it('defaults to the suite issuer and the tornade-admin client', () => {
    expect(ssoFromEnv({ URBANGATE_CLIENT_SECRET: 's' })).toEqual({
      issuer: 'https://id.urbangate.dev',
      clientId: 'tornade-admin',
      clientSecret: 's',
      adminRole: 'tornade:admin',
      providerId: 'urbangate',
    })
  })

  it('takes the issuer and client it is given', () => {
    const c = ssoFromEnv({
      URBANGATE_ISSUER_URL: 'http://localhost:4444',
      URBANGATE_CLIENT_ID: 'tornade-dev',
      URBANGATE_CLIENT_SECRET: 's',
    })
    expect(c?.issuer).toBe('http://localhost:4444')
    expect(c?.clientId).toBe('tornade-dev')
  })
})
