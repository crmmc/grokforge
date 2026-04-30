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

func (f *ChatFlow) parseEvent(event xai.StreamEvent) StreamEvent {
	// Parse the raw JSON from xai event — field names match xAI's actual API:
	// "token" = text chunk, "isThinking" = boolean flag for reasoning content.
	var result struct {
		Result struct {
			Response struct {
				Token      string `json:"token"`
				IsThinking bool   `json:"isThinking"`
				RolloutID  string `json:"rolloutId"`
				// modelResponse contains generated images and final message
				ModelResponse *struct {
					Message            string   `json:"message"`
					GeneratedImageUrls []string `json:"generatedImageUrls"`
				} `json:"modelResponse"`
				// cardAttachment contains external image/link cards
				CardAttachment *struct {
					JSONData string `json:"jsonData"`
				} `json:"cardAttachment"`
				// webSearchResults contains web search citations
				WebSearchResults *struct {
					Results []struct {
						URL   string `json:"url"`
						Title string `json:"title"`
					} `json:"results"`
				} `json:"webSearchResults"`
				// xSearchResults contains X/Twitter post citations
				XSearchResults *struct {
					Results []struct {
						PostID   string `json:"postId"`
						Username string `json:"username"`
						Text     string `json:"text"`
					} `json:"results"`
				} `json:"xSearchResults"`
			} `json:"response"`
		} `json:"result"`
	}
	if err := json.Unmarshal(event.Data, &result); err != nil {
		return StreamEvent{Error: err}
	}

	resp := result.Result.Response
	token := resp.Token

	if resp.IsThinking {
		slog.Debug("flow: thinking token received", "len", len(token))
	}

	var content, reasoning string

	// Route token based on isThinking flag
	if resp.IsThinking {
		reasoning = token
	} else {
		content = token
	}

	// Extract images from modelResponse
	if mr := resp.ModelResponse; mr != nil {
		for _, imgURL := range mr.GeneratedImageUrls {
			parts := strings.Split(imgURL, "/")
			imgID := "image"
			if len(parts) >= 2 {
				imgID = parts[len(parts)-2]
			}
			content += fmt.Sprintf("\n![%s](%s)", imgID, imgURL)
		}
	}

	// Extract images from cardAttachment
	if ca := resp.CardAttachment; ca != nil && ca.JSONData != "" {
		var card struct {
			Image struct {
				Original string `json:"original"`
				Title    string `json:"title"`
			} `json:"image"`
		}
		if json.Unmarshal([]byte(ca.JSONData), &card) == nil && card.Image.Original != "" {
			title := strings.ReplaceAll(strings.TrimSpace(card.Image.Title), "\n", " ")
			if title == "" {
				title = "image"
			}
			content += fmt.Sprintf("\n![%s](%s)", title, card.Image.Original)
		}
	}

	// Extract search sources from web and X search results
	var sources []SearchSource
	if wsr := resp.WebSearchResults; wsr != nil {
		for _, item := range wsr.Results {
			if item.URL != "" {
				sources = append(sources, SearchSource{
					URL:   item.URL,
					Title: item.Title,
					Type:  "web",
				})
			}
		}
	}
	if xsr := resp.XSearchResults; xsr != nil {
		for _, item := range xsr.Results {
			if item.PostID != "" && item.Username != "" {
				sources = append(sources, SearchSource{
					URL:   fmt.Sprintf("https://x.com/%s/status/%s", item.Username, item.PostID),
					Title: normalizeXTitle(item.Username, item.Text),
					Type:  "x_post",
				})
			}
		}
	}

	// Parse tool calls from response content
	return StreamEvent{
		Content:          content,
		ReasoningContent: reasoning,
		IsThinking:       resp.IsThinking,
		RolloutID:        strings.TrimSpace(resp.RolloutID),
		SearchSources:    sources,
	}
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
