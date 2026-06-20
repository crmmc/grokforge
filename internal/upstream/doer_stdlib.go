package upstream

import (
	"net/http"

	"github.com/crmmc/grokforge/internal/upstream/transport"
)

// StdlibDoer executes standard library HTTP requests. It is only used in tests.
type StdlibDoer struct{ Client *http.Client }

func (d *StdlibDoer) Do(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Body = req.Body
	clone.Header = make(http.Header, len(req.Header))
	for k, vs := range req.Header {
		if k == transport.HeaderOrderKey {
			continue
		}
		clone.Header[k] = append([]string(nil), vs...)
	}
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(clone)
}
