package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type strictPayload struct {
	Name string `json:"name"`
}

func TestDecodeJSONBodyStrict(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		wantVal strictPayload
	}{
		{name: "single valid object", body: `{"name":"a"}`, wantVal: strictPayload{Name: "a"}},
		{name: "empty body is EOF error", body: ``, wantErr: true},
		{name: "malformed json", body: `{`, wantErr: true},
		{name: "unknown field rejected", body: `{"name":"a","extra":1}`, wantErr: true},
		{name: "two json values rejected", body: `{"name":"a"} {"name":"b"}`, wantErr: true},
		{name: "trailing garbage rejected", body: `{"name":"a"} junk`, wantErr: true},
		{name: "second empty object rejected as extra value", body: `{"name":"a"} {}`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tc.body))
			var got strictPayload

			err := decodeJSONBodyStrict(req, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantVal, got)
		})
	}
}
