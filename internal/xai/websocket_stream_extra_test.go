package xai

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseImagineResponse(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		want    *ImagineResponse
		wantErr bool
	}{
		{
			name: "typed passthrough",
			msg:  `{"type":"image.preview","item":{"requestId":"r1","imageData":"d1"}}`,
			want: &ImagineResponse{Type: "image.preview", Item: ImagineResponseItem{RequestID: "r1", ImageData: "d1"}},
		},
		{
			name: "done passthrough",
			msg:  `{"type":"done"}`,
			want: &ImagineResponse{Type: "done"},
		},
		{
			name:    "malformed json",
			msg:     `{"type":`,
			wantErr: true,
		},
		{
			name:    "legacy image without blob",
			msg:     `{"type":"image"}`,
			wantErr: true,
		},
		{
			name:    "legacy image with wrong requestId type",
			msg:     `{"type":"image","requestId":123,"blob":"abc"}`,
			wantErr: true,
		},
		{
			name: "legacy preview from raw blob",
			msg:  `{"type":"image","requestId":"r2","blob":"` + strings.Repeat("p", 120) + `"}`,
			want: &ImagineResponse{Type: "image.preview", Item: ImagineResponseItem{RequestID: "r2", ImageData: strings.Repeat("p", 120)}},
		},
		{
			name: "legacy final from data uri",
			msg:  `{"type":"image","url":"https://grok.com/images/f130879e-dbf7-49fc-8095-816eb37f6b22.png","blob":"data:image/png;base64,` + strings.Repeat("f", legacyFinalMinBytes+1) + `"}`,
			want: &ImagineResponse{Type: "image.final", Item: ImagineResponseItem{RequestID: "f130879e-dbf7-49fc-8095-816eb37f6b22", ImageData: strings.Repeat("f", legacyFinalMinBytes+1)}},
		},
		{
			name: "legacy medium classified by size",
			msg:  `{"type":"image","url":"https://example.com/x/y.gif","blob":"` + strings.Repeat("m", legacyMediumMinBytes+1) + `"}`,
			want: &ImagineResponse{Type: "image.medium", Item: ImagineResponseItem{RequestID: "", ImageData: strings.Repeat("m", legacyMediumMinBytes+1)}},
		},
		{
			name: "error with err_msg",
			msg:  `{"type":"error","err_msg":"boom"}`,
			want: &ImagineResponse{Type: "error", Error: errors.New("boom")},
		},
		{
			name: "error with message",
			msg:  `{"type":"error","message":"msg fallback"}`,
			want: &ImagineResponse{Type: "error", Error: errors.New("msg fallback")},
		},
		{
			name: "error with error field",
			msg:  `{"type":"error","error":"err fallback"}`,
			want: &ImagineResponse{Type: "error", Error: errors.New("err fallback")},
		},
		{
			name: "error with nothing falls back to server error",
			msg:  `{"type":"error"}`,
			want: &ImagineResponse{Type: "error", Error: errors.New("server error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseImagineResponse([]byte(tt.msg))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want.Type, got.Type)
			assert.Equal(t, tt.want.Item, got.Item)
			if tt.want.Error != nil {
				require.Error(t, got.Error)
				assert.Equal(t, tt.want.Error.Error(), got.Error.Error())
			}
		})
	}
}

func TestNormalizeLegacyBlob(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"data uri stripped", "data:image/png;base64, AAA ", "AAA"},
		{"plain blob untouched", " raw ", "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeLegacyBlob(tt.in))
		})
	}
}

func TestParseLegacyImageID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"matching png", "https://grok.com/images/f130879e-dbf7-49fc-8095-816eb37f6b22.png", "f130879e-dbf7-49fc-8095-816eb37f6b22"},
		{"matching jpg", "https://grok.com/images/9d560d18-c143-46ef-a4cb-7b7537c6bbeb.jpg", "9d560d18-c143-46ef-a4cb-7b7537c6bbeb"},
		{"non image extension", "https://grok.com/images/x.gif", ""},
		{"no path", "https://grok.com/", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseLegacyImageID(tt.in))
		})
	}
}

