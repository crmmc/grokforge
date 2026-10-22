package xai

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trailerFrame builds a gRPC-Web trailer frame carrying the raw payload.
func trailerFrame(payload string) []byte {
	buf := make([]byte, 5+len(payload))
	buf[0] = 0x80
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	return buf
}

func TestGrpcwebParseTrailersTrailerWithoutStatus(t *testing.T) {
	body := trailerFrame("grpc-message: oops\r\n")
	_, _, err := grpcwebParseTrailers(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing grpc-status")
}

func TestGrpcwebParseTrailersInvalidStatus(t *testing.T) {
	body := trailerFrame("grpc-status: fast\r\ngrpc-message: bad\r\n")
	_, _, err := grpcwebParseTrailers(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid grpc-status")
}

func TestGrpcwebParseTrailersTruncatedFrames(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{"short trailing bytes", append(grpcwebEncode([]byte{0x10, 0x01}), 0x80, 0x00)},
		{"length exceeds body", []byte{0x00, 0x00, 0x00, 0x00, 0xC8, 0x01, 0x02, 0x03}},
		{"empty body", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := grpcwebParseTrailers(tt.body)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no trailer frame")
		})
	}
}

func TestGrpcwebParseTrailersDataFrameThenTrailer(t *testing.T) {
	body := append(grpcwebEncode([]byte{0x10, 0x01}), trailerFrame("grpc-status:0\r\ngrpc-message:ok\r\n")...)
	code, msg, err := grpcwebParseTrailers(body)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ok", msg)
}

func TestParseTrailerPayloadSkipsLinesWithoutColon(t *testing.T) {
	m := parseTrailerPayload([]byte("garbage-line\r\ngrpc-status: 3\r\nGrpc-Message: hi\r\n\r\n"))
	assert.Equal(t, "3", m["grpc-status"])
	assert.Equal(t, "hi", m["grpc-message"])
	assert.Len(t, m, 2)
}
