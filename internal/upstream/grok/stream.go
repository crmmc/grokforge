package grok

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
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

func (g *GrokUpstream) parseStream(ctx context.Context, token string, body io.Reader, ch chan<- upstream.StreamEvent) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	downloader := g.downloadFunc(token)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		var raw string
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			if data == "[DONE]" {
				return nil
			}
			raw = data
		} else if strings.HasPrefix(line, "{") {
			raw = line
		} else {
			continue
		}

		ev := parseGrokEvent(json.RawMessage(raw))
		if isEmptyEvent(ev) {
			continue
		}
		ev.Downloader = downloader
		ch <- ev
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: %v", upstream.ErrNetwork, err)
	}
	return nil
}

func parseGrokEvent(raw json.RawMessage) upstream.StreamEvent {
	var result chatStreamPayload
	if err := json.Unmarshal(raw, &result); err != nil {
		return upstream.StreamEvent{Error: err}
	}

	resp := result.Result.Response
	content, reasoning := splitThinkingToken(resp.Token, resp.IsThinking)
	content = appendModelResponseImages(content, resp.ModelResponse)
	content = appendCardAttachmentImage(content, resp.CardAttachment)

	return upstream.StreamEvent{
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

func collectSearchSources(resp chatStreamResponse) []upstream.SearchSource {
	sources := appendWebSearchSources(nil, resp.WebSearchResults)
	return appendXSearchSources(sources, resp.XSearchResults)
}

func appendWebSearchSources(sources []upstream.SearchSource, wsr *chatWebSearchResults) []upstream.SearchSource {
	if wsr == nil {
		return sources
	}
	for _, item := range wsr.Results {
		if item.URL != "" {
			sources = append(sources, upstream.SearchSource{URL: item.URL, Title: item.Title, Type: "web"})
		}
	}
	return sources
}

func appendXSearchSources(sources []upstream.SearchSource, xsr *chatXSearchResults) []upstream.SearchSource {
	if xsr == nil {
		return sources
	}
	for _, item := range xsr.Results {
		if item.PostID != "" && item.Username != "" {
			sources = append(sources, upstream.SearchSource{
				URL:   fmt.Sprintf("https://x.com/%s/status/%s", item.Username, item.PostID),
				Title: normalizeXTitle(item.Username, item.Text),
				Type:  "x_post",
			})
		}
	}
	return sources
}

func normalizeXTitle(username, text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "𝕏/@" + username
	}
	runes := []rune(text)
	if len(runes) > 50 {
		return string(runes[:50]) + "…"
	}
	return text
}

func isEmptyEvent(ev upstream.StreamEvent) bool {
	return ev.Content == "" && ev.ReasoningContent == "" && ev.FinishReason == nil &&
		ev.Usage == nil && len(ev.ToolCalls) == 0 && ev.Error == nil &&
		!ev.IsThinking && ev.RolloutID == "" && len(ev.SearchSources) == 0
}
