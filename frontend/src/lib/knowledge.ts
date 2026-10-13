import { z } from 'zod'

export const categorySchema = z.object({ id: z.string(), slug: z.string(), name_kk: z.string(), name_ru: z.string(), description: z.string(), is_active: z.boolean(), created_at: z.string(), updated_at: z.string() })
export type Category = z.infer<typeof categorySchema>
export const categoryResultSchema = z.object({ category: categorySchema })
export const categoriesPageSchema = z.object({ items: z.array(categorySchema), total: z.number(), limit: z.number(), offset: z.number() })
export const articleSchema = z.object({
  id: z.string(), version: z.number().int(), title: z.string(), slug: z.string(), description: z.string(),
  category_id: z.string(), category_name_kk: z.string(), category_name_ru: z.string(), category_active: z.boolean(), symptoms: z.string(),
  solution_steps: z.array(z.string()), tags: z.array(z.string()), author_id: z.string(), author_name: z.string(),
  video_id: z.string().nullable(), image_ids: z.array(z.string()), is_published: z.boolean(), published_at: z.string().nullable(),
  created_at: z.string(), updated_at: z.string(), view_count: z.number(), helpful_count: z.number(), not_helpful_count: z.number(),
})
export type Article = z.infer<typeof articleSchema>
export const articleResultSchema = z.object({ article: articleSchema })
export const articlesPageSchema = z.object({ items: z.array(articleSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const viewsSchema = z.object({ recorded: z.boolean(), view_count: z.number() })
export const feedbackSchema = z.object({ recorded: z.boolean(), helpful_count: z.number(), not_helpful_count: z.number() })
export const mediaSchema = z.object({ id: z.string(), original_filename: z.string(), content_type: z.string(), byte_count: z.number(), purpose: z.enum(['ARTICLE_IMAGE', 'ARTICLE_VIDEO', 'TICKET_ATTACHMENT']), created_at: z.string() })
export const mediaResultSchema = z.object({ media: mediaSchema })
export type MediaPurpose = z.infer<typeof mediaSchema>['purpose']
export const knowledgePath = '/knowledge'
export const mediaURL = (id: string) => `/api/v1/media/${encodeURIComponent(id)}`
export const imageMaxBytes = 10 * 1024 * 1024
export const videoMaxBytes = 100 * 1024 * 1024
export function validMediaFile(file: File, purpose: MediaPurpose) {
  const types = purpose !== 'ARTICLE_VIDEO' ? ['image/png', 'image/jpeg'] : ['video/mp4', 'video/webm']
  return file.size > 0 && file.size <= (purpose !== 'ARTICLE_VIDEO' ? imageMaxBytes : videoMaxBytes) && types.includes(file.type)
}
export function hasStaffRole(role: string | undefined) { return role === 'ADMIN' || role === 'IT_ENGINEER' }
