package transport

import (
	"fmt"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type Options struct {
	RequestTimeout     time.Duration
	Browser            string
	ProxyURL           string
	SkipProxySSLVerify bool
}

// Options fields must remain comparable; DynamicStatelessDoer uses them to
// decide when to rebuild the underlying client.

func NewStatelessDoer(opts Options) (tls_client.HttpClient, error) {
	tlsOpts := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(resolveBrowserProfile(opts.Browser)),
		tls_client.WithNotFollowRedirects(),
	}
	if opts.RequestTimeout > 0 {
		tlsOpts = append(tlsOpts, tls_client.WithTimeoutSeconds(int(opts.RequestTimeout.Seconds())))
	}
	// Do not call WithCookieJar. Upstream headers must be the only cookie source.
	if opts.SkipProxySSLVerify {
		tlsOpts = append(tlsOpts, tls_client.WithInsecureSkipVerify())
	}
	if opts.ProxyURL != "" {
		tlsOpts = append(tlsOpts, tls_client.WithProxyUrl(opts.ProxyURL))
	}
	return tls_client.NewHttpClient(nil, tlsOpts...)
}

type DynamicStatelessDoer struct {
	mu      sync.Mutex
	current Options
	client  tls_client.HttpClient
	provide func() Options
}

func NewDynamicStatelessDoer(provide func() Options) *DynamicStatelessDoer {
	return &DynamicStatelessDoer{provide: provide}
}

func (d *DynamicStatelessDoer) Do(req *fhttp.Request) (*fhttp.Response, error) {
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

func (d *DynamicStatelessDoer) clientFor(opts Options) (tls_client.HttpClient, error) {
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

func resolveBrowserProfile(name string) profiles.ClientProfile {
	if name == "" {
		return profiles.Chrome_146
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if p, ok := profiles.MappedTLSClients[key]; ok {
		return p
	}
	if p, ok := profiles.MappedTLSClients[insertUnderscore(key)]; ok {
		return p
	}
	return profiles.Chrome_146
}

// insertUnderscore inserts an underscore between trailing letters and leading digits.
func insertUnderscore(s string) string {
	for i := 1; i < len(s); i++ {
		if isLetter(s[i-1]) && isDigit(s[i]) {
			return s[:i] + "_" + s[i:]
		}
	}
	return s
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }
