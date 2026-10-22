package flow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowedToolNamesTable(t *testing.T) {
	assert.Nil(t, allowedToolNames(nil))
	assert.Nil(t, allowedToolNames([]Tool{}))

	named := allowedToolNames([]Tool{
		{Function: Function{Name: "a"}},
		{Function: Function{Name: ""}},
		{Function: Function{Name: "b"}},
	})
	require.NotNil(t, named)
	_, ok := named["a"]
	assert.True(t, ok)
	_, ok = named["b"]
	assert.True(t, ok)
	assert.Len(t, named, 2)
}

func TestParseToolCallPayloadTable(t *testing.T) {
	allowed := map[string]struct{}{"f": {}}
	idx := 3
	tests := []struct {
		name  string
		raw   string
		valid map[string]struct{}
		index *int
		want  *ToolCall
	}{
		{"empty payload", "   ", nil, nil, nil},
		{"invalid json", "not-json", nil, nil, nil},
		{"missing name", `{"arguments":"{}"}`, nil, nil, nil},
		{"not in allow list", `{"name":"x","arguments":"{}"}`, allowed, nil, nil},
		{"allowed with index", `{"name":"f","arguments":"{}"}`, allowed, &idx, &ToolCall{Type: "function", Function: FunctionCall{Name: "f", Arguments: "{}"}}},
		{"allowed without index", `{"name":"f","arguments":{"k":1}}`, allowed, nil, &ToolCall{Type: "function", Function: FunctionCall{Name: "f", Arguments: `{"k":1}`}}},
		{"no allow list accepts any", `{"name":"any","arguments":"{}"}`, nil, nil, &ToolCall{Type: "function", Function: FunctionCall{Name: "any", Arguments: "{}"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseToolCallPayload(tt.raw, tt.valid, tt.index)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want.Type, got.Type)
			assert.Equal(t, tt.want.Function, got.Function)
			if tt.index != nil {
				require.NotNil(t, got.Index)
				assert.Equal(t, *tt.index, *got.Index)
			} else {
				assert.Nil(t, got.Index)
			}
			assert.NotEmpty(t, got.ID)
		})
	}
}

func TestParseToolCallBlockSetsIndex(t *testing.T) {
	tools := []Tool{{Type: "function", Function: Function{Name: "f"}}}
	call := ParseToolCallBlock(`{"name":"f","arguments":"{}"}`, tools, 2)
	require.NotNil(t, call)
	require.NotNil(t, call.Index)
	assert.Equal(t, 2, *call.Index)

	assert.Nil(t, ParseToolCallBlock(`{"name":"other","arguments":"{}"}`, tools, 0))
	assert.Nil(t, ParseToolCallBlock("", tools, 0))
}

func TestNormalizeArgumentsTable(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"empty", nil, "{}"},
		{"double encoded string", json.RawMessage(`"{\"a\": 1,}"`), `{"a": 1}`},
		{"object passthrough", json.RawMessage(`{"a":1}`), `{"a":1}`},
		{"plain string content", json.RawMessage(`"hello"`), "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeArguments(tt.raw))
		})
	}
}

func TestRepairJSONCRLFAndEmpty(t *testing.T) {
	assert.Empty(t, repairJSON("   "))
	assert.Equal(t, "{\n\"a\":1\n}", repairJSON("{\r\n\"a\":1\r\n}"))
	assert.Equal(t, "{\"a\":1\n}", repairJSON("{\"a\":1\r}"))
}

func TestStreamToolCallParserFlushStates(t *testing.T) {
	t.Run("text state flush returns partial carry", func(t *testing.T) {
		p := newStreamToolCallParser(nil)
		text, calls := p.Push("abc<tool_")
		assert.Equal(t, "abc", text)
		assert.Empty(t, calls)

		text, calls = p.Flush()
		assert.Equal(t, "<tool_", text)
		assert.Empty(t, calls)

		// Flush is empty on the second call.
		text, calls = p.Flush()
		assert.Empty(t, text)
		assert.Empty(t, calls)
	})

	t.Run("tool state flush parses buffered payload", func(t *testing.T) {
		tools := []Tool{{Type: "function", Function: Function{Name: "f"}}}
		p := newStreamToolCallParser(tools)
		text, calls := p.Push(`<tool_call>{"name":"f","arguments":{}}`)
		assert.Empty(t, text)
		assert.Empty(t, calls)

		text, calls = p.Flush()
		assert.Empty(t, text)
		require.Len(t, calls, 1)
		assert.Equal(t, "f", calls[0].Function.Name)
	})

	t.Run("tool state flush returns raw for unparsable payload", func(t *testing.T) {
		p := newStreamToolCallParser(nil)
		p.Push(`<tool_call>garbage-no-end`)
		text, calls := p.Flush()
		assert.Equal(t, "<tool_call>garbage-no-end", text)
		assert.Empty(t, calls)
	})
}

