package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/xai"
)

type chatImageLiteClientMock struct {
	imageURL     string
	downloadBody []byte
	downloadURLs []string
}

func (m *chatImageLiteClientMock) Chat(_ context.Context, _ *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	payload := `{"result":{"response":{"modelResponse":{"generatedImageUrls":["` + m.imageURL + `"]}}}}`
	ch := make(chan xai.StreamEvent, 1)
	ch <- xai.StreamEvent{Data: json.RawMessage(payload)}
	close(ch)
	return ch, nil
}

func (m *chatImageLiteClientMock) UploadFile(context.Context, string, string, string) (string, string, error) {
	return "file-1", "users/u/generated/ref/content", nil
}

func (m *chatImageLiteClientMock) CreateImagePost(context.Context, string) (string, error) {
	return "post-1", nil
}

func (m *chatImageLiteClientMock) DownloadURL(_ context.Context, rawURL string) ([]byte, error) {
	m.downloadURLs = append(m.downloadURLs, rawURL)
	return m.downloadBody, nil
}

func TestHandleChat_ImageLiteRoute_ForcesBase64WhenURLRequested(t *testing.T) {
	upstreamURL := "https://assets.grok.com/users/u/generated/id/image.png"
	imageBytes := []byte("image-bytes")
	client := &chatImageLiteClientMock{
		imageURL:     upstreamURL,
		downloadBody: imageBytes,
	}
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator {
		return nil
	})
	imageFlow.SetEditClientFactory(func(token string) flow.ImageEditClient {
		return client
	})
	imageFlow.SetModelResolver(flowTestModelResolver())

	s := httpapi.NewServer(&httpapi.ServerConfig{
		ChatProvider: &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()},
	})
	body := `{"model":"grok-imagine-image-lite","messages":[{"role":"user","content":"draw"}],"image_config":{"response_format":"url"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), upstreamURL) {
		t.Fatalf("response leaked upstream URL: %s", w.Body.String())
	}
	wantB64 := base64.StdEncoding.EncodeToString(imageBytes)
	if !strings.Contains(w.Body.String(), wantB64) {
		t.Fatalf("response missing base64 image data: %s", w.Body.String())
	}
	if len(client.downloadURLs) != 1 || client.downloadURLs[0] != upstreamURL {
		t.Fatalf("downloadURLs = %v, want [%s]", client.downloadURLs, upstreamURL)
	}
}

func TestHandleChat_ImageEditRoute_ForcesBase64WhenURLRequested(t *testing.T) {
	upstreamURL := "https://assets.grok.com/users/u/generated/id/image.png"
	imageBytes := []byte("edited-image-bytes")
	client := &chatImageLiteClientMock{
		imageURL:     upstreamURL,
		downloadBody: imageBytes,
	}
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator {
		return nil
	})
	imageFlow.SetEditClientFactory(func(token string) flow.ImageEditClient {
		return client
	})
	imageFlow.SetModelResolver(flowTestModelResolver())

	s := httpapi.NewServer(&httpapi.ServerConfig{
		ChatProvider: &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()},
	})
	body := `{"model":"grok-imagine-image-edit","messages":[{"role":"user","content":[{"type":"text","text":"edit this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}}]}],"image_config":{"response_format":"url"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), upstreamURL) {
		t.Fatalf("response leaked upstream URL: %s", w.Body.String())
	}
	wantB64 := base64.StdEncoding.EncodeToString(imageBytes)
	if !strings.Contains(w.Body.String(), wantB64) {
		t.Fatalf("response missing base64 image data: %s", w.Body.String())
	}
	if len(client.downloadURLs) != 1 || client.downloadURLs[0] != upstreamURL {
		t.Fatalf("downloadURLs = %v, want [%s]", client.downloadURLs, upstreamURL)
	}
}

func TestHandleChat_ImageLiteRoute_InvalidResponseFormat(t *testing.T) {
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator {
		return nil
	})
	s := httpapi.NewServer(&httpapi.ServerConfig{
		ChatProvider: &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()},
	})
	body := `{"model":"grok-imagine-image-lite","messages":[{"role":"user","content":"draw"}],"image_config":{"response_format":"bogus"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_image_config") {
		t.Fatalf("response missing invalid_image_config: %s", w.Body.String())
	}
}
