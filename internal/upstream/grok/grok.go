package grok

import (
	"context"

	"github.com/crmmc/grokforge/internal/upstream"
)

const (
	DefaultURL       = "https://grok.com/rest/app-chat/conversations/new"
	DefaultUploadURL = "https://grok.com/rest/app-chat/upload-file"
)

// Options is supplied by wiring so grok does not depend on config/runtime packages.
type Options struct {
	BuildCookieString func(token string) string
	HeaderOrder       func() []string
	BrowserProfile    func() string
	UserAgent         func() string
	StatsigID         func() string
	UploadURL         func() string
}

type GrokUpstream struct {
	baseURL string
	doer    upstream.Doer
	opts    Options
}

func New(baseURL string, doer upstream.Doer, opts Options) *GrokUpstream {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &GrokUpstream{baseURL: baseURL, doer: doer, opts: opts}
}

func (g *GrokUpstream) Name() string { return "grok" }

func (g *GrokUpstream) Chat(ctx context.Context, token string, req *upstream.ChatRequest) (<-chan upstream.StreamEvent, error) {
	messages, fileAttachments, err := g.processMultimodal(ctx, token, req.Messages)
	if err != nil {
		return nil, err
	}

	reqCopy := *req
	reqCopy.Messages = messages
	body, err := g.buildBody(&reqCopy, fileAttachments)
	if err != nil {
		return nil, err
	}
	httpReq, err := g.buildRequest(ctx, token, body)
	if err != nil {
		return nil, err
	}
	resp, err := g.doer.Do(httpReq)
	if err != nil {
		return nil, g.mapError(err)
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		return nil, g.mapHTTPError(resp)
	}

	ch := make(chan upstream.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		if err := g.parseStream(ctx, token, resp.Body, ch); err != nil {
			ch <- upstream.StreamEvent{Error: err}
		}
	}()
	return ch, nil
}

var _ upstream.Upstream = (*GrokUpstream)(nil)

func (g *GrokUpstream) cookieString(token string) string {
	if g.opts.BuildCookieString == nil {
		return ""
	}
	return g.opts.BuildCookieString(token)
}

func (g *GrokUpstream) headerOrder() []string {
	if g.opts.HeaderOrder == nil {
		return nil
	}
	return g.opts.HeaderOrder()
}

func (g *GrokUpstream) browserProfile() string {
	if g.opts.BrowserProfile == nil {
		return ""
	}
	return g.opts.BrowserProfile()
}

func (g *GrokUpstream) userAgent() string {
	if g.opts.UserAgent == nil {
		return ""
	}
	return g.opts.UserAgent()
}

func (g *GrokUpstream) statsigID() string {
	if g.opts.StatsigID == nil {
		return ""
	}
	return g.opts.StatsigID()
}

func (g *GrokUpstream) uploadURL() string {
	if g.opts.UploadURL != nil {
		if u := g.opts.UploadURL(); u != "" {
			return u
		}
	}
	return DefaultUploadURL
}