func TestStreamToolCallParserInvalidBlockBecomesText(t *testing.T) {
	p := newStreamToolCallParser(nil)
	text, calls := p.Push("x<tool_call>bad</tool_call>y")
	assert.Empty(t, calls)
	assert.Equal(t, "x<tool_call>bad</tool_call>y", text)
}

func TestStreamToolCallParserIndexesIncrement(t *testing.T) {
	tools := []Tool{{Type: "function", Function: Function{Name: "f"}}}
	p := newStreamToolCallParser(tools)
	_, calls := p.Push(`<tool_call>{"name":"f","arguments":"{}"}</tool_call>mid<tool_call>{"name":"f","arguments":"{}"}</tool_call>`)
	require.Len(t, calls, 2)
	require.NotNil(t, calls[0].Index)
	require.NotNil(t, calls[1].Index)
	assert.Equal(t, 0, *calls[0].Index)
	assert.Equal(t, 1, *calls[1].Index)
}

func TestDropTagParserLifecycle(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		chunks   []string
		wantOut  string
		wantBack string // expected text returned by Flush after the chunks
	}{
		{
			name:    "complete tag dropped",
			tag:     "xaiartifact",
			chunks:  []string{"a<xaiartifact>hide</xaiartifact>b"},
			wantOut: "ab",
		},
		{
			name:     "unterminated tag swallows rest and flush empties",
			tag:      "xaiartifact",
			chunks:   []string{"a<xaiartifact>hide"},
			wantOut:  "a",
			wantBack: "",
		},
		{
			name:     "partial tag carry flushed back",
			tag:      "xaiartifact",
			chunks:   []string{"ab<xaiart"},
			wantOut:  "ab",
			wantBack: "<xaiart",
		},
		{
			name:     "open tag without gt carried",
			tag:      "xaiartifact",
			chunks:   []string{"a<xaiartifact"},
			wantOut:  "a",
			wantBack: "<xaiartifact",
		},
		{
			name:    "self closing tag removed",
			tag:     "br",
			chunks:  []string{"a<br/>b"},
			wantOut: "ab",
		},
		{
			name:    "end tag split across chunks",
			tag:     "xaiartifact",
			chunks:  []string{"<xaiartifact>a", "b</xaiart", "ifact>c"},
			wantOut: "c",
		},
		{
			name:    "empty chunk is noop",
			tag:     "xaiartifact",
			chunks:  []string{"a", "", "b"},
			wantOut: "ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newDropTagParser(tt.tag)
			var out strings.Builder
			for _, chunk := range tt.chunks {
				out.WriteString(p.Consume(chunk))
			}
			assert.Equal(t, tt.wantOut, out.String())
			assert.Equal(t, tt.wantBack, p.Flush())
		})
	}
}

func TestStreamTokenFilterFlushWithCard(t *testing.T) {
	filter := newStreamTokenFilter([]string{"xai:tool_usage_card", "xaiartifact"})
	got := filter.Apply(StreamEvent{
		Content:   "pre <xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name><xai:tool_args>{\"query\":\"q\"}</xai:tool_args></xai:tool_usage_card> post",
		RolloutID: "r1",
	})
	assert.Contains(t, got.Content, "[r1][WebSearch] q")
	assert.Contains(t, got.Content, "pre ")

	// An incomplete card is discarded on flush: nothing leaks out.
	cardFilter := newStreamTokenFilter([]string{"xai:tool_usage_card"})
	assert.Empty(t, cardFilter.Apply(StreamEvent{Content: "<xai:tool_usage_card><xai:tool_name>web_se", RolloutID: "r1"}).Content)
	assert.Empty(t, cardFilter.Flush("r1"))

	// Partial drop-tag carry is preserved by flush.
	tagFilter := newStreamTokenFilter([]string{"xaiartifact"})
	assert.Equal(t, "tail ", tagFilter.Apply(StreamEvent{Content: "tail <xaiart"}).Content)
	assert.Equal(t, "<xaiart", tagFilter.Flush(""))
}

