package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
)

const chatAssetsGrokBaseURL = "https://assets.grok.com/"

type chatStreamPayload struct {
	Result struct {
		Response chatStreamResponse `json:"response"`
	} `json:"result"`
}

type chatStreamResponse struct {
	Token            string                `json:"token"`
	IsThinking       bool                  `json:"isThinking"`
	RolloutID        string                `json:"rolloutId"`
	ModelResponse    *chatModelResponse    `json:"modelResponse"`
	CardAttachment   *chatCardAttachment   `json:"cardAttachment"`
	WebSearchResults *chatWebSearchResults `json:"webSearchResults"`
	XSearchResults   *chatXSearchResults   `json:"xSearchResults"`
}

type chatModelResponse struct {
	GeneratedImageUrls []string `json:"generatedImageUrls"`
}

type chatCardAttachment struct {
	JSONData string `json:"jsonData"`
}

type chatWebSearchResults struct {
	Results []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"results"`
}

type chatXSearchResults struct {
	Results []struct {
		PostID   string `json:"postId"`
		Username string `json:"username"`
		Text     string `json:"text"`
	} `json:"results"`
}

var markdownImageAltReplacer = strings.NewReplacer("[", " ", "]", " ")

func (f *ChatFlow) parseEvent(event xai.StreamEvent) StreamEvent {
	// Parse the raw JSON from xai event — field names match xAI's actual API:
	// "token" = text chunk, "isThinking" = boolean flag for reasoning content.
	var result chatStreamPayload
	if err := json.Unmarshal(event.Data, &result); err != nil {
		return StreamEvent{Error: err}
	}

	resp := result.Result.Response
	if resp.IsThinking {
		slog.Debug("flow: thinking token received", "len", len(resp.Token))
	}

	content, reasoning := splitThinkingToken(resp.Token, resp.IsThinking)
	content = appendModelResponseImages(content, resp.ModelResponse)
	content = appendCardAttachmentImage(content, resp.CardAttachment)

	return StreamEvent{
		Content:          content,
		ReasoningContent: reasoning,
		IsThinking:       resp.IsThinking,
		RolloutID:        strings.TrimSpace(resp.RolloutID),
		SearchSources:    collectSearchSources(resp),
	}
}

func splitThinkingToken(token string, isThinking bool) (string, string) {
	if isThinking {
		return "", token
	}
	return token, ""
}

func appendModelResponseImages(content string, mr *chatModelResponse) string {
	if mr == nil {
		return content
	}
	for _, imgURL := range mr.GeneratedImageUrls {
		content = appendMarkdownImage(content, generatedImageAlt(imgURL), imgURL)
	}
	return content
}

func generatedImageAlt(imgURL string) string {
	parts := strings.Split(imgURL, "/")
	if len(parts) < 2 {
		return "image"
	}
	return parts[len(parts)-2]
}

func appendCardAttachmentImage(content string, ca *chatCardAttachment) string {
	if ca == nil || ca.JSONData == "" {
		return content
	}
	var card struct {
		Image struct {
			Original string `json:"original"`
			Title    string `json:"title"`
		} `json:"image"`
		ImageChunk struct {
			ImageURL  string `json:"imageUrl"`
			ImageUUID string `json:"imageUuid"`
			Progress  int    `json:"progress"`
			Moderated bool   `json:"moderated"`
		} `json:"image_chunk"`
	}
	if json.Unmarshal([]byte(ca.JSONData), &card) != nil {
		return content
	}
	if strings.TrimSpace(card.Image.Original) != "" {
		return appendMarkdownImage(content, card.Image.Title, cardAttachmentImageURL(card.Image.Original))
	}
	if card.ImageChunk.Progress == 100 &&
		!card.ImageChunk.Moderated &&
		strings.TrimSpace(card.ImageChunk.ImageURL) != "" {
		return appendMarkdownImage(content, card.ImageChunk.ImageUUID, cardAttachmentImageURL(card.ImageChunk.ImageURL))
	}
	return content
}

func cardAttachmentImageURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "//") {
		return "https:" + trimmed
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return trimmed
	}
	return chatAssetsGrokBaseURL + strings.TrimLeft(trimmed, "/")
}

func appendMarkdownImage(content, alt, target string) string {
	trimmedTarget := strings.TrimSpace(target)
	if trimmedTarget == "" {
		return content
	}
	return content + fmt.Sprintf("\n![%s](%s)", markdownImageAlt(alt), trimmedTarget)
}

func markdownImageAlt(value string) string {
	normalized := strings.Join(strings.Fields(value), " ")
	normalized = markdownImageAltReplacer.Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" {
		return "image"
	}
	return normalized
}

func collectSearchSources(resp chatStreamResponse) []SearchSource {
	sources := appendWebSearchSources(nil, resp.WebSearchResults)
	return appendXSearchSources(sources, resp.XSearchResults)
}

func appendWebSearchSources(sources []SearchSource, wsr *chatWebSearchResults) []SearchSource {
	if wsr == nil {
		return sources
	}
	for _, item := range wsr.Results {
		if item.URL != "" {
			sources = append(sources, SearchSource{URL: item.URL, Title: item.Title, Type: "web"})
		}
	}
	return sources
}

func appendXSearchSources(sources []SearchSource, xsr *chatXSearchResults) []SearchSource {
	if xsr == nil {
		return sources
	}
	for _, item := range xsr.Results {
		if item.PostID != "" && item.Username != "" {
			sources = append(sources, SearchSource{
				URL:   fmt.Sprintf("https://x.com/%s/status/%s", item.Username, item.PostID),
				Title: normalizeXTitle(item.Username, item.Text),
				Type:  "x_post",
			})
		}
	}
	return sources
}

// estimatePromptTokens estimates input token count from request messages.
func (f *ChatFlow) estimatePromptTokens(req *ChatRequest) int {
	var chars int
	for _, m := range req.Messages {
		chars += len(m.Role)
		switch c := m.Content.(type) {
		case string:
			chars += len(c)
		}
	}
	return estimateTokens(chars)
}

// recordUsage records an API usage log entry via the buffer (non-blocking).
func (f *ChatFlow) recordUsage(apiKeyID uint, tokenID uint, model, endpoint string, status int, latency time.Duration, ttft time.Duration, tokensInput, tokensOutput int, estimated bool) {
	if f.usageLog == nil {
		return
	}
	_ = f.usageLog.Record(context.Background(), &store.UsageLog{
		APIKeyID:     apiKeyID,
		TokenID:      tokenID,
		Model:        model,
		Endpoint:     endpoint,
		Status:       status,
		DurationMs:   latency.Milliseconds(),
		TTFTMs:       int(ttft.Milliseconds()),
		CacheTokens:  0,
		TokensInput:  tokensInput,
		TokensOutput: tokensOutput,
		Estimated:    estimated,
		CreatedAt:    time.Now(),
	})
}

// normalizeXTitle builds a display title for an X/Twitter post.
// Uses the first 50 chars of text, falling back to "𝕏/@username".
func normalizeXTitle(username, text string) string {
	// Normalize whitespace
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "𝕏/@" + username
	}
	// Truncate to 50 runes
	runes := []rune(text)
	if len(runes) > 50 {
		return string(runes[:50]) + "…"
	}
	return text
}
