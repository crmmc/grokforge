package xai

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConsoleSSEStreamSkipsCommentsAndHandlesCRLF(t *testing.T) {
	input := ": keep-alive\r\n" +
		"event: response.created\r\n" +
		"data: {\"id\":7}\r\n" +
		"\r\n" +
		"event: no-data-only\r\n" +
		"\r\n"
	events := make(chan StreamEvent, 10)
	c := &client{}

	require.NoError(t, c.parseConsoleSSEStream(strings.NewReader(input), events))
	close(events)

	var got []StreamEvent
	for ev := range events {
		got = append(got, ev)
	}
	require.Len(t, got, 1)

	var wrapped struct {
		Event string `json:"event"`
		Data  struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got[0].Data, &wrapped))
	assert.Equal(t, "response.created", wrapped.Event)
	assert.Equal(t, 7, wrapped.Data.ID)
}

func TestParseConsoleSSEStreamScannerError(t *testing.T) {
	c := &client{}
	events := make(chan StreamEvent, 4)
	err := c.parseConsoleSSEStream(&errorReader{err: errors.New("read fail")}, events)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNetwork)
}

func TestEmitConsoleSSEBlockEmptyBlockIsNoop(t *testing.T) {
	events := make(chan StreamEvent, 4)
	done, err := emitConsoleSSEBlock(consoleSSEBlock{}, events)
	assert.False(t, done)
	assert.NoError(t, err)
	assert.Empty(t, events)
}