func TestStreamImagesErrorResponse(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(map[string]any{"type": "error", "err_msg": "content policy"})
	})
	defer server.Close()

	c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok", blockedGraceMillis: 10000}
	eventCh, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.NoError(t, err)

	events := collectImageEvents(t, eventCh)
	require.Len(t, events, 1)
	assert.Equal(t, ImageEventError, events[0].Type)
	require.Error(t, events[0].Error)
	assert.Contains(t, events[0].Error.Error(), "content policy")
}

func TestStreamImagesSkipsMalformedThenDone(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("not-json"))
		_ = conn.WriteJSON(ImagineResponse{Type: "done"})
	})
	defer server.Close()

	c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok", blockedGraceMillis: 10000}
	eventCh, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.NoError(t, err)

	assert.Empty(t, collectImageEvents(t, eventCh), "malformed message should be skipped without events")
}

func TestStreamImagesLegacyInvalidSkipped(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(map[string]any{"type": "image"})
		_ = conn.WriteJSON(ImagineResponse{Type: "done"})
	})
	defer server.Close()

	c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok", blockedGraceMillis: 10000}
	eventCh, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.NoError(t, err)

	assert.Empty(t, collectImageEvents(t, eventCh))
}

func TestStreamImagesMediumTwiceThenFinal(t *testing.T) {
	requestID := "req-med-1"
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(ImagineResponse{
			Type: "image.medium",
			Item: ImagineResponseItem{RequestID: requestID, ImageData: "m1"},
		})
		_ = conn.WriteJSON(ImagineResponse{
			Type: "image.medium",
			Item: ImagineResponseItem{RequestID: requestID, ImageData: "m2"},
		})
		_ = conn.WriteJSON(ImagineResponse{
			Type: "image.final",
			Item: ImagineResponseItem{RequestID: requestID, ImageData: "final"},
		})
	})
	defer server.Close()

	c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok", blockedGraceMillis: 10000}
	eventCh, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.NoError(t, err)

	events := collectImageEvents(t, eventCh)
	require.Len(t, events, 3)
	assert.Equal(t, []ImageEventType{ImageEventMedium, ImageEventMedium, ImageEventFinal},
		[]ImageEventType{events[0].Type, events[1].Type, events[2].Type})
	assert.Equal(t, "final", events[2].ImageData)
}

func TestStreamImagesReadErrorWithoutFinal(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		_ = conn.UnderlyingConn().Close() // abrupt close, no close frame
	})
	defer server.Close()

	c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok", blockedGraceMillis: 10000}
	eventCh, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.NoError(t, err)

	events := collectImageEvents(t, eventCh)
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	assert.Equal(t, ImageEventError, last.Type)
	require.Error(t, last.Error)
	assert.Contains(t, last.Error.Error(), "read message")
}

func TestStreamImagesNormalCloseIsSilent(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		deadline := time.Now().Add(2 * time.Second)
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"), deadline)
	})
	defer server.Close()

	c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok", blockedGraceMillis: 10000}
	eventCh, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.NoError(t, err)

	events := collectImageEvents(t, eventCh)
	assert.Empty(t, events, "normal close frame should end the stream without an error event")
}

func TestStreamImagesContextCancelAtLoopTop(t *testing.T) {
	block := make(chan struct{})
	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage() // keep the connection open
		<-block
	})
	t.Cleanup(func() {
		close(block)
		server.Close()
	})

	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(context.Background(), wsURL(server.URL), nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &ImagineClient{blockedGraceMillis: 10000}
	eventCh := make(chan ImageEvent, 4)
	c.streamImages(ctx, conn, "rid", "p", "1:1", false, false, eventCh)

	events := collectImageEvents(t, eventCh)
	require.Len(t, events, 1)
	assert.Equal(t, ImageEventError, events[0].Type)
	assert.ErrorIs(t, events[0].Error, context.Canceled)
}

// ---------- deterministic write-error conns ----------

type stubAddr struct{}

func (stubAddr) Network() string { return "stub" }
func (stubAddr) String() string  { return "stub" }

// stubNetConn is a net.Conn whose writes can be made to fail on demand.
type stubNetConn struct {
	mu        sync.Mutex
	writes    int
	failAll   bool
	failAfter int // 0 = never
}

