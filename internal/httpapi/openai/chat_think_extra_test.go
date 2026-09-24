package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThinkStreamWriter_Disabled(t *testing.T) {
	w := newThinkStreamWriter(false)
	assert.Equal(t, "", w.HandleReasoning("thought"))
	// HandleText does not depend on the show flag: text always passes through.
	assert.Equal(t, "answer", w.HandleText("answer"))
	assert.Equal(t, "", w.Close())
}

func TestThinkStreamWriter_EmptyInputs(t *testing.T) {
	w := newThinkStreamWriter(true)
	assert.Equal(t, "", w.HandleReasoning(""))
	assert.Equal(t, "", w.HandleText(""))
}

func TestThinkStreamWriter_Lifecycle(t *testing.T) {
	w := newThinkStreamWriter(true)

	assert.Equal(t, thinkOpenTag+"first", w.HandleReasoning("first"))
	assert.Equal(t, "more", w.HandleReasoning("more"))
	assert.Equal(t, thinkCloseTag+"answer", w.HandleText("answer"))
	assert.Equal(t, "tail", w.HandleText("tail"))
	assert.Equal(t, "", w.Close())
}

func TestThinkStreamWriter_CloseWhileOpen(t *testing.T) {
	w := newThinkStreamWriter(true)
	assert.Equal(t, thinkOpenTag+"only", w.HandleReasoning("only"))
	assert.Equal(t, thinkCloseTag, w.Close())
	assert.Equal(t, "", w.Close())
}

func TestThinkCollector_DisabledAndEmpty(t *testing.T) {
	c := newThinkCollector(false)
	c.AddReasoning("thought")
	c.AddText("")
	assert.Equal(t, "", c.Finalize())
}

func TestThinkCollector_ReasoningThenText(t *testing.T) {
	c := newThinkCollector(true)
	c.AddReasoning("step one")
	c.AddReasoning("step two")
	c.AddText("answer")
	assert.Equal(t, thinkOpenTag+"step one"+"step two"+thinkCloseTag+"answer", c.Finalize())
}

func TestThinkCollector_EmptyReasoningIgnored(t *testing.T) {
	c := newThinkCollector(true)
	c.AddReasoning("")
	c.AddText("answer")
	assert.Equal(t, "answer", c.Finalize())
}

func TestThinkCollector_FinalizeClosesOpenThinking(t *testing.T) {
	c := newThinkCollector(true)
	c.AddReasoning("unfinished")
	assert.Equal(t, thinkOpenTag+"unfinished"+thinkCloseTag, c.Finalize())
	assert.Equal(t, thinkOpenTag+"unfinished"+thinkCloseTag, c.Finalize())
}

func TestThinkCollector_TextOnly(t *testing.T) {
	c := newThinkCollector(true)
	c.AddText("hello")
	c.AddText("")
	c.AddText("world")
	assert.Equal(t, "helloworld", c.Finalize())
}
