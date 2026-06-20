//go:build !cgo

package transport

import (
	"fmt"
	"net/http"
)

func curlPerform(req *http.Request, opts Options, headers []string, body []byte) (*http.Response, error) {
	return nil, fmt.Errorf("curl-impersonate transport requires cgo")
}
