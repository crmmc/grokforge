package transport

import "net/http"

type FakeDoer struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (f FakeDoer) Do(req *http.Request) (*http.Response, error) {
	return f.DoFunc(req)
}
