//go:build cgo

package transport

/*
#cgo darwin CFLAGS: -I/opt/homebrew/opt/curl/include -I/usr/local/opt/curl/include
#cgo darwin LDFLAGS: -L/opt/homebrew/opt/curl/lib -L/usr/local/opt/curl/lib -lcurl
#cgo linux LDFLAGS: -lcurl -ldl
#include "curlclient_cgo.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	curlHandleSeq atomic.Uint64
	curlHandles   sync.Map
)

type curlState struct {
	ctx        context.Context
	bodyWriter *io.PipeWriter

	mu          sync.Mutex
	header      http.Header
	statusCode  int
	headerErr   error
	headerReady chan struct{}
	readyOnce   sync.Once
}

func curlPerform(req *http.Request, opts Options, headers []string, body []byte) (*http.Response, error) {
	reader, writer := io.Pipe()
	state := &curlState{
		ctx:         req.Context(),
		bodyWriter:  writer,
		header:      make(http.Header),
		headerReady: make(chan struct{}),
	}

	handle := uintptr(curlHandleSeq.Add(1))
	curlHandles.Store(handle, state)

	errCh := make(chan error, 1)
	go func() {
		defer curlHandles.Delete(handle)
		err := performCurlC(handle, req, opts, headers, body)
		if err != nil {
			state.signalHeaders(err)
			_ = writer.CloseWithError(err)
		} else {
			state.signalHeaders(nil)
			_ = writer.Close()
		}
		errCh <- err
	}()

	select {
	case <-state.headerReady:
		state.mu.Lock()
		statusCode := state.statusCode
		header := state.header.Clone()
		headerErr := state.headerErr
		state.mu.Unlock()
		if headerErr != nil {
			_ = reader.CloseWithError(headerErr)
			return nil, headerErr
		}
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		return responseFromParts(req, statusCode, header, reader), nil
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
		state.mu.Lock()
		statusCode := state.statusCode
		header := state.header.Clone()
		state.mu.Unlock()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		return responseFromParts(req, statusCode, header, reader), nil
	case <-req.Context().Done():
		err := req.Context().Err()
		state.signalHeaders(err)
		_ = reader.CloseWithError(err)
		return nil, err
	}
}

func performCurlC(handle uintptr, req *http.Request, opts Options, headers []string, body []byte) error {
	method := C.CString(req.Method)
	defer C.free(unsafe.Pointer(method))
	url := C.CString(req.URL.String())
	defer C.free(unsafe.Pointer(url))
	profile := C.CString(EffectiveProfile(opts.Browser, req.Header.Get("User-Agent")))
	defer C.free(unsafe.Pointer(profile))
	proxy := C.CString(opts.ProxyURL)
	defer C.free(unsafe.Pointer(proxy))

	cHeaders, cleanupHeaders := cStringArray(headers)
	defer cleanupHeaders()

	var bodyPtr unsafe.Pointer
	if len(body) > 0 {
		bodyPtr = C.CBytes(body)
		defer C.free(bodyPtr)
	}

	var result C.gf_curl_result
	rc := C.gf_curl_perform(
		C.uintptr_t(handle),
		method,
		url,
		profile,
		proxy,
		C.long(opts.RequestTimeout.Milliseconds()),
		boolToCInt(opts.SkipProxySSLVerify),
		cHeaders,
		C.int(len(headers)),
		bodyPtr,
		C.size_t(len(body)),
		&result,
	)
	defer func() {
		if result.error != nil {
			C.free(unsafe.Pointer(result.error))
		}
	}()

	if result.status_code > 0 {
		if state, ok := loadCurlState(handle); ok {
			state.setStatus(int(result.status_code))
		}
	}
	if rc != 0 {
		msg := "curl perform failed"
		if result.error != nil {
			msg = C.GoString(result.error)
		}
		if result.impersonate_missing != 0 {
			return fmt.Errorf("curl-impersonate unavailable: %s", msg)
		}
		return fmt.Errorf("curl perform: %s", msg)
	}
	return nil
}

func cStringArray(values []string) (**C.char, func()) {
	if len(values) == 0 {
		return nil, func() {}
	}
	size := C.size_t(len(values)) * C.size_t(unsafe.Sizeof(uintptr(0)))
	array := C.malloc(size)
	if array == nil {
		return nil, func() {}
	}
	ptrs := (*[1 << 28]*C.char)(array)[:len(values):len(values)]
	for i, value := range values {
		ptrs[i] = C.CString(value)
	}
	return (**C.char)(array), func() {
		for _, ptr := range ptrs {
			C.free(unsafe.Pointer(ptr))
		}
		C.free(array)
	}
}

func boolToCInt(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

func loadCurlState(handle uintptr) (*curlState, bool) {
	value, ok := curlHandles.Load(handle)
	if !ok {
		return nil, false
	}
	state, ok := value.(*curlState)
	return state, ok
}

func (s *curlState) setStatus(status int) {
	s.mu.Lock()
	s.statusCode = status
	s.mu.Unlock()
}

func (s *curlState) signalHeaders(err error) {
	s.readyOnce.Do(func() {
		s.mu.Lock()
		s.headerErr = err
		s.mu.Unlock()
		close(s.headerReady)
	})
}

func (s *curlState) writeBody(p []byte) int {
	select {
	case <-s.ctx.Done():
		return 0
	default:
	}
	n, err := s.bodyWriter.Write(p)
	if err != nil {
		return 0
	}
	return n
}

func (s *curlState) writeHeaderLine(line string) int {
	select {
	case <-s.ctx.Done():
		return 0
	default:
	}
	trimmed := strings.TrimRight(line, "\r\n")
	if trimmed == "" {
		s.signalHeaders(nil)
		return len(line)
	}
	if strings.HasPrefix(strings.ToUpper(trimmed), "HTTP/") {
		parts := strings.Fields(trimmed)
		if len(parts) >= 2 {
			if status, err := strconv.Atoi(parts[1]); err == nil {
				s.mu.Lock()
				s.statusCode = status
				s.header = make(http.Header)
				s.mu.Unlock()
			}
		}
		return len(line)
	}
	key, value, ok := strings.Cut(trimmed, ":")
	if !ok {
		return len(line)
	}
	s.mu.Lock()
	s.header.Add(http.CanonicalHeaderKey(strings.TrimSpace(key)), strings.TrimSpace(value))
	s.mu.Unlock()
	return len(line)
}

//export gfGoCurlWrite
func gfGoCurlWrite(handle C.uintptr_t, ptr *C.char, size C.size_t) C.size_t {
	state, ok := loadCurlState(uintptr(handle))
	if !ok {
		return 0
	}
	data := C.GoBytes(unsafe.Pointer(ptr), C.int(size))
	written := state.writeBody(data)
	return C.size_t(written)
}

//export gfGoCurlHeader
func gfGoCurlHeader(handle C.uintptr_t, ptr *C.char, size C.size_t) C.size_t {
	state, ok := loadCurlState(uintptr(handle))
	if !ok {
		return 0
	}
	line := C.GoStringN(ptr, C.int(size))
	written := state.writeHeaderLine(line)
	return C.size_t(written)
}
