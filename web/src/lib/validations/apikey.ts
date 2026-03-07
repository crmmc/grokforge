import { z } from 'zod'

export const apiKeyStatusSchema = z.enum(['active', 'inactive', 'expired', 'rate_limited'])

export const apiKeyCreateSchema = z.object({
  name: z.string().min(1, 'Name is required').max(100, 'Name too long'),
  model_whitelist: z.array(z.string()).optional(),
  rate_limit: z.number().int().min(0).optional(),
  daily_limit: z.number().int().min(0).optional(),
  expires_at: z.string().datetime().optional().or(z.literal('')),
})

export type APIKeyCreateInput = z.infer<typeof apiKeyCreateSchema>
