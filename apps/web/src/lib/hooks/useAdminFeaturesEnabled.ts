import { useQuery } from '@tanstack/react-query'
import { hasAdminFeatures } from '@lalternative/admin'
import { getProfile } from '../services/auth'

/**
 * React hook variant of `@lalternative/admin`'s `hasAdminFeatures`. Reads the
 * shared `['me']` query so it never adds a request. Re-exports the pure rule
 * for use in route `beforeLoad` guards (which run outside React).
 */
export { hasAdminFeatures } from '@lalternative/admin'

export function useAdminFeaturesEnabled(): boolean {
  const { data } = useQuery({
    queryKey: ['me'],
    queryFn: getProfile,
    staleTime: Infinity,
  })
  return hasAdminFeatures(data)
}
