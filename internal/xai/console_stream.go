package xai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxConsoleStreamFrameSize = 8 * 1024 * 1024

type consoleSSEBlock struct {
	event string
	data  []string
}

// parseConsoleSSEStream reads Console Responses API SSE blocks.
func (c *client) parseConsoleSSEStream(body io.Reader, events chan<- StreamEvent) error {
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxConsoleStreamFrameSize)

	var block consoleSSEBlock
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if done, err := emitConsoleSSEBlock(block, events); done || err != nil {
				return err
			}
			block = consoleSSEBlock{}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if value, ok := strings.CutPrefix(line, "event:"); ok {
			block.event = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			block.data = append(block.data, strings.TrimPrefix(value, " "))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	_, err := emitConsoleSSEBlock(block, events)
	return err
}

func emitConsoleSSEBlock(block consoleSSEBlock, events chan<- StreamEvent) (bool, error) {
	if len(block.data) == 0 {
		return false, nil
	}
	data := strings.Join(block.data, "\n")
	if data == "[DONE]" {
		return true, nil
	}

	wrapped := struct {
		Event string `json:"event"`
		Data  any    `json:"data"`
	}{Event: block.event}

	var raw json.RawMessage
	if json.Unmarshal([]byte(data), &raw) == nil {
		wrapped.Data = raw
	} else {
		wrapped.Data = data
	}

	payload, err := json.Marshal(wrapped)
	if err != nil {
		return false, err
	}
	events <- StreamEvent{Data: payload}
	return false, nil
}
