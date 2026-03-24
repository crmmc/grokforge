package flow

import "strings"

const (
	streamToolStateText = "text"
	streamToolStateCall = "tool"
)

type streamToolCallParser struct {
	tools      []Tool
	state      string
	partial    string
	toolBuffer strings.Builder
	nextIndex  int
}

func newStreamToolCallParser(tools []Tool) *streamToolCallParser {
	return &streamToolCallParser{
		tools: tools,
		state: streamToolStateText,
	}
}

func (p *streamToolCallParser) Push(chunk string) (string, []ToolCall) {
	if chunk == "" {
		return "", nil
	}
	data := p.partial + chunk
	p.partial = ""

	var texts []string
	var calls []ToolCall
	for data != "" {
		if p.state == streamToolStateText {
			data = p.consumeTextState(data, &texts)
			continue
		}
		data = p.consumeToolState(data, &texts, &calls)
	}
	return strings.Join(texts, ""), calls
}

func (p *streamToolCallParser) Flush() (string, []ToolCall) {
	if p.state == streamToolStateText {
		text := p.partial
		p.partial = ""
		return text, nil
	}

	raw := p.toolBuffer.String() + p.partial
	p.toolBuffer.Reset()
	p.partial = ""
	p.state = streamToolStateText

	if call := ParseToolCallBlock(raw, p.tools, p.nextIndex); call != nil {
		p.nextIndex++
		return "", []ToolCall{*call}
	}
	if raw == "" {
		return "", nil
	}
	return toolCallStartTag + raw, nil
}

func (p *streamToolCallParser) consumeTextState(data string, texts *[]string) string {
	startIdx := strings.Index(data, toolCallStartTag)
	if startIdx < 0 {
		text, carry := SplitByTagPrefix(data, toolCallStartTag)
		if text != "" {
			*texts = append(*texts, text)
		}
		p.partial = carry
		return ""
	}
	if startIdx > 0 {
		*texts = append(*texts, data[:startIdx])
	}
	p.state = streamToolStateCall
	return data[startIdx+len(toolCallStartTag):]
}

func (p *streamToolCallParser) consumeToolState(data string, texts *[]string, calls *[]ToolCall) string {
	endIdx := strings.Index(data, toolCallEndTag)
	if endIdx < 0 {
		chunk, carry := SplitByTagPrefix(data, toolCallEndTag)
		if chunk != "" {
			p.toolBuffer.WriteString(chunk)
		}
		p.partial = carry
		return ""
	}

	p.toolBuffer.WriteString(data[:endIdx])
	raw := p.toolBuffer.String()
	if call := ParseToolCallBlock(raw, p.tools, p.nextIndex); call != nil {
		*calls = append(*calls, *call)
		p.nextIndex++
	} else if strings.TrimSpace(raw) != "" {
		*texts = append(*texts, toolCallStartTag+raw+toolCallEndTag)
	}

	p.toolBuffer.Reset()
	p.state = streamToolStateText
	return data[endIdx+len(toolCallEndTag):]
}
