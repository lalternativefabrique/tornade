import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { appsApi } from '@/lib/apps-api'
import type { AppCredentials, RegisteredApp } from '@/lib/apps-api'

export const Route = createFileRoute('/admin/apps')({ component: AppsPage })

const appsKey = ['admin', 'apps']

function AppsPage() {
  const queryClient = useQueryClient()
  const apps = useQuery({ queryKey: appsKey, queryFn: appsApi.list })
  const [name, setName] = useState('')
  const [credentials, setCredentials] = useState<AppCredentials | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refresh = () => queryClient.invalidateQueries({ queryKey: appsKey })
  const onCredentials = (c: AppCredentials) => {
    setCredentials(c)
    setError(null)
    void refresh()
  }
  const onError = (e: Error) => setError(e.message)

  const register = useMutation({
    mutationFn: appsApi.register,
    onSuccess: onCredentials,
    onError,
  })
  const rotate = useMutation({
    mutationFn: appsApi.rotate,
    onSuccess: onCredentials,
    onError,
  })
  const revoke = useMutation({
    mutationFn: appsApi.revoke,
    onSuccess: () => void refresh(),
    onError,
  })

  return (
    <div className="flex flex-col gap-8">
      <header>
        <h1 className="text-2xl font-semibold">Applications</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Chaque application a une clé : celle que son serveur présente sur ses
          propres appels, et avec laquelle il signe les URL que ses navigateurs
          écoutent. Elle n'est montrée qu'une fois.
        </p>
      </header>

      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const trimmed = name.trim()
          if (!trimmed) return
          register.mutate(trimmed, { onSuccess: () => setName('') })
        }}
      >
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Nom de l'émetteur</span>
          <input
            className="rounded-md border border-border bg-background px-3 py-2 font-mono text-sm"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="partage"
            pattern="[a-z][a-z0-9-]*"
            required
          />
        </label>
        <button
          type="submit"
          disabled={register.isPending}
          className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
        >
          {register.isPending ? 'Enregistrement…' : 'Enregistrer'}
        </button>
      </form>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {credentials && (
        <Credentials
          value={credentials}
          onDismiss={() => setCredentials(null)}
        />
      )}

      <AppsTable
        apps={apps.data?.apps ?? []}
        loading={apps.isLoading}
        onRotate={(n) => rotate.mutate(n)}
        onRevoke={(n) => {
          if (
            window.confirm(
              `Révoquer ${n} ? Ses lectures cesseront immédiatement.`,
            )
          )
            revoke.mutate(n)
        }}
      />
    </div>
  )
}

function Credentials({
  value,
  onDismiss,
}: {
  value: AppCredentials
  onDismiss: () => void
}) {
  const upper = value.name.toUpperCase().replace(/-/g, '_')
  const lines = `${upper}_TORNADE_KEY=${value.key}`
  return (
    <section className="rounded-md border border-amber-500/40 bg-amber-500/10 p-4 text-sm">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="font-semibold">Clé de {value.name}</h2>
          <p className="mt-1 text-muted-foreground">
            À coller dans la configuration de l'application. Elle ne sera plus
            affichée.
            {value.grace_until && (
              <>
                {' '}
                L'ancienne clé fonctionne jusqu'au{' '}
                {new Date(value.grace_until).toLocaleString('fr-FR')}.
              </>
            )}
          </p>
        </div>
        <button
          type="button"
          onClick={onDismiss}
          className="text-muted-foreground hover:text-foreground"
        >
          Fermer
        </button>
      </div>
      <pre className="mt-3 overflow-x-auto rounded bg-background p-3 font-mono text-xs">
        {lines}
      </pre>
      <button
        type="button"
        className="mt-2 text-xs underline"
        onClick={() => void navigator.clipboard.writeText(lines)}
      >
        Copier
      </button>
    </section>
  )
}

function AppsTable({
  apps,
  loading,
  onRotate,
  onRevoke,
}: {
  apps: RegisteredApp[]
  loading: boolean
  onRotate: (name: string) => void
  onRevoke: (name: string) => void
}) {
  if (loading)
    return <p className="text-sm text-muted-foreground">Chargement…</p>
  if (apps.length === 0)
    return (
      <p className="text-sm text-muted-foreground">
        Aucune application enregistrée.
      </p>
    )
  const date = (s?: string) => (s ? new Date(s).toLocaleString('fr-FR') : '—')
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead className="text-left text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th className="py-2 pr-4">Nom</th>
            <th className="py-2 pr-4">État</th>
            <th className="py-2 pr-4">Clé</th>
            <th className="py-2 pr-4">Créée</th>
            <th className="py-2 pr-4">Dernière rotation</th>
            <th className="py-2 pr-4">Grâce jusqu'au</th>
            <th className="py-2" />
          </tr>
        </thead>
        <tbody>
          {apps.map((a) => (
            <tr key={a.name} className="border-t border-border">
              <td className="py-2 pr-4 font-mono">{a.name}</td>
              <td className="py-2 pr-4">
                {a.active ? 'active' : `révoquée le ${date(a.revoked_at)}`}
              </td>
              <td className="py-2 pr-4 font-mono text-xs">····{a.last4}</td>
              <td className="py-2 pr-4">{date(a.created_at)}</td>
              <td className="py-2 pr-4">{date(a.rotated_at)}</td>
              <td className="py-2 pr-4">{date(a.grace_until)}</td>
              <td className="py-2 text-right">
                {a.active && (
                  <>
                    <button
                      type="button"
                      className="mr-3 underline"
                      onClick={() => onRotate(a.name)}
                    >
                      Faire tourner la clé
                    </button>
                    <button
                      type="button"
                      className="text-destructive underline"
                      onClick={() => onRevoke(a.name)}
                    >
                      Révoquer
                    </button>
                  </>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
