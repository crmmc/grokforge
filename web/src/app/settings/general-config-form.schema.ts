import { z } from 'zod'
import { appConfigSchema, cacheConfigSchema, proxyConfigSchema, retryConfigSchema } from '@/lib/validations/config'

const imageBlockedSchema = z.object({
  format: z.enum(['base64', 'local_url']),
  blocked_parallel_enabled: z.boolean(),
  blocked_parallel_attempts: z.number().int().min(1),
})

export const generalSchema = z.object({
  app: appConfigSchema,
  proxy: proxyConfigSchema,
  retry: retryConfigSchema,
  image: imageBlockedSchema,
  cache: cacheConfigSchema,
})

export type GeneralInput = z.infer<typeof generalSchema>
