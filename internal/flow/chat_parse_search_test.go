package flow

import (
	"encoding/json"
	"testing"

	"github.com/crmmc/grokforge/internal/xai"
)

func TestParseEvent_WebSearchResults(t *testing.T) {
	f := &ChatFlow{}
	data := json.RawMessage(`{
		"result": {
			"response": {
				"token": "hello",
				"isThinking": false,
				"webSearchResults": {
					"results": [
						{"url": "https://example.com/a", "title": "Article A"},
						{"url": "https://example.com/b", "title": "Article B"}
					]
				}
			}
		}
	}`)

	event := f.parseEvent(xai.StreamEvent{Data: data})
	if event.Error != nil {
		t.Fatalf("unexpected error: %v", event.Error)
	}
	if event.Content != "hello" {
		t.Errorf("content = %q, want %q", event.Content, "hello")
	}
	if len(event.SearchSources) != 2 {
		t.Fatalf("search sources count = %d, want 2", len(event.SearchSources))
	}
	if event.SearchSources[0].URL != "https://example.com/a" {
		t.Errorf("source[0].URL = %q, want %q", event.SearchSources[0].URL, "https://example.com/a")
	}
	if event.SearchSources[0].Title != "Article A" {
		t.Errorf("source[0].Title = %q, want %q", event.SearchSources[0].Title, "Article A")
	}
	if event.SearchSources[0].Type != "web" {
		t.Errorf("source[0].Type = %q, want %q", event.SearchSources[0].Type, "web")
	}
	if event.SearchSources[1].URL != "https://example.com/b" {
		t.Errorf("source[1].URL = %q, want %q", event.SearchSources[1].URL, "https://example.com/b")
	}
}

func TestParseEvent_XSearchResults(t *testing.T) {
	f := &ChatFlow{}
	data := json.RawMessage(`{
		"result": {
			"response": {
				"token": "",
				"isThinking": false,
				"xSearchResults": {
					"results": [
						{"postId": "123456", "username": "elonmusk", "text": "Just announced something big"},
						{"postId": "789012", "username": "xai", "text": ""}
					]
				}
			}
		}
	}`)

	event := f.parseEvent(xai.StreamEvent{Data: data})
	if event.Error != nil {
		t.Fatalf("unexpected error: %v", event.Error)
	}
	if len(event.SearchSources) != 2 {
		t.Fatalf("search sources count = %d, want 2", len(event.SearchSources))
	}
	src0 := event.SearchSources[0]
	if src0.URL != "https://x.com/elonmusk/status/123456" {
		t.Errorf("source[0].URL = %q, want x.com URL", src0.URL)
	}
	if src0.Title != "Just announced something big" {
		t.Errorf("source[0].Title = %q, want text content", src0.Title)
	}
	if src0.Type != "x_post" {
		t.Errorf("source[0].Type = %q, want %q", src0.Type, "x_post")
	}
	// Empty text falls back to 𝕏/@username
	src1 := event.SearchSources[1]
	if src1.Title != "𝕏/@xai" {
		t.Errorf("source[1].Title = %q, want %q", src1.Title, "𝕏/@xai")
	}
}

func TestParseEvent_MixedSearchResults(t *testing.T) {
	f := &ChatFlow{}
	data := json.RawMessage(`{
		"result": {
			"response": {
				"token": "news",
				"isThinking": false,
				"webSearchResults": {
					"results": [
						{"url": "https://news.com/1", "title": "News 1"}
					]
				},
				"xSearchResults": {
					"results": [
						{"postId": "111", "username": "user1", "text": "Tweet text"}
					]
				}
			}
		}
	}`)

	event := f.parseEvent(xai.StreamEvent{Data: data})
	if event.Error != nil {
		t.Fatalf("unexpected error: %v", event.Error)
	}
	if len(event.SearchSources) != 2 {
		t.Fatalf("search sources count = %d, want 2", len(event.SearchSources))
	}
	if event.SearchSources[0].Type != "web" {
		t.Errorf("source[0].Type = %q, want %q", event.SearchSources[0].Type, "web")
	}
	if event.SearchSources[1].Type != "x_post" {
		t.Errorf("source[1].Type = %q, want %q", event.SearchSources[1].Type, "x_post")
	}
}

