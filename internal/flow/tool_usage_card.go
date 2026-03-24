package flow

import (
	"encoding/json"
	"regexp"
	"strings"
)

const (
	toolUsageCardTag      = "xai:tool_usage_card"
	toolUsageCardStartTag = "<xai:tool_usage_card"
	toolUsageCardEndTag   = "</xai:tool_usage_card>"
)

var (
	cdataRegex   = regexp.MustCompile(`<!\[CDATA\[(.*?)\]\]>`)
	htmlTagRegex = regexp.MustCompile(`<[^>]+>`)
)

type streamTokenFilter struct {
	dropParsers []*dropTagParser
	toolCardOn  bool
	toolCardPsr *toolUsageCardParser
}

func newStreamTokenFilter(tags []string) *streamTokenFilter {
	filter := &streamTokenFilter{dropParsers: make([]*dropTagParser, 0, len(tags))}
	for _, tag := range tags {
		normalized := strings.TrimSpace(tag)
		if normalized == "" {
			continue
		}
		if normalized == toolUsageCardTag {
			filter.toolCardOn = true
			continue
		}
		filter.dropParsers = append(filter.dropParsers, newDropTagParser(normalized))
	}
	if filter.toolCardOn {
		filter.toolCardPsr = &toolUsageCardParser{}
	}
	return filter
}

func (f *streamTokenFilter) Apply(event StreamEvent) StreamEvent {
	event.Content = f.filterText(event.Content, event.RolloutID)
	event.ReasoningContent = f.filterText(event.ReasoningContent, event.RolloutID)
	return event
}

func (f *streamTokenFilter) filterText(text, rolloutID string) string {
	if text == "" {
		return text
	}
	filtered := text
	if f.toolCardPsr != nil {
		filtered = f.toolCardPsr.Consume(filtered, rolloutID)
	}
	for _, parser := range f.dropParsers {
		filtered = parser.Consume(filtered)
	}
	return filtered
}

func (f *streamTokenFilter) Flush(rolloutID string) string {
	filtered := ""
	if f.toolCardPsr != nil {
		filtered = f.toolCardPsr.Flush(rolloutID)
	}
	for _, parser := range f.dropParsers {
		filtered = parser.Consume(filtered) + parser.Flush()
	}
	return filtered
}

type toolUsageCardParser struct {
	inCard    bool
	partial   string
	rolloutID string
	buffer    strings.Builder
}

func (p *toolUsageCardParser) Consume(chunk, rolloutID string) string {
	if chunk == "" {
		return ""
	}
	if rid := strings.TrimSpace(rolloutID); rid != "" {
		p.rolloutID = rid
	}
	data := p.partial + chunk
	p.partial = ""

	var out strings.Builder
	for data != "" {
		if p.inCard {
			data = p.consumeCardContent(data, &out)
			continue
		}
		data = p.consumePlainText(data, &out)
	}
	return out.String()
}

func (p *toolUsageCardParser) Flush(rolloutID string) string {
	if rid := strings.TrimSpace(rolloutID); rid != "" {
		p.rolloutID = rid
	}
	if p.inCard {
		p.buffer.Reset()
		p.partial = ""
		p.inCard = false
		return ""
	}
	text := p.partial
	p.partial = ""
	return text
}

func (p *toolUsageCardParser) consumeCardContent(data string, out *strings.Builder) string {
	endIdx := strings.Index(data, toolUsageCardEndTag)
	if endIdx < 0 {
		chunk, carry := SplitByTagPrefix(data, toolUsageCardEndTag)
		if chunk != "" {
			p.buffer.WriteString(chunk)
		}
		p.partial = carry
		return ""
	}

	endPos := endIdx + len(toolUsageCardEndTag)
	p.buffer.WriteString(data[:endPos])
	p.emitCardLine(out, p.buffer.String())
	p.buffer.Reset()
	p.inCard = false
	return data[endPos:]
}

