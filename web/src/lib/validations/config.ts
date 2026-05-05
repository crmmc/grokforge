import { z } from "zod";

export const appConfigSchema = z.object({
  app_key: z.string(),
  media_generation_enabled: z.boolean(),
  temporary: z.boolean(),
  disable_memory: z.boolean(),
  stream: z.boolean(),
  thinking: z.boolean(),
  dynamic_statsig: z.boolean(),
  custom_instruction: z.string(),
  filter_tags: z.array(z.string()),
  host: z.string(),
  port: z.number().int().min(1).max(65535),
  log_json: z.boolean(),
  log_level: z.enum(["debug", "info", "warn", "error"]),
  db_driver: z.enum(["sqlite", "postgres"]),
  db_path: z.string(),
  db_dsn: z.string(),
  request_timeout: z.number().int().min(1),
  read_header_timeout: z.number().int().min(1),
  max_header_bytes: z.number().int().min(4096),
  body_limit: z.number().int().min(4096),
  chat_body_limit: z.number().int().min(4096),
  admin_max_fails: z.number().int().min(1),
  admin_window_sec: z.number().int().min(10),
  global_rate_limit_rpm: z.number().int().min(0),
  global_rate_limit_window: z.number().int().min(1),
});

export const proxyConfigSchema = z.object({
  base_proxy_url: z.string(),
  asset_proxy_url: z.string(),
  cf_cookies: z.string(),
  skip_proxy_ssl_verify: z.boolean(),
  enabled: z.boolean(),
  flaresolverr_url: z.string(),
  refresh_interval: z.number().int().min(0),
  timeout: z.number().int().min(0),
  cf_clearance: z.string(),
  browser: z.string(),
  user_agent: z.string(),
});

export const retryConfigSchema = z.object({
  max_tokens: z.number().int().min(1).max(20),
  per_token_retries: z.number().int().min(1).max(10),
  reset_session_status_codes: z.array(z.number().int().min(100).max(599)),
  retry_backoff_base: z.number().min(0),
  retry_backoff_factor: z.number().min(1),
  retry_backoff_max: z.number().min(0),
  retry_budget: z.number().min(0),
});

export const tokenConfigSchema = z.object({
  fail_threshold: z.number().int().min(1),
  usage_flush_interval_sec: z.number().int().min(1),
  selection_algorithm: z.enum(["high_quota_first", "random", "round_robin"]),
  recent_use_penalty_sec: z.number().int().min(0),
});

export const imageConfigSchema = z.object({
  nsfw: z.boolean(),
  format: z.enum(["base64", "local_url"]),
  blocked_parallel_attempts: z.number().int().min(1),
  blocked_parallel_enabled: z.boolean(),
});

export const cacheConfigSchema = z.object({
  image_max_mb: z.number().int().min(0),
  video_max_mb: z.number().int().min(0),
});

export const configSchema = z.object({
  app: appConfigSchema,
  image: imageConfigSchema,
  proxy: proxyConfigSchema,
  retry: retryConfigSchema,
  token: tokenConfigSchema,
  cache: cacheConfigSchema,
});
