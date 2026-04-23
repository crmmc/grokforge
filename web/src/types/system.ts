// System and config types

export interface SystemStatus {
  status: "healthy" | "degraded" | "unhealthy";
  version: string;
  uptime: number;
  tokens: {
    total: number;
    active: number;
  };
  api_keys: {
    total: number;
    active: number;
  };
}

// Full config types matching Go backend
export interface AppConfigResponse {
  app_key: string;
  media_generation_enabled: boolean;
  temporary: boolean;
  disable_memory: boolean;
  stream: boolean;
  thinking: boolean;
  dynamic_statsig: boolean;
  custom_instruction: string;
  filter_tags: string[];
  host: string;
  port: number;
  log_json: boolean;
  log_level: string;
  db_driver: string;
  db_path: string;
  db_dsn: string;
  request_timeout: number;
  read_header_timeout: number;
  max_header_bytes: number;
  body_limit: number;
  chat_body_limit: number;
  admin_max_fails: number;
  admin_window_sec: number;
}

export interface ProxyConfigResponse {
  base_proxy_url: string;
  asset_proxy_url: string;
  cf_cookies: string;
  skip_proxy_ssl_verify: boolean;
  enabled: boolean;
  flaresolverr_url: string;
  refresh_interval: number;
  timeout: number;
  cf_clearance: string;
  browser: string;
  user_agent: string;
}

export interface RetryConfigResponse {
  max_tokens: number;
  per_token_retries: number;
  reset_session_status_codes: number[];
  retry_backoff_base: number;
  retry_backoff_factor: number;
  retry_backoff_max: number;
  retry_budget: number;
}

export interface TokenConfigResponse {
  fail_threshold: number;
  usage_flush_interval_sec: number;
  selection_algorithm: string;
}

export interface ImageConfigResponse {
  nsfw: boolean;
  blocked_parallel_attempts: number;
  blocked_parallel_enabled: boolean;
}

export interface ConfigResponse {
  app: AppConfigResponse;
  image: ImageConfigResponse;
  proxy: ProxyConfigResponse;
  retry: RetryConfigResponse;
  token: TokenConfigResponse;
}

export interface UsageStats {
  period: "hour" | "day" | "week" | "month";
  requests: number;
  tokens_input: number;
  tokens_output: number;
  cache_tokens: number;
  errors: number;
  by_model: Record<
    string,
    {
      requests: number;
      tokens_input: number;
      tokens_output: number;
    }
  >;
  by_api_key: Array<{
    api_key_name: string;
    requests: number;
    tokens_input: number;
    tokens_output: number;
  }>;
}
