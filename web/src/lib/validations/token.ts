import { z } from 'zod'

export const tokenStatusSchema = z.enum(['active', 'disabled', 'expired'])

const quotasMapSchema = z.record(z.string(), z.number().int().min(0)).optional()

export const tokenUpdateSchema = z.object({
  status: tokenStatusSchema.optional(),
  pool: z.string().optional(),
  quotas: quotasMapSchema,
  remark: z.string().max(500, 'Remark must be 500 characters or less').optional(),
  nsfw_enabled: z.boolean().optional(),
})

export type TokenUpdateInput = z.infer<typeof tokenUpdateSchema>
