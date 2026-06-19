package console

import (
	"context"

	"github.com/crmmc/grokforge/internal/upstream"
)

const DefaultURL = "https://console.x.ai/v1/responses"

type Options struct {
	BuildCookieString func(token string) string
	HeaderOrder       func() []string
	BrowserProfile    func() string
	UserAgent         func() string
	StatsigID         func() string
}

type ConsoleUpstream struct {
	baseURL string
	doer    upstream.Doer
	opts    Options
}

func New(baseURL string, doer upstream.Doer, opts Options) *ConsoleUpstream {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &ConsoleUpstream{baseURL: baseURL, doer: doer, opts: opts}
}

func (c *ConsoleUpstream) Name() string { return "console" }

func (c *ConsoleUpstream) Chat(ctx context.Context, token string, req *upstream.ChatRequest) (<-chan upstream.StreamEvent, error) {
	body, err := c.buildBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.buildRequest(ctx, token, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.doer.Do(httpReq)
	if err != nil {
		return nil, c.mapError(err)
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		return nil, c.mapHTTPError(resp)
	}

	ch := make(chan upstream.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		if err := c.parseStream(ctx, resp.Body, ch); err != nil {
			ch <- upstream.StreamEvent{Error: err}
		}
	}()
	return ch, nil
}

var _ upstream.Upstream = (*ConsoleUpstream)(nil)

func (c *ConsoleUpstream) cookieString(token string) string {
	if c.opts.BuildCookieString == nil {
		return ""
	}
	return c.opts.BuildCookieString(token)
}

func (c *ConsoleUpstream) headerOrder() []string {
	if c.opts.HeaderOrder == nil {
		return nil
	}
	return c.opts.HeaderOrder()
}

func (c *ConsoleUpstream) browserProfile() string {
	if c.opts.BrowserProfile == nil {
		return ""
	}
	return c.opts.BrowserProfile()
}

func (c *ConsoleUpstream) userAgent() string {
	if c.opts.UserAgent == nil {
		return ""
	}
	return c.opts.UserAgent()
}

func (c *ConsoleUpstream) statsigID() string {
	if c.opts.StatsigID == nil {
		return ""
	}
	return c.opts.StatsigID()
}
