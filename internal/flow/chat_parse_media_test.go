package flow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/xai"
)

func TestChatFlow_ParseEvent_CardAttachmentImageChunk(t *testing.T) {
	cardJSON := `{"image_chunk":{"progress":100,"imageUuid":"image-1","imageUrl":"users/u/generated/id/image.png"}}`
	event := cardAttachmentEvent(cardJSON)

	got := (&ChatFlow{}).parseEvent(event)
	want := "![image-1](https://assets.grok.com/users/u/generated/id/image.png)"
	if !strings.Contains(got.Content, want) {
		t.Fatalf("Content = %q, want markdown image %q", got.Content, want)
	}
}

func TestChatFlow_ParseEvent_CardAttachmentModeratedImageChunk(t *testing.T) {
	cardJSON := `{"image_chunk":{"progress":100,"imageUuid":"image-1","imageUrl":"users/u/generated/id/image.png","moderated":true}}`
	event := cardAttachmentEvent(cardJSON)

	got := (&ChatFlow{}).parseEvent(event)
	if got.Content != "" {
		t.Fatalf("Content = %q, want empty for moderated image chunk", got.Content)
	}
}

func TestChatFlow_ParseEvent_CardAttachmentRelativeImageOriginal(t *testing.T) {
	cardJSON := `{"image":{"original":"cards/id/image.png","title":"card"}}`
	event := cardAttachmentEvent(cardJSON)

	got := (&ChatFlow{}).parseEvent(event)
	want := "![card](https://assets.grok.com/cards/id/image.png)"
	if !strings.Contains(got.Content, want) {
		t.Fatalf("Content = %q, want markdown image %q", got.Content, want)
	}
}

func TestChatFlow_ParseEvent_CardAttachmentSchemeRelativeImageOriginal(t *testing.T) {
	cardJSON := `{"image":{"original":"//assets.grok.com/cards/id/image.png","title":"card"}}`
	event := cardAttachmentEvent(cardJSON)

	got := (&ChatFlow{}).parseEvent(event)
	want := "![card](https://assets.grok.com/cards/id/image.png)"
	if !strings.Contains(got.Content, want) {
		t.Fatalf("Content = %q, want markdown image %q", got.Content, want)
	}
}

func TestChatFlow_ParseEvent_CardAttachmentUppercaseSchemeImageOriginal(t *testing.T) {
	cardJSON := `{"image":{"original":"HTTPS://assets.grok.com/cards/id/image.png","title":"card"}}`
	event := cardAttachmentEvent(cardJSON)

	got := (&ChatFlow{}).parseEvent(event)
	want := "![card](HTTPS://assets.grok.com/cards/id/image.png)"
	if !strings.Contains(got.Content, want) {
		t.Fatalf("Content = %q, want markdown image %q", got.Content, want)
	}
}

func cardAttachmentEvent(cardJSON string) xai.StreamEvent {
	data := fmt.Sprintf(
		`{"result":{"response":{"token":"","isThinking":false,"cardAttachment":{"jsonData":%s}}}}`,
		strconv.Quote(cardJSON),
	)
	return xai.StreamEvent{Data: json.RawMessage(data)}
}
