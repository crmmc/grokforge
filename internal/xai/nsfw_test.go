package xai

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

// mockHTTPClient records requests and returns canned responses.
type mockHTTPClient struct {
	calls    []mockCall
	response *http.Response
	err      error
}

type mockCall struct {
	URL         string
	Method      string
	ContentType string
	Origin      string
	Referer     string
	Cookie      string
	Body        []byte
	HasGRPCWeb  bool
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	m.calls = append(m.calls, mockCall{
		URL:         req.URL.String(),
		Method:      req.Method,
		ContentType: req.Header.Get("Content-Type"),
		Origin:      req.Header.Get("Origin"),
		Referer:     req.Header.Get("Referer"),
		Cookie:      req.Header.Get("Cookie"),
		Body:        body,
		HasGRPCWeb:  req.Header.Get("x-grpc-web") == "1",
	})
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func grpcOKResponse() *http.Response {
	body := successTrailer()
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{},
	}
}

func restOKResponse() *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     http.Header{},
	}
}

// --- NsfwClient request tests ---

func newTestNsfwClient(mock *mockHTTPClient) *NsfwClient {
	return &NsfwClient{
		token:     "test-sso-token",
		opts:      DefaultOptions(),
		statsigID: staticStatsigID,
		http:      mock,
	}
}

func TestAcceptTOS_RequestDetails(t *testing.T) {
	mock := &mockHTTPClient{response: grpcOKResponse()}
	c := newTestNsfwClient(mock)

	err := c.AcceptTOS(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	call := mock.calls[0]

	if call.URL != acceptTOSURL {
		t.Errorf("URL = %s, want %s", call.URL, acceptTOSURL)
	}
	if call.ContentType != "application/grpc-web+proto" {
		t.Errorf("Content-Type = %s, want application/grpc-web+proto", call.ContentType)
	}
	if call.Origin != accountsOrigin {
		t.Errorf("Origin = %s, want %s", call.Origin, accountsOrigin)
	}
	if call.Referer != accountsOrigin+"/accept-tos" {
		t.Errorf("Referer = %s, want %s/accept-tos", call.Referer, accountsOrigin)
	}
	if !call.HasGRPCWeb {
		t.Error("expected x-grpc-web: 1 header")
	}
	if !strings.Contains(call.Cookie, "sso=test-sso-token") {
		t.Errorf("Cookie should contain sso token, got %s", call.Cookie)
	}
}

func TestSetBirthDate_RequestDetails(t *testing.T) {
	mock := &mockHTTPClient{response: restOKResponse()}
	c := newTestNsfwClient(mock)

	err := c.SetBirthDate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	call := mock.calls[0]

	if call.URL != setBirthURL {
		t.Errorf("URL = %s, want %s", call.URL, setBirthURL)
	}
	if call.ContentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", call.ContentType)
	}
	parsed, _ := url.Parse(call.Origin)
	if parsed.Host != "grok.com" {
		t.Errorf("Origin host = %s, want grok.com", parsed.Host)
	}
	if call.Referer != "https://grok.com/?_s=data" {
		t.Errorf("Referer = %s, want https://grok.com/?_s=data", call.Referer)
	}
	if call.HasGRPCWeb {
		t.Error("SetBirthDate should NOT have x-grpc-web header")
	}
	if !strings.Contains(string(call.Body), "birthDate") {
		t.Error("body should contain birthDate")
	}
}

func TestSetBirthDate_RejectsHTMLResponse(t *testing.T) {
	htmlResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`<html><body>Login</body></html>`)),
		Header:     http.Header{},
	}
	mock := &mockHTTPClient{response: htmlResp}
	c := newTestNsfwClient(mock)

	err := c.SetBirthDate(context.Background())
	if err == nil {
		t.Fatal("expected error for HTML response")
	}
	if !strings.Contains(err.Error(), "non-JSON") {
		t.Errorf("error should mention non-JSON: %v", err)
	}
}

func TestSetBirthDate_Rejects302(t *testing.T) {
	redirectResp := &http.Response{
		StatusCode: 302,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{},
	}
	mock := &mockHTTPClient{response: redirectResp}
	c := newTestNsfwClient(mock)

	err := c.SetBirthDate(context.Background())
	if err == nil {
		t.Fatal("expected error for 302 redirect")
	}
	if !strings.Contains(err.Error(), "HTTP 302") {
		t.Errorf("error should mention HTTP 302: %v", err)
	}
}

