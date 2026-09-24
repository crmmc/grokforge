package tokencount

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tiktoken-go/tokenizer"
)

// fakeCodec implements tokenizer.Codec with canned responses for tests.
type fakeCodec struct {
	count int
	err   error
}

func (f fakeCodec) GetName() string                         { return "fake" }
func (f fakeCodec) Count(string) (int, error)               { return f.count, f.err }
func (f fakeCodec) Encode(string) ([]uint, []string, error) { return nil, nil, nil }
func (f fakeCodec) Decode([]uint) (string, error)           { return "", nil }

// swapCodec force-replaces the package codec and error for the duration of
// the test. The real codec is initialized first so sync.Once has fired and
// the swap is deterministic.
func swapCodec(t *testing.T, c tokenizer.Codec, err error) {
	t.Helper()
	_, _ = getCodec()
	origCodec, origErr := codec, codecErr
	codec, codecErr = c, err
	t.Cleanup(func() { codec, codecErr = origCodec, origErr })
}

func TestCoerceText(t *testing.T) {
	type payload struct {
		Role string `json:"role"`
		Text string `json:"text"`
	}
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil yields empty text", value: nil, want: ""},
		{name: "string is trimmed", value: "  hello world  ", want: "hello world"},
		{name: "empty string stays empty", value: "   ", want: ""},
		{name: "int is marshaled", value: 42, want: "42"},
		{name: "struct is marshaled to JSON", value: payload{Role: "user", Text: "hi"}, want: `{"role":"user","text":"hi"}`},
		{name: "slice is marshaled to JSON", value: []string{"a", "b"}, want: `["a","b"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, coerceText(tc.value))
		})
	}

	t.Run("unmarshalable value falls back to fmt.Sprint", func(t *testing.T) {
		value := make(chan int)
		want := strings.TrimSpace(fmt.Sprint(value))
		require.NotEmpty(t, want)
		assert.Equal(t, want, coerceText(value))
	})
}

func TestFallbackEstimate(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "empty text", text: "", want: 0},
		{name: "whitespace only", text: "   ", want: 0},
		{name: "one char rounds up to one token", text: "a", want: 1},
		{name: "two chars stay one token", text: "ab", want: 1},
		{name: "three chars stay one token", text: "abc", want: 1},
		{name: "four chars round up to two tokens", text: "abcd", want: 2},
		{name: "eight chars round up to three tokens", text: "abcdefgh", want: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, fallbackEstimate(tc.text))
		})
	}
}

func TestEstimateTokens_EmptyInputs(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "empty string", value: ""},
		{name: "whitespace string", value: "   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Zero(t, EstimateTokens(tc.value))
		})
	}
}

func TestEstimateTokens_UsesCodecCount(t *testing.T) {
	swapCodec(t, fakeCodec{count: 7}, nil)
	assert.Equal(t, 7, EstimateTokens("hello world"))
}

func TestEstimateTokens_FallsBackWhenCodecUnavailable(t *testing.T) {
	swapCodec(t, nil, errors.New("codec unavailable"))
	assert.Equal(t, fallbackEstimate("hello world"), EstimateTokens("hello world"))
}

func TestEstimateTokens_FallsBackWhenCountFails(t *testing.T) {
	swapCodec(t, fakeCodec{err: errors.New("count failed")}, nil)
	assert.Equal(t, fallbackEstimate("hello world"), EstimateTokens("hello world"))
}

func TestEstimatePromptTokens_OverheadTable(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "empty input stays zero", value: nil},
		{name: "non-empty input gets prompt overhead", value: "hello world"},
		{name: "structured input gets prompt overhead", value: map[string]any{"k": "v"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := EstimateTokens(tc.value)
			got := EstimatePromptTokens(tc.value)
			if base == 0 {
				assert.Zero(t, got)
			} else {
				assert.Equal(t, base+promptOverhead, got)
			}
		})
	}
}

func TestEstimateTokens_RealCodecCountsPositive(t *testing.T) {
	assert.Positive(t, EstimateTokens("hello world"))
	assert.Positive(t, EstimateTokens(map[string]any{"messages": []string{"a", "b"}}))
}
