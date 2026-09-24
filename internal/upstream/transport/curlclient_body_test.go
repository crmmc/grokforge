package transport

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlClient_Do_BodyReadError(t *testing.T) {
	client, err := newCurlClient(Options{})
	require.NoError(t, err)

	wantErr := errors.New("read failed")
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/upload", &trackingBody{Reader: errReader{err: wantErr}})
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.ErrorIs(t, err, wantErr, "body read error must propagate from Do")
	assert.Nil(t, resp)
}