func TestParseEvent_NoSearchResults(t *testing.T) {
	f := &ChatFlow{}
	data := json.RawMessage(`{
		"result": {
			"response": {
				"token": "no search",
				"isThinking": false
			}
		}
	}`)

	event := f.parseEvent(xai.StreamEvent{Data: data})
	if event.Error != nil {
		t.Fatalf("unexpected error: %v", event.Error)
	}
	if len(event.SearchSources) != 0 {
		t.Errorf("search sources count = %d, want 0", len(event.SearchSources))
	}
}

func TestParseEvent_EmptySearchResults(t *testing.T) {
	f := &ChatFlow{}
	data := json.RawMessage(`{
		"result": {
			"response": {
				"token": "empty",
				"isThinking": false,
				"webSearchResults": {"results": []},
				"xSearchResults": {"results": []}
			}
		}
	}`)

	event := f.parseEvent(xai.StreamEvent{Data: data})
	if event.Error != nil {
		t.Fatalf("unexpected error: %v", event.Error)
	}
	if len(event.SearchSources) != 0 {
		t.Errorf("search sources count = %d, want 0", len(event.SearchSources))
	}
}

func TestParseEvent_SkipsEmptyURLAndMissingFields(t *testing.T) {
	f := &ChatFlow{}
	data := json.RawMessage(`{
		"result": {
			"response": {
				"token": "",
				"isThinking": false,
				"webSearchResults": {
					"results": [
						{"url": "", "title": "No URL"},
						{"url": "https://valid.com", "title": "Valid"}
					]
				},
				"xSearchResults": {
					"results": [
						{"postId": "", "username": "user1", "text": "missing postId"},
						{"postId": "123", "username": "", "text": "missing username"},
						{"postId": "456", "username": "user2", "text": "valid"}
					]
				}
			}
		}
	}`)

	event := f.parseEvent(xai.StreamEvent{Data: data})
	if event.Error != nil {
		t.Fatalf("unexpected error: %v", event.Error)
	}
	// 1 valid web + 1 valid x_post
	if len(event.SearchSources) != 2 {
		t.Fatalf("search sources count = %d, want 2", len(event.SearchSources))
	}
	if event.SearchSources[0].URL != "https://valid.com" {
		t.Errorf("source[0].URL = %q, want valid web URL", event.SearchSources[0].URL)
	}
	if event.SearchSources[1].URL != "https://x.com/user2/status/456" {
		t.Errorf("source[1].URL = %q, want valid x URL", event.SearchSources[1].URL)
	}
}

func TestNormalizeXTitle(t *testing.T) {
	tests := []struct {
		name     string
		username string
		text     string
		want     string
	}{
		{
			name:     "normal text",
			username: "user1",
			text:     "Hello world",
			want:     "Hello world",
		},
		{
			name:     "empty text fallback",
			username: "elonmusk",
			text:     "",
			want:     "𝕏/@elonmusk",
		},
		{
			name:     "whitespace only fallback",
			username: "user2",
			text:     "   \t\n  ",
			want:     "𝕏/@user2",
		},
		{
			name:     "truncate at 50 runes",
			username: "user3",
			text:     "这是一段超过五十个字符的中文文本，用来测试截断功能是否正常工作，应该在第五十个字符处截断并添加省略号的效果",
			want:     "这是一段超过五十个字符的中文文本，用来测试截断功能是否正常工作，应该在第五十个字符处截断并添加省略号…",
		},
		{
			name:     "exactly 50 runes no truncation",
			username: "user4",
			text:     "12345678901234567890123456789012345678901234567890",
			want:     "12345678901234567890123456789012345678901234567890",
		},
		{
			name:     "normalize whitespace",
			username: "user5",
			text:     "hello   world\n\tnewline",
			want:     "hello world newline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeXTitle(tt.username, tt.text)
			if got != tt.want {
				t.Errorf("normalizeXTitle(%q, %q) = %q, want %q", tt.username, tt.text, got, tt.want)
			}
		})
	}
}
