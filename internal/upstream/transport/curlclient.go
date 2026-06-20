package transport

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type curlClient struct{ opts Options }

func newCurlClient(opts Options) (*curlClient, error) {
	if resolved := ResolveProfile(opts.Browser); resolved != "" {
		opts.Browser = resolved
	} else {
		opts.Browser = DefaultProfile
	}
	return &curlClient{opts: opts}, nil
}

func (c *curlClient) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("upstream transport: nil request")
	}
	body, err := readRequestBody(req.Body)
	if err != nil {
		return nil, err
	}
	return curlPerform(req, c.opts, orderedHeaders(req.Header), body)
}

func readRequestBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	return io.ReadAll(body)
}

func orderedHeaders(h http.Header) []string {
	if h == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(h))
	for _, key := range h[HeaderOrderKey] {
		canonical := http.CanonicalHeaderKey(key)
		if values, ok := h[canonical]; ok {
			appendHeaderValues(&out, canonical, values)
			seen[strings.ToLower(canonical)] = true
			continue
		}
		if values, ok := h[key]; ok {
			appendHeaderValues(&out, key, values)
			seen[strings.ToLower(key)] = true
		}
	}

	keys := make([]string, 0, len(h))
	for key := range h {
		if key == HeaderOrderKey || seen[strings.ToLower(key)] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendHeaderValues(&out, key, h[key])
	}
	return out
}

func appendHeaderValues(out *[]string, key string, values []string) {
	for _, value := range values {
		*out = append(*out, key+": "+value)
	}
}

func responseFromParts(req *http.Request, statusCode int, header http.Header, body io.ReadCloser) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	status := fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
	if statusCode == 0 {
		status = "0"
	}
	return &http.Response{
		StatusCode: statusCode,
		Status:     status,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		Header:     header,
		Body:       body,
		Request:    req,
	}
}

func bufferedErrorBody(msg string) io.ReadCloser {
	return io.NopCloser(bytes.NewBufferString(msg))
}
