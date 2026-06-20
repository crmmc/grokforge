#include "curlclient_cgo.h"

#include <curl/curl.h>
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

extern size_t gfGoCurlWrite(uintptr_t handle, char *ptr, size_t size);
extern size_t gfGoCurlHeader(uintptr_t handle, char *ptr, size_t size);

typedef CURLcode (*curl_easy_impersonate_fn)(CURL *curl, const char *target, int default_headers);

static size_t gf_write_cb(char *ptr, size_t size, size_t nmemb, void *userdata) {
  size_t n = size * nmemb;
  uintptr_t handle = (uintptr_t)userdata;
  return gfGoCurlWrite(handle, ptr, n);
}

static size_t gf_header_cb(char *ptr, size_t size, size_t nmemb, void *userdata) {
  size_t n = size * nmemb;
  uintptr_t handle = (uintptr_t)userdata;
  return gfGoCurlHeader(handle, ptr, n);
}

static void gf_set_error(gf_curl_result *result, const char *msg) {
  if (result == NULL || msg == NULL) {
    return;
  }
  result->error = strdup(msg);
}

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
    gf_curl_result *result) {
  if (result != NULL) {
    result->status_code = 0;
    result->error = NULL;
    result->impersonate_missing = 0;
  }

  curl_global_init(CURL_GLOBAL_DEFAULT);
  CURL *curl = curl_easy_init();
  if (curl == NULL) {
    gf_set_error(result, "curl_easy_init failed");
    return 1;
  }

  curl_easy_impersonate_fn impersonate =
      (curl_easy_impersonate_fn)dlsym(RTLD_DEFAULT, "curl_easy_impersonate");
  if (impersonate == NULL) {
    if (result != NULL) {
      result->impersonate_missing = 1;
    }
    gf_set_error(result, "curl_easy_impersonate symbol not found");
    curl_easy_cleanup(curl);
    return 1;
  }

  CURLcode rc = impersonate(curl, profile, 0);
  if (rc != CURLE_OK) {
    gf_set_error(result, curl_easy_strerror(rc));
    curl_easy_cleanup(curl);
    return 1;
  }

  struct curl_slist *header_list = NULL;
  for (int i = 0; i < header_count; i++) {
    if (headers[i] != NULL) {
      header_list = curl_slist_append(header_list, headers[i]);
    }
  }

  curl_easy_setopt(curl, CURLOPT_URL, url);
  curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 0L);
  curl_easy_setopt(curl, CURLOPT_HTTPHEADER, header_list);
  curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, gf_write_cb);
  curl_easy_setopt(curl, CURLOPT_WRITEDATA, (void *)handle);
  curl_easy_setopt(curl, CURLOPT_HEADERFUNCTION, gf_header_cb);
  curl_easy_setopt(curl, CURLOPT_HEADERDATA, (void *)handle);
  curl_easy_setopt(curl, CURLOPT_NOSIGNAL, 1L);

  if (timeout_ms > 0) {
    curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, timeout_ms);
  }
  if (proxy != NULL && proxy[0] != '\0') {
    curl_easy_setopt(curl, CURLOPT_PROXY, proxy);
  }
  if (skip_proxy_ssl_verify) {
    curl_easy_setopt(curl, CURLOPT_PROXY_SSL_VERIFYPEER, 0L);
    curl_easy_setopt(curl, CURLOPT_PROXY_SSL_VERIFYHOST, 0L);
  }

  if (method != NULL && strcmp(method, "GET") == 0 && body_len == 0) {
    curl_easy_setopt(curl, CURLOPT_HTTPGET, 1L);
  } else if (method != NULL && strcmp(method, "POST") == 0) {
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body_len > 0 ? body : "");
    curl_easy_setopt(curl, CURLOPT_POSTFIELDSIZE, (long)body_len);
  } else {
    curl_easy_setopt(curl, CURLOPT_CUSTOMREQUEST, method);
    if (body_len > 0) {
      curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body);
      curl_easy_setopt(curl, CURLOPT_POSTFIELDSIZE, (long)body_len);
    }
  }

  rc = curl_easy_perform(curl);
  long status = 0;
  curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &status);
  if (result != NULL) {
    result->status_code = status;
  }

  if (header_list != NULL) {
    curl_slist_free_all(header_list);
  }
  curl_easy_cleanup(curl);

  if (rc != CURLE_OK) {
    gf_set_error(result, curl_easy_strerror(rc));
    return 1;
  }
  return 0;
}
