package upstream

import (
	"net/http"

	fhttp "github.com/bogdanfinn/fhttp"
)

// StdlibDoer 把 *fhttp.Request 转为标准 *http.Request 执行。仅测试使用。
type StdlibDoer struct{ Client *http.Client }

func (d *StdlibDoer) Do(req *fhttp.Request) (*fhttp.Response, error) {
	stdReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	for k, vs := range req.Header {
		if k == fhttp.HeaderOrderKey || k == fhttp.PHeaderOrderKey {
			continue // tls-client 专用排序键，标准库忽略
		}
		for _, v := range vs {
			stdReq.Header.Add(k, v)
		}
	}
	stdResp, err := d.Client.Do(stdReq)
	if err != nil {
		return nil, err
	}
	resp := &fhttp.Response{
		StatusCode: stdResp.StatusCode,
		Header:     fhttp.Header(stdResp.Header),
		Body:       stdResp.Body,
		Request:    req,
	}
	return resp, nil
}
