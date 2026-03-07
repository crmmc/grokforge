package flow

import (
	"strings"
	"testing"
)

func TestToolUsageCardParser_SingleChunk(t *testing.T) {
	parser := &toolUsageCardParser{}
	chunk := `before<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name><xai:tool_args>{"query":"latest news"}</xai:tool_args></xai:tool_usage_card>after`

	got := parser.Consume(chunk, "rollout-1")
	if !strings.Contains(got, "[rollout-1][WebSearch] latest news") {
		t.Fatalf("expected converted tool usage line, got %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("expected surrounding text to be preserved, got %q", got)
	}
}

func TestToolUsageCardParser_PartialChunks(t *testing.T) {
	parser := &toolUsageCardParser{}
	chunk1 := `hello <xai:tool_usage_card><xai:tool_name>chatroom_send</xai:tool_name>`
	chunk2 := `<xai:tool_args>{"message":"thinking"}</xai:tool_args></xai:tool_usage_card> world`

	got1 := parser.Consume(chunk1, "rid-2")
	if strings.Contains(got1, "AgentThink") {
		t.Fatalf("should not emit tool line before closing tag, got %q", got1)
	}

	got2 := parser.Consume(chunk2, "")
	combined := got1 + got2
	if !strings.Contains(combined, "[rid-2][AgentThink] thinking") {
		t.Fatalf("expected merged chunks to produce tool line, got %q", combined)
	}
	if !strings.Contains(combined, "hello") || !strings.Contains(combined, "world") {
		t.Fatalf("expected plain text around card to remain, got %q", combined)
	}
}

func TestStreamTokenFilter_Apply(t *testing.T) {
	filter := newStreamTokenFilter([]string{"xaiartifact", "xai:tool_usage_card"})

	blocked := filter.Apply(StreamEvent{
		Content: `<xaiartifact>blocked</xaiartifact>`,
	})
	if blocked.Content != "" {
		t.Fatalf("expected xaiartifact content to be dropped, got %q", blocked.Content)
	}

	converted := filter.Apply(StreamEvent{
		Content:   `<xai:tool_usage_card><xai:tool_name>search_images</xai:tool_name><xai:tool_args>{"description":"red shoes"}</xai:tool_args></xai:tool_usage_card>`,
		RolloutID: "rid-3",
	})
	if !strings.Contains(converted.Content, "[rid-3][SearchImage] red shoes") {
		t.Fatalf("expected tool card conversion output, got %q", converted.Content)
	}
}