func TestSetNSFW_RequestDetails(t *testing.T) {
	mock := &mockHTTPClient{response: grpcOKResponse()}
	c := newTestNsfwClient(mock)

	err := c.SetNSFW(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	call := mock.calls[0]

	if call.URL != setNsfwURL {
		t.Errorf("URL = %s, want %s", call.URL, setNsfwURL)
	}
	if call.ContentType != "application/grpc-web+proto" {
		t.Errorf("Content-Type = %s, want application/grpc-web+proto", call.ContentType)
	}
	if call.Referer != "https://grok.com/?_s=data" {
		t.Errorf("Referer = %s, want https://grok.com/?_s=data", call.Referer)
	}
	if !call.HasGRPCWeb {
		t.Error("expected x-grpc-web: 1 header")
	}
}

func TestDoGRPCCall_RejectsNoTrailer(t *testing.T) {
	// Response with data frame only, no trailer — should fail after Fix 1
	dataOnly := grpcwebEncode([]byte{0x10, 0x01})
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(dataOnly)),
		Header:     http.Header{},
	}
	mock := &mockHTTPClient{response: resp}
	c := newTestNsfwClient(mock)

	err := c.AcceptTOS(context.Background())
	if err == nil {
		t.Fatal("expected error when trailer is missing")
	}
	if !strings.Contains(err.Error(), "parse trailers") {
		t.Errorf("error should mention parse trailers: %v", err)
	}
}

// --- EnableNSFW sequence tests ---

func TestEnableNSFW_FullSequence(t *testing.T) {
	responses := []*http.Response{
		grpcOKResponse(),  // AcceptTOS
		restOKResponse(),  // SetBirthDate
		grpcOKResponse(),  // SetNSFW
	}
	seqMock := &sequentialMock{responses: responses}
	c := &NsfwClient{
		token:     "test-token",
		opts:      DefaultOptions(),
		statsigID: staticStatsigID,
		http:      seqMock,
	}

	err := EnableNSFW(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(seqMock.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(seqMock.calls))
	}

	if seqMock.calls[0].URL != acceptTOSURL {
		t.Errorf("call 0: URL = %s, want %s", seqMock.calls[0].URL, acceptTOSURL)
	}
	if seqMock.calls[1].URL != setBirthURL {
		t.Errorf("call 1: URL = %s, want %s", seqMock.calls[1].URL, setBirthURL)
	}
	if seqMock.calls[2].URL != setNsfwURL {
		t.Errorf("call 2: URL = %s, want %s", seqMock.calls[2].URL, setNsfwURL)
	}
}

func TestEnableNSFW_StopsOnFirstFailure(t *testing.T) {
	failResp := &http.Response{
		StatusCode: 403,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{},
	}
	seqMock := &sequentialMock{responses: []*http.Response{failResp}}
	c := &NsfwClient{
		token:     "test-token",
		opts:      DefaultOptions(),
		statsigID: staticStatsigID,
		http:      seqMock,
	}

	err := EnableNSFW(context.Background(), c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "accept_tos") {
		t.Errorf("error should mention accept_tos: %v", err)
	}
	if len(seqMock.calls) != 1 {
		t.Errorf("expected 1 call (stopped early), got %d", len(seqMock.calls))
	}
}

func TestEnableNSFW_StopsOnSecondFailure(t *testing.T) {
	failResp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{},
	}
	seqMock := &sequentialMock{responses: []*http.Response{
		grpcOKResponse(), // AcceptTOS OK
		failResp,         // SetBirthDate fails
	}}
	c := &NsfwClient{
		token:     "test-token",
		opts:      DefaultOptions(),
		statsigID: staticStatsigID,
		http:      seqMock,
	}

	err := EnableNSFW(context.Background(), c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "set_birth_date") {
		t.Errorf("error should mention set_birth_date: %v", err)
	}
	if len(seqMock.calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(seqMock.calls))
	}
}

// sequentialMock returns responses in order.
type sequentialMock struct {
	calls     []mockCall
	responses []*http.Response
	idx       int
}

func (m *sequentialMock) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	m.calls = append(m.calls, mockCall{
		URL:         req.URL.String(),
		Method:      req.Method,
		ContentType: req.Header.Get("Content-Type"),
		Origin:      req.Header.Get("Origin"),
		Referer:     req.Header.Get("Referer"),
		Cookie:      req.Header.Get("Cookie"),
		Body:        body,
		HasGRPCWeb:  req.Header.Get("x-grpc-web") == "1",
	})
	if m.idx >= len(m.responses) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader("no more responses")),
			Header:     http.Header{},
		}, nil
	}
	resp := m.responses[m.idx]
	m.idx++
	return resp, nil
}
