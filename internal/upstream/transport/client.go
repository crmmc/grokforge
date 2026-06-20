package transport

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HeaderOrderKey stores the desired wire header order in http.Header. It is a
// transport-only pseudo header and is stripped before libcurl sends requests.
const HeaderOrderKey = "X-Grokforge-Header-Order"

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Options struct {
	RequestTimeout     time.Duration
	Browser            string
	ProxyURL           string
	SkipProxySSLVerify bool
}

// Options fields must remain comparable; DynamicStatelessDoer uses them to
// decide when to rebuild the underlying client.

func NewStatelessDoer(opts Options) (Doer, error) {
	return newCurlClient(opts)
}

type DynamicStatelessDoer struct {
	mu      sync.Mutex
	current Options
	client  Doer
	provide func() Options
}

func NewDynamicStatelessDoer(provide func() Options) *DynamicStatelessDoer {
	return &DynamicStatelessDoer{provide: provide}
}

func (d *DynamicStatelessDoer) Do(req *http.Request) (*http.Response, error) {
	if d.provide == nil {
		return nil, fmt.Errorf("upstream transport: nil options provider")
	}
	opts := d.provide()
	client, err := d.clientFor(opts)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func (d *DynamicStatelessDoer) clientFor(opts Options) (Doer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client != nil && d.current == opts {
		return d.client, nil
	}
	client, err := NewStatelessDoer(opts)
	if err != nil {
		return nil, err
	}
	d.current = opts
	d.client = client
	return client, nil
}
