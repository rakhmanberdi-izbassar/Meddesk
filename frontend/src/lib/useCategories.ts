import { useQuery } from '@tanstack/react-query'
import { useAuth } from './auth'
import { queryString } from './api'
import { categoriesPageSchema, knowledgePath, type Category } from './knowledge'

export function useCategories() {
  const { api } = useAuth()
  return useQuery({ queryKey: ['knowledge', 'active-categories'], queryFn: async ({ signal }) => {
    const items: Category[] = []
    for (let offset = 0; ;) {
      const result = await api(`${knowledgePath}/categories?${queryString({ is_active: 'true', limit: 100, offset })}`, categoriesPageSchema, { signal })
      items.push(...result.items)
      offset += result.items.length
      if (offset >= result.total || result.items.length === 0) return items
    }
  } })
}