func TestNewStreamTokenFilterEmptyTagSkipped(t *testing.T) {
	filter := newStreamTokenFilter([]string{"", "  ", "xaiartifact"})
	require.Len(t, filter.dropParsers, 1)
	assert.Nil(t, filter.toolCardPsr)
}

func TestToolUsageCardParserFlushStates(t *testing.T) {
	t.Run("in-card flush discards buffer", func(t *testing.T) {
		p := &toolUsageCardParser{}
		assert.Empty(t, p.Consume("<xai:tool_usage_card><xai:tool_name>x", "r"))
		assert.Empty(t, p.Flush("r"))
		assert.Equal(t, "junk-after-reset", p.Consume("junk-after-reset", "r"))
	})

	t.Run("plain flush returns partial carry", func(t *testing.T) {
		p := &toolUsageCardParser{}
		assert.Equal(t, "abc", p.Consume("abc<xai:tool_", ""))
		assert.Equal(t, "<xai:tool_", p.Flush(""))
	})

	t.Run("flush with rollout id and no state", func(t *testing.T) {
		p := &toolUsageCardParser{}
		assert.Empty(t, p.Flush("  rid  "))
	})
}

func TestToolUsageCardParserEmitSkipsEmptyLine(t *testing.T) {
	p := &toolUsageCardParser{}
	got := p.Consume("a<xai:tool_usage_card></xai:tool_usage_card>b", "")
	assert.Equal(t, "ab", got)
}

func TestParseToolUsageLineTable(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		rolloutID string
		want      string
	}{
		{
			"web search with query",
			"<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name><xai:tool_args>{\"query\":\"news\"}</xai:tool_args></xai:tool_usage_card>",
			"r1",
			"[r1][WebSearch] news",
		},
		{
			"web search falls back to raw args",
			"<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name><xai:tool_args>not-json</xai:tool_args></xai:tool_usage_card>",
			"",
			"[WebSearch] not-json",
		},
		{
			"search image uses description",
			"<xai:tool_usage_card><xai:tool_name>search_images</xai:tool_name><xai:tool_args>{\"image_description\":\"cats\"}</xai:tool_args></xai:tool_usage_card>",
			"r2",
			"[r2][SearchImage] cats",
		},
		{
			"chatroom send uses message",
			"<xai:tool_usage_card><xai:tool_name>chatroom_send</xai:tool_name><xai:tool_args>{\"message\":\"hmm\"}</xai:tool_args></xai:tool_usage_card>",
			"",
			"[AgentThink] hmm",
		},
		{
			"unknown tool keeps name and args",
			"<xai:tool_usage_card><xai:tool_name>code_exec</xai:tool_name><xai:tool_args>{\"code\":\"1+1\"}</xai:tool_args></xai:tool_usage_card>",
			"",
			"code_exec {\"code\":\"1+1\"}",
		},
		{
			"name only",
			"<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name></xai:tool_usage_card>",
			"",
			"[WebSearch]",
		},
		{
			"no recognized tags strips markup",
			"<xai:tool_usage_card>plain text</xai:tool_usage_card>",
			"",
			"plain text",
		},
		{
			"cdata args unwrapped",
			"<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name><xai:tool_args><![CDATA[{\"query\":\"q\"}]]></xai:tool_args></xai:tool_usage_card>",
			"",
			"[WebSearch] q",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseToolUsageLine(tt.raw, tt.rolloutID))
		})
	}
}

func TestExtractTagValueTable(t *testing.T) {
	assert.Equal(t, "", extractTagValue("no tags", "x"))
	assert.Equal(t, "", extractTagValue("<x>value-without-end", "x"))
	assert.Equal(t, "v", extractTagValue("a<x> v </x>b", "x"))
	assert.Equal(t, "wrapped", extractTagValue("<x><![CDATA[wrapped]]></x>", "x"))
}

