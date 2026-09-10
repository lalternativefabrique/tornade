export interface RegisteredApp {
  name: string
  active: boolean
  last4: string
  created_at: string
  rotated_at?: string
  revoked_at?: string
  grace_until?: string
}

export interface AppCredentials {
  name: string
  key: string
  grace_until?: string
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1/admin/apps${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers as Record<string, string>),
    },
  })
  if (res.status === 204) return undefined as T
  const body = (await res.json().catch(() => ({}))) as T & { error?: string }
  if (!res.ok) throw new Error(body.error ?? `tornade answered ${res.status}`)
  return body
}

export const appsApi = {
  list: () => call<{ apps: RegisteredApp[] }>(''),
  register: (name: string) =>
    call<AppCredentials>('', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  rotate: (name: string) =>
    call<AppCredentials>(`/${name}/rotate`, { method: 'POST' }),
  revoke: (name: string) => call<void>(`/${name}`, { method: 'DELETE' }),
}