func (c *stubNetConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	if c.failAll || (c.failAfter > 0 && c.writes > c.failAfter) {
		return 0, errors.New("stub write failure")
	}
	return len(p), nil
}

func (c *stubNetConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *stubNetConn) Close() error                     { return nil }
func (c *stubNetConn) LocalAddr() net.Addr              { return stubAddr{} }
func (c *stubNetConn) RemoteAddr() net.Addr             { return stubAddr{} }
func (c *stubNetConn) SetDeadline(time.Time) error      { return nil }
func (c *stubNetConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stubNetConn) SetWriteDeadline(time.Time) error { return nil }

// hijackRecorder is an http.ResponseWriter that hijacks into a stub conn.
type hijackRecorder struct {
	header http.Header
	conn   *stubNetConn
}

func (h *hijackRecorder) Header() http.Header         { return h.header }
func (h *hijackRecorder) Write(p []byte) (int, error) { return len(p), nil }
func (h *hijackRecorder) WriteHeader(int)             {}
func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	br := bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(h.conn))
	return h.conn, br, nil
}

// newStubWSConn upgrades a fake request over the given conn to obtain a live
// *websocket.Conn whose writes are fully controlled by the test.
func newStubWSConn(t *testing.T, conn *stubNetConn) *websocket.Conn {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	req := httptest.NewRequest(http.MethodGet, "http://example.test/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", key)

	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	wsConn, err := up.Upgrade(&hijackRecorder{header: http.Header{}, conn: conn}, req, nil)
	require.NoError(t, err)
	return wsConn
}

func collectImageEvents(t *testing.T, ch <-chan ImageEvent) []ImageEvent {
	t.Helper()
	var out []ImageEvent
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("timed out collecting image events")
		}
	}
}

func TestStreamImagesWriteResetError(t *testing.T) {
	conn := &stubNetConn{}
	wsConn := newStubWSConn(t, conn)
	conn.mu.Lock()
	conn.failAll = true
	conn.mu.Unlock()

	c := &ImagineClient{blockedGraceMillis: 10000}
	eventCh := make(chan ImageEvent, 4)
	c.streamImages(context.Background(), wsConn, "rid", "p", "1:1", false, false, eventCh)

	events := collectImageEvents(t, eventCh)
	require.Len(t, events, 1)
	assert.Equal(t, ImageEventError, events[0].Type)
	require.Error(t, events[0].Error)
	assert.Contains(t, events[0].Error.Error(), "write reset")
}

func TestStreamImagesWriteRequestError(t *testing.T) {
	conn := &stubNetConn{}
	wsConn := newStubWSConn(t, conn)
	conn.mu.Lock()
	// First streamImages write (reset) succeeds, second (request) fails,
	// regardless of how many writes the handshake consumed.
	conn.failAfter = conn.writes + 1
	conn.mu.Unlock()

	c := &ImagineClient{blockedGraceMillis: 10000}
	eventCh := make(chan ImageEvent, 4)
	c.streamImages(context.Background(), wsConn, "rid", "p", "1:1", false, false, eventCh)

	events := collectImageEvents(t, eventCh)
	require.Len(t, events, 1)
	assert.Equal(t, ImageEventError, events[0].Type)
	require.Error(t, events[0].Error)
	assert.Contains(t, events[0].Error.Error(), "write request")
}

func TestBuildImagineRequestShape(t *testing.T) {
	req := buildImagineRequest("rid", "prompt", "16:9", true, true)
	assert.Equal(t, "conversation.item.create", req.Type)
	require.Len(t, req.Item.Content, 1)
	content := req.Item.Content[0]
	assert.Equal(t, "rid", content.RequestID)
	assert.Equal(t, "prompt", content.Text)
	assert.Equal(t, "input_text", content.Type)
	assert.Equal(t, "16:9", content.Properties.AspectRatio)
	assert.True(t, content.Properties.EnableNSFW)
	assert.True(t, content.Properties.EnablePro)
	assert.True(t, content.Properties.EnableSideBySide)

	// JSON round-trips through the same shape used on the wire.
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"enable_nsfw":true`)
}