func TestStripCDATATable(t *testing.T) {
	assert.Empty(t, stripCDATA(""))
	assert.Equal(t, "x", stripCDATA(" <![CDATA[x]]> "))
	assert.Equal(t, "plain", stripCDATA(" plain "))
	assert.Equal(t, "", stripCDATA("<![CDATA[]]>"))
}

func TestParseToolArgsTable(t *testing.T) {
	assert.Nil(t, parseToolArgs("   "))
	assert.Nil(t, parseToolArgs("{bad"))
	got := parseToolArgs(`{"k":"v"}`)
	require.NotNil(t, got)
	assert.Equal(t, "v", got["k"])
}

func TestFirstNonEmptyTextTable(t *testing.T) {
	assert.Empty(t, firstNonEmptyText(nil, "a"))
	assert.Empty(t, firstNonEmptyText(map[string]any{"a": "   "}, "a"))
	assert.Equal(t, "v", firstNonEmptyText(map[string]any{"a": " ", "b": "v"}, "a", "b"))
	assert.Empty(t, firstNonEmptyText(map[string]any{"a": 3}, "a"))
}

func TestSplitByTagPrefixTable(t *testing.T) {
	text, carry := SplitByTagPrefix("", "<tag")
	assert.Empty(t, text)
	assert.Empty(t, carry)

	text, carry = SplitByTagPrefix("hello", "<tool_call>")
	assert.Equal(t, "hello", text)
	assert.Empty(t, carry)

	text, carry = SplitByTagPrefix("abc<tool_", "<tool_call>")
	assert.Equal(t, "abc", text)
	assert.Equal(t, "<tool_", carry)
}

func TestSuffixPrefixLengthTable(t *testing.T) {
	assert.Equal(t, 0, SuffixPrefixLength("abc", "x"))
	assert.Equal(t, 0, SuffixPrefixLength("abc", "<tool_call>"))
	assert.Equal(t, 1, SuffixPrefixLength("abc<", "<tool_call>"))
	assert.Equal(t, 4, SuffixPrefixLength("ab<too", "<tool_call>"))
	assert.Equal(t, 2, SuffixPrefixLength("ab", "abcd"))
}

func TestIsServerErrorTable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no status", errors.New("boom"), false},
		{"4xx", errors.New("404 missing"), false},
		{"500", errors.New("500 internal"), true},
		{"599", errors.New("599 weird"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isServerError(tt.err))
		})
	}
}

func TestReportTrackedTokenErrorMatrix(t *testing.T) {
	newSvc := func() *mockTokenService { return &mockTokenService{} }

	t.Run("nil service and nil error are no-ops", func(t *testing.T) {
		reportTrackedTokenError(nil, 1, "auto", errors.New("x"))
		svc := newSvc()
		reportTrackedTokenError(svc, 1, "auto", nil)
		assert.Empty(t, svc.errorCalls)
		assert.Empty(t, svc.releaseCalls)
	})

	t.Run("xai invalid token marks expired", func(t *testing.T) {
		svc := newSvc()
		reportTrackedTokenError(svc, 1, "auto", xai.ErrInvalidToken)
		assert.Equal(t, []uint{1}, svc.expiredCalls)
	})

	t.Run("upstream invalid token marks expired", func(t *testing.T) {
		svc := newSvc()
		reportTrackedTokenError(svc, 1, "auto", upstream.ErrInvalidToken)
		assert.Equal(t, []uint{1}, svc.expiredCalls)
	})

	t.Run("forbidden and cf challenge only release", func(t *testing.T) {
		for _, err := range []error{xai.ErrForbidden, xai.ErrCFChallenge, upstream.ErrForbidden, upstream.ErrCFChallenge} {
			svc := newSvc()
			reportTrackedTokenError(svc, 1, "auto", err)
			assert.Equal(t, []uint{1}, svc.releaseCalls)
			assert.Empty(t, svc.errorCalls)
			assert.Empty(t, svc.rateLimitCalls)
		}
	})

	t.Run("quota level reports rate limit", func(t *testing.T) {
		svc := newSvc()
		reportTrackedTokenError(svc, 1, "auto", xai.ErrRateLimited)
		assert.Equal(t, []uint{1}, svc.rateLimitCalls)
	})

	t.Run("transport error is recoverable", func(t *testing.T) {
		svc := newSvc()
		reportTrackedTokenError(svc, 1, "auto", errors.New("500 boom"))
		assert.Equal(t, []uint{1}, svc.errorCalls)
	})

	t.Run("plain error is not recoverable", func(t *testing.T) {
		svc := newSvc()
		reportTrackedTokenError(svc, 1, "auto", errors.New("nope"))
		assert.Equal(t, []uint{1}, svc.errorCalls)
	})

	t.Run("reason truncated", func(t *testing.T) {
		svc := &capturingReasonService{}
		reportTrackedTokenError(svc, 1, "auto", errors.New(strings.Repeat("y", 300)))
		assert.Len(t, svc.reason, 256)
		assert.Equal(t, false, svc.recoverable)
	})
}