func (p *toolUsageCardParser) consumePlainText(data string, out *strings.Builder) string {
	startIdx := strings.Index(data, toolUsageCardStartTag)
	if startIdx < 0 {
		text, carry := SplitByTagPrefix(data, toolUsageCardStartTag)
		if text != "" {
			out.WriteString(text)
		}
		p.partial = carry
		return ""
	}
	if startIdx > 0 {
		out.WriteString(data[:startIdx])
	}

	cardData := data[startIdx:]
	endIdx := strings.Index(cardData, toolUsageCardEndTag)
	if endIdx < 0 {
		p.inCard = true
		p.buffer.WriteString(cardData)
		return ""
	}

	endPos := endIdx + len(toolUsageCardEndTag)
	p.emitCardLine(out, cardData[:endPos])
	return cardData[endPos:]
}

func (p *toolUsageCardParser) emitCardLine(out *strings.Builder, raw string) {
	line := parseToolUsageLine(raw, p.rolloutID)
	if line == "" {
		return
	}
	current := out.String()
	if len(current) > 0 && !strings.HasSuffix(current, "\n") {
		out.WriteByte('\n')
	}
	out.WriteString(line)
	out.WriteByte('\n')
}

func parseToolUsageLine(raw, rolloutID string) string {
	name := extractTagValue(raw, "xai:tool_name")
	args := extractTagValue(raw, "xai:tool_args")
	payload := parseToolArgs(args)
	prefix := ""
	if strings.TrimSpace(rolloutID) != "" {
		prefix = "[" + strings.TrimSpace(rolloutID) + "]"
	}

	label, text := name, args
	switch name {
	case "web_search":
		label = prefix + "[WebSearch]"
		text = firstNonEmptyText(payload, "query", "q")
		if text == "" {
			text = args
		}
	case "search_images":
		label = prefix + "[SearchImage]"
		text = firstNonEmptyText(payload, "image_description", "description", "query")
		if text == "" {
			text = args
		}
	case "chatroom_send":
		label = prefix + "[AgentThink]"
		text = firstNonEmptyText(payload, "message")
		if text == "" {
			text = args
		}
	}

	switch {
	case strings.TrimSpace(label) != "" && strings.TrimSpace(text) != "":
		return strings.TrimSpace(label + " " + text)
	case strings.TrimSpace(label) != "":
		return strings.TrimSpace(label)
	case strings.TrimSpace(text) != "":
		return strings.TrimSpace(text)
	default:
		return strings.TrimSpace(htmlTagRegex.ReplaceAllString(raw, ""))
	}
}

func extractTagValue(raw, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	startIdx := strings.Index(raw, startTag)
	if startIdx < 0 {
		return ""
	}
	start := startIdx + len(startTag)
	endRelative := strings.Index(raw[start:], endTag)
	if endRelative < 0 {
		return ""
	}
	value := raw[start : start+endRelative]
	return stripCDATA(strings.TrimSpace(value))
}

func stripCDATA(value string) string {
	if value == "" {
		return ""
	}
	return strings.TrimSpace(cdataRegex.ReplaceAllString(value, "$1"))
}

func parseToolArgs(raw string) map[string]any {
	args := strings.TrimSpace(raw)
	if args == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return nil
	}
	return payload
}

func firstNonEmptyText(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, key := range keys {
		value, _ := payload[key].(string)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// SplitByTagPrefix splits text at the point where a suffix of text matches
// a prefix of tag, returning (clean, carry). This prevents partial tag
// matches from being emitted during streaming.
func SplitByTagPrefix(text, tag string) (string, string) {
	if text == "" {
		return "", ""
	}
	keep := SuffixPrefixLength(text, tag)
	if keep == 0 {
		return text, ""
	}
	return text[:len(text)-keep], text[len(text)-keep:]
}

// SuffixPrefixLength returns the length of the longest suffix of text
// that matches a prefix of tag.
func SuffixPrefixLength(text, tag string) int {
	maxKeep := len(tag) - 1
	if maxKeep <= 0 {
		return 0
	}
	if len(text) < maxKeep {
		maxKeep = len(text)
	}
	for keep := maxKeep; keep > 0; keep-- {
		if strings.HasSuffix(text, tag[:keep]) {
			return keep
		}
	}
	return 0
}
