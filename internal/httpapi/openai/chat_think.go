package openai

import "strings"

const (
	thinkOpenTag  = "<think>\n"
	thinkCloseTag = "\n</think>\n"
)

type thinkStreamWriter struct {
	show bool
	open bool
}

func newThinkStreamWriter(show bool) *thinkStreamWriter {
	return &thinkStreamWriter{show: show}
}

func (t *thinkStreamWriter) HandleReasoning(text string) string {
	if !t.show || text == "" {
		return ""
	}
	if !t.open {
		t.open = true
		return thinkOpenTag + text
	}
	return text
}

func (t *thinkStreamWriter) HandleText(text string) string {
	if text == "" {
		return ""
	}
	if t.open {
		t.open = false
		return thinkCloseTag + text
	}
	return text
}

func (t *thinkStreamWriter) Close() string {
	if !t.open {
		return ""
	}
	t.open = false
	return thinkCloseTag
}

type thinkCollector struct {
	show  bool
	open  bool
	parts []string
}

func newThinkCollector(show bool) *thinkCollector {
	return &thinkCollector{show: show}
}

func (t *thinkCollector) AddReasoning(text string) {
	if !t.show || text == "" {
		return
	}
	if !t.open {
		t.open = true
		t.parts = append(t.parts, thinkOpenTag)
	}
	t.parts = append(t.parts, text)
}

func (t *thinkCollector) AddText(text string) {
	if text == "" {
		return
	}
	if t.open {
		t.open = false
		t.parts = append(t.parts, thinkCloseTag)
	}
	t.parts = append(t.parts, text)
}

func (t *thinkCollector) Finalize() string {
	if t.open {
		t.open = false
		t.parts = append(t.parts, thinkCloseTag)
	}
	return strings.Join(t.parts, "")
}