// capturingReasonService records the reason passed to ReportError.
type capturingReasonService struct {
	mockTokenService
	reason      string
	recoverable bool
}

func (c *capturingReasonService) ReportError(id uint, mode string, recoverable bool, reason string) {
	c.reason = reason
	c.recoverable = recoverable
}

var _ TokenServicer = (*capturingReasonService)(nil)

func TestToolUsageCardParserEmptyChunk(t *testing.T) {
	p := &toolUsageCardParser{}
	assert.Empty(t, p.Consume("", "r"))
}

func TestToolUsageCardParserCardContentAcrossChunks(t *testing.T) {
	t.Run("end tag arrives in second chunk", func(t *testing.T) {
		p := &toolUsageCardParser{}
		first := p.Consume("<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name>", "r1")
		assert.Empty(t, first)
		second := p.Consume("<xai:tool_args>{\"query\":\"q\"}</xai:tool_args></xai:tool_usage_card>tail", "")
		assert.Contains(t, second, "[r1][WebSearch] q")
		assert.Contains(t, second, "tail")
	})

	t.Run("end tag still pending keeps buffering", func(t *testing.T) {
		p := &toolUsageCardParser{}
		assert.Empty(t, p.Consume("<xai:tool_usage_card><xai:tool_name>web", "r1"))
		// No end tag yet: buffered content must not leak into the output.
		assert.Empty(t, p.Consume("_search more partial", ""))
		assert.Empty(t, p.Flush("r1"))
	})
}

func TestParseToolUsageLineFallbacks(t *testing.T) {
	t.Run("search images falls back to raw args", func(t *testing.T) {
		raw := "<xai:tool_usage_card><xai:tool_name>search_images</xai:tool_name><xai:tool_args>not-json</xai:tool_args></xai:tool_usage_card>"
		assert.Equal(t, "[SearchImage] not-json", parseToolUsageLine(raw, ""))
	})

	t.Run("chatroom send falls back to raw args", func(t *testing.T) {
		raw := "<xai:tool_usage_card><xai:tool_name>chatroom_send</xai:tool_name><xai:tool_args>not-json</xai:tool_args></xai:tool_usage_card>"
		assert.Equal(t, "[AgentThink] not-json", parseToolUsageLine(raw, ""))
	})

	t.Run("web search uses short q key", func(t *testing.T) {
		raw := "<xai:tool_usage_card><xai:tool_name>web_search</xai:tool_name><xai:tool_args>{\"q\":\"news\"}</xai:tool_args></xai:tool_usage_card>"
		assert.Equal(t, "[WebSearch] news", parseToolUsageLine(raw, ""))
	})

	t.Run("args text without name", func(t *testing.T) {
		raw := "<xai:tool_usage_card><xai:tool_args>{\"query\":\"q\"}</xai:tool_args></xai:tool_usage_card>"
		assert.Equal(t, "{\"query\":\"q\"}", parseToolUsageLine(raw, ""))
	})
}

func TestStreamToolCallParserFlushEmptyToolBuffer(t *testing.T) {
	p := newStreamToolCallParser(nil)
	text, calls := p.Push("<tool_call>")
	assert.Empty(t, text)
	assert.Empty(t, calls)

	text, calls = p.Flush()
	assert.Empty(t, text)
	assert.Empty(t, calls)
}
