package xai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNsfwClient(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		c, err := NewNsfwClient("nsfw-token")
		require.NoError(t, err)
		defer c.Close()
		assert.Equal(t, "nsfw-token", c.token)
		assert.Empty(t, c.statsigID, "dynamic statsig is the default")
		assert.NotNil(t, c.http)
	})

	t.Run("dynamic statsig disabled", func(t *testing.T) {
		c, err := NewNsfwClient("nsfw-token", WithDynamicStatsig(false))
		require.NoError(t, err)
		defer c.Close()
		assert.Equal(t, staticStatsigID, c.statsigID)
	})
}

func TestNsfwGRPCCallErrors(t *testing.T) {
	tests := []struct {
		name    string
		doer    *mockHTTPClient
		call    func(c *NsfwClient) error
		wantErr string
	}{
		{
			name:    "accept_tos transport error",
			doer:    &mockHTTPClient{err: errors.New("network down")},
			call:    func(c *NsfwClient) error { return c.AcceptTOS(context.Background()) },
			wantErr: "nsfw: accept_tos:",
		},
		{
			name: "accept_tos grpc error code",
			doer: &mockHTTPClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(string(errorTrailer(7, "not allowed")))),
			}},
			call:    func(c *NsfwClient) error { return c.AcceptTOS(context.Background()) },
			wantErr: "gRPC error code=7",
		},
		{
			name: "accept_tos body read error",
			doer: &mockHTTPClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
			}},
			call:    func(c *NsfwClient) error { return c.AcceptTOS(context.Background()) },
			wantErr: "read body",
		},
		{
			name:    "set_birth_date transport error",
			doer:    &mockHTTPClient{err: errors.New("network down")},
			call:    func(c *NsfwClient) error { return c.SetBirthDate(context.Background()) },
			wantErr: "nsfw: set_birth_date:",
		},
		{
			name: "set_birth_date body read error",
			doer: &mockHTTPClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
			}},
			call:    func(c *NsfwClient) error { return c.SetBirthDate(context.Background()) },
			wantErr: "read body",
		},
		{
			name: "set_nsfw grpc error code",
			doer: &mockHTTPClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(string(errorTrailer(9, "denied")))),
			}},
			call:    func(c *NsfwClient) error { return c.SetNSFW(context.Background(), true) },
			wantErr: "gRPC error code=9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestNsfwClient(tt.doer)
			err := tt.call(c)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSetBirthDateAcceptedStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"200", http.StatusOK},
		{"201", http.StatusCreated},
		{"204", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockHTTPClient{response: &http.Response{
				StatusCode: tt.status,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}}
			c := newTestNsfwClient(mock)
			require.NoError(t, c.SetBirthDate(context.Background()))
		})
	}
}

func TestEnableNSFWStopsOnThirdFailure(t *testing.T) {
	seqMock := &sequentialMock{responses: []*http.Response{
		grpcOKResponse(), // AcceptTOS OK
		restOKResponse(), // SetBirthDate OK
		{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))},
	}}
	c := &NsfwClient{
		token:     "test-token",
		opts:      DefaultOptions(),
		statsigID: staticStatsigID,
		http:      seqMock,
	}

	err := EnableNSFW(context.Background(), c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 3 (set_nsfw)")
	assert.Len(t, seqMock.calls, 3)
}
