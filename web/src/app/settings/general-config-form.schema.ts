import { z } from 'zod'
import { appConfigSchema, proxyConfigSchema, retryConfigSchema } from '@/lib/validations/config'

const imageBlockedSchema = z.object({
  blocked_parallel_enabled: z.boolean(),
  blocked_parallel_attempts: z.number().int().min(1),
})

export const generalSchema = z.object({
  app: appConfigSchema,
  proxy: proxyConfigSchema,
  retry: retryConfigSchema,
  image: imageBlockedSchema,
})

export type GeneralInput = z.infer<typeof generalSchema>
