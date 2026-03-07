package openai

import (
	"strings"

	"github.com/crmmc/grokforge/internal/flow"
)

const (
	toolCallStartTag = "<tool_call>"
	toolCallEndTag   = "</tool_call>"
)

type toolCallStreamParser struct {
	tools      []flow.Tool
	state      string
	partial    string
	toolBuffer strings.Builder
	nextIndex  int
}

func newToolCallStreamParser(tools []flow.Tool) *toolCallStreamParser {
	return &toolCallStreamParser{
		tools: tools,
		state: "text",
	}
}

func (p *toolCallStreamParser) Push(chunk string) ([]string, []flow.ToolCall) {
	if chunk == "" {
		return nil, nil
	}
	data := p.partial + chunk
	p.partial = ""

	var texts []string
	var calls []flow.ToolCall

	for data != "" {
		if p.state == "text" {
			data, texts = p.consumeTextState(data, texts)
			continue
		}

		data, texts, calls = p.consumeToolState(data, texts, calls)
	}

	return texts, calls
}

func (p *toolCallStreamParser) Flush() ([]string, []flow.ToolCall) {
	if p.state == "text" {
		if p.partial == "" {
			return nil, nil
		}
		text := p.partial
		p.partial = ""
		return []string{text}, nil
	}

	raw := p.toolBuffer.String() + p.partial
	p.toolBuffer.Reset()
	p.partial = ""
	p.state = "text"

	if call := flow.ParseToolCallBlock(raw, p.tools, p.nextIndex); call != nil {
		p.nextIndex++
		return nil, []flow.ToolCall{*call}
	}
	if raw != "" {
		return []string{toolCallStartTag + raw}, nil
	}
	return nil, nil
}

func (p *toolCallStreamParser) consumeTextState(data string, texts []string) (string, []string) {
	start := strings.Index(data, toolCallStartTag)
	if start < 0 {
		text, carry := flow.SplitByTagPrefix(data, toolCallStartTag)
		if text != "" {
			texts = append(texts, text)
		}
		p.partial = carry
		return "", texts
	}
	if start > 0 {
		texts = append(texts, data[:start])
	}
	p.state = "tool"
	return data[start+len(toolCallStartTag):], texts
}

func (p *toolCallStreamParser) consumeToolState(data string, texts []string, calls []flow.ToolCall) (string, []string, []flow.ToolCall) {
	end := strings.Index(data, toolCallEndTag)
	if end < 0 {
		chunkText, carry := flow.SplitByTagPrefix(data, toolCallEndTag)
		if chunkText != "" {
			p.toolBuffer.WriteString(chunkText)
		}
		p.partial = carry
		return "", texts, calls
	}
	p.toolBuffer.WriteString(data[:end])
	rest := data[end+len(toolCallEndTag):]
	raw := p.toolBuffer.String()
	if call := flow.ParseToolCallBlock(raw, p.tools, p.nextIndex); call != nil {
		calls = append(calls, *call)
		p.nextIndex++
	} else if strings.TrimSpace(raw) != "" {
		texts = append(texts, toolCallStartTag+raw+toolCallEndTag)
	}
	p.toolBuffer.Reset()
	p.state = "text"
	return rest, texts, calls
}
