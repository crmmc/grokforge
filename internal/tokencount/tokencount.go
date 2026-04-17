package tokencount

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

const promptOverhead = 4
const fallbackCharsPerToken = 3

var (
	codecOnce sync.Once
	codec     tokenizer.Codec
	codecErr  error
)

func EstimateTokens(value any) int {
	text := coerceText(value)
	if text == "" {
		return 0
	}

	tokenCodec, err := getCodec()
	if err != nil {
		return fallbackEstimate(text)
	}

	count, err := tokenCodec.Count(text)
	if err != nil {
		return fallbackEstimate(text)
	}
	return count
}

func EstimatePromptTokens(value any) int {
	base := EstimateTokens(value)
	if base == 0 {
		return 0
	}
	return base + promptOverhead
}

func getCodec() (tokenizer.Codec, error) {
	codecOnce.Do(func() {
		codec, codecErr = tokenizer.Get(tokenizer.O200kBase)
	})
	return codec, codecErr
}

func coerceText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return strings.TrimSpace(string(raw))
	}
}

func fallbackEstimate(text string) int {
	length := len(strings.TrimSpace(text))
	if length == 0 {
		return 0
	}
	return (length + fallbackCharsPerToken - 1) / fallbackCharsPerToken
}
