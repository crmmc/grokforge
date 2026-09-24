// Package curlstub provides a test-only stub for the curl_easy_impersonate
// symbol.
//
// The transport package resolves curl_easy_impersonate at runtime with
// dlsym(RTLD_DEFAULT, ...). Machines whose linked libcurl is stock curl do
// not export that symbol, which makes every request fail before it is sent
// and leaves the Go-side request/response path untestable. Importing this
// package from tests links a no-op curl_easy_impersonate into the test
// binary so dlsym succeeds and real requests can flow end-to-end against
// httptest servers. On machines whose libcurl already provides the real
// symbol, the definition in the main executable simply shadows it.
//
// MUST NOT be imported by production code: the stub would shadow a real
// curl-impersonate implementation and silently disable browser
// impersonation in the shipped binary.
package curlstub

/*
int curl_easy_impersonate(void *curl, const char *target, int default_headers) {
	(void)curl;
	(void)target;
	(void)default_headers;
	return 0;
}

void *gf_stub_impersonate_anchor = (void *)&curl_easy_impersonate;
*/
import "C"

import "unsafe"

// Anchor references the stub symbol table entry so linkers keep the
// otherwise unreferenced curl_easy_impersonate definition alive.
var Anchor = unsafe.Pointer(C.gf_stub_impersonate_anchor)
