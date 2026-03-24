package flow

import "strings"

type dropTagParser struct {
	startTag string
	endTag   string
	inTag    bool
	partial  string
}

func newDropTagParser(tag string) *dropTagParser {
	normalized := strings.TrimSpace(tag)
	return &dropTagParser{
		startTag: "<" + normalized,
		endTag:   "</" + normalized + ">",
	}
}

func (p *dropTagParser) Consume(chunk string) string {
	if chunk == "" {
		return ""
	}
	data := p.partial + chunk
	p.partial = ""

	var out strings.Builder
	for data != "" {
		if p.inTag {
			data = p.consumeTagContent(data)
			continue
		}
		data = p.consumePlainText(data, &out)
	}
	return out.String()
}

func (p *dropTagParser) Flush() string {
	if p.inTag {
		p.partial = ""
		p.inTag = false
		return ""
	}
	text := p.partial
	p.partial = ""
	return text
}

func (p *dropTagParser) consumePlainText(data string, out *strings.Builder) string {
	startIdx := strings.Index(data, p.startTag)
	if startIdx < 0 {
		text, carry := SplitByTagPrefix(data, p.startTag)
		if text != "" {
			out.WriteString(text)
		}
		p.partial = carry
		return ""
	}
	if startIdx > 0 {
		out.WriteString(data[:startIdx])
	}

	remaining := data[startIdx:]
	openEnd := strings.Index(remaining, ">")
	if openEnd < 0 {
		p.partial = remaining
		return ""
	}
	if strings.HasSuffix(remaining[:openEnd+1], "/>") {
		return remaining[openEnd+1:]
	}

	p.inTag = true
	return remaining[openEnd+1:]
}

func (p *dropTagParser) consumeTagContent(data string) string {
	endIdx := strings.Index(data, p.endTag)
	if endIdx < 0 {
		_, carry := SplitByTagPrefix(data, p.endTag)
		p.partial = carry
		return ""
	}

	p.inTag = false
	return data[endIdx+len(p.endTag):]
}
