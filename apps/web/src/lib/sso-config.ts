export interface SsoEnv {
  URBANGATE_ISSUER_URL?: string
  URBANGATE_CLIENT_ID?: string
  URBANGATE_CLIENT_SECRET?: string
}

export const SSO_PROVIDER_ID = 'urbangate'

export function ssoFromEnv(env: SsoEnv) {
  if (!env.URBANGATE_CLIENT_SECRET) return undefined
  return {
    issuer: env.URBANGATE_ISSUER_URL ?? 'https://id.urbangate.dev',
    clientId: env.URBANGATE_CLIENT_ID ?? 'tornade-admin',
    clientSecret: env.URBANGATE_CLIENT_SECRET,
    adminRole: 'tornade:admin',
    providerId: SSO_PROVIDER_ID,
  }
}
