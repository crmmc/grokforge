#pragma once

#include <stddef.h>
#include <stdint.h>

typedef struct gf_curl_result {
  long status_code;
  char *error;
  int impersonate_missing;
} gf_curl_result;

int gf_curl_perform(
    uintptr_t handle,
    const char *method,
    const char *url,
    const char *profile,
    const char *proxy,
    long timeout_ms,
    int skip_proxy_ssl_verify,
    const char **headers,
    int header_count,
    const void *body,
    size_t body_len,
    gf_curl_result *result);
