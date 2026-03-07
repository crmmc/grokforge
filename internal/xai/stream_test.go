package xai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSSEStream_SingleEvent(t *testing.T) {
	input := `data: {"result":{"response":{"text":"Hello"}}}

`
	events := make(chan StreamEvent, 10)
	c := &client{}

	err := c.parseSSEStream(strings.NewReader(input), events)
	close(events)

	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	count := 0
	for ev := range events {
		if ev.Error != nil {
			t.Errorf("Unexpected error: %v", ev.Error)
		}
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 event, got %d", count)
	}
}

func TestParseSSEStream_MultipleEvents(t *testing.T) {
	input := `data: {"chunk":1}

data: {"chunk":2}

data: {"chunk":3}

data: [DONE]
`
	events := make(chan StreamEvent, 10)
	c := &client{}

	err := c.parseSSEStream(strings.NewReader(input), events)
	close(events)

	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	var chunks []int
	for ev := range events {
		if ev.Error != nil {
			t.Errorf("Unexpected error: %v", ev.Error)
			continue
		}
		var data struct {
			Chunk int `json:"chunk"`
		}
		if err := json.Unmarshal(ev.Data, &data); err == nil && data.Chunk > 0 {
			chunks = append(chunks, data.Chunk)
		}
	}

	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestParseSSEStream_SkipsComments(t *testing.T) {
	input := `: this is a comment
data: {"value":1}

: another comment
data: {"value":2}

`
	events := make(chan StreamEvent, 10)
	c := &client{}

	err := c.parseSSEStream(strings.NewReader(input), events)
	close(events)

	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	count := 0
	for ev := range events {
		if ev.Error != nil {
			t.Errorf("Unexpected error: %v", ev.Error)
		}
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 events (comments skipped), got %d", count)
	}
}

func TestParseSSEStream_EmptyLines(t *testing.T) {
	input := `

data: {"value":1}



data: {"value":2}

`
	events := make(chan StreamEvent, 10)
	c := &client{}

	err := c.parseSSEStream(strings.NewReader(input), events)
	close(events)

	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	count := 0
	for ev := range events {
		if ev.Error != nil {
			t.Errorf("Unexpected error: %v", ev.Error)
		}
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 events (empty lines skipped), got %d", count)
	}
}

func TestBuildChatBody(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Model:  "grok-3",
		Stream: true,
	}

	body, err := buildChatBody(req)
	if err != nil {
		t.Fatalf("buildChatBody error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("Failed to unmarshal body: %v", err)
	}

	// Check required fields
	if payload["modelName"] != "grok-3" {
		t.Errorf("modelName = %v, want grok-3", payload["modelName"])
	}
	if payload["message"] != "Hello" {
		t.Errorf("message = %v, want Hello", payload["message"])
	}
	if payload["temporary"] != false {
		t.Errorf("temporary = %v, want false", payload["temporary"])
	}

	// P0 #1: modelMode must be present
	if payload["modelMode"] != "MODEL_MODE_GROK_3" {
		t.Errorf("modelMode = %v, want MODEL_MODE_GROK_3", payload["modelMode"])
	}

	// P0 #3: responseMetadata must be present
	rm, ok := payload["responseMetadata"].(map[string]any)
	if !ok {
		t.Fatal("responseMetadata missing or not an object")
	}
	rmd, ok := rm["requestModelDetails"].(map[string]any)
	if !ok {
		t.Fatal("responseMetadata.requestModelDetails missing or not an object")
	}
	if rmd["modelId"] != "grok-3" {
		t.Errorf("responseMetadata.requestModelDetails.modelId = %v, want grok-3", rmd["modelId"])
	}

	// P0 #4: customInstructions must NOT be present
	if _, exists := payload["customInstructions"]; exists {
		t.Error("payload should NOT contain customInstructions")
	}

	// isPreset and deepsearchPreset must NOT be present
	if _, exists := payload["isPreset"]; exists {
		t.Error("payload should NOT contain isPreset")
	}
	if _, exists := payload["deepsearchPreset"]; exists {
		t.Error("payload should NOT contain deepsearchPreset")
	}

	// customPersonality should be absent when CustomInstruction is empty
	if _, exists := payload["customPersonality"]; exists {
		t.Error("customPersonality should be absent when CustomInstruction is empty")
	}
}

func TestBuildChatBody_CustomPersonality(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		wantPresent bool
	}{
		{"non-empty", "Be concise", true},
		{"empty", "", false},
		{"whitespace-only", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatRequest{
				Messages:          []Message{{Role: "user", Content: "hi"}},
				Model:             "grok-3",
				CustomInstruction: tt.instruction,
			}
			body, err := buildChatBody(req)
			if err != nil {
				t.Fatalf("buildChatBody error: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			_, exists := payload["customPersonality"]
			if exists != tt.wantPresent {
				t.Errorf("customPersonality present=%v, want %v", exists, tt.wantPresent)
			}
			if tt.wantPresent {
				if payload["customPersonality"] != tt.instruction {
					t.Errorf("customPersonality = %v, want %v", payload["customPersonality"], tt.instruction)
				}
			}
		})
	}
}

func TestBuildChatBody_ResponseMetadata_WithTemperature(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Model:    "grok-4-heavy",
	}
	temp := 0.7
	req.Temperature = &temp
	body, err := buildChatBody(req)
	if err != nil {
		t.Fatalf("buildChatBody error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// modelMode should reflect grok-4-heavy
	if payload["modelMode"] != "MODEL_MODE_HEAVY" {
		t.Errorf("modelMode = %v, want MODEL_MODE_HEAVY", payload["modelMode"])
	}

	// modelName should be mapped to grok-4
	if payload["modelName"] != "grok-4" {
		t.Errorf("modelName = %v, want grok-4", payload["modelName"])
	}

	// responseMetadata should contain modelConfigOverride with temperature
	rm, ok := payload["responseMetadata"].(map[string]any)
	if !ok {
		t.Fatal("responseMetadata missing or not an object")
	}
	mco, ok := rm["modelConfigOverride"].(map[string]any)
	if !ok {
		t.Fatal("responseMetadata.modelConfigOverride missing or not an object")
	}
	if mco["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", mco["temperature"])
	}
}

func TestMapModel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Direct mappings from Python reference
		{"grok-3", "grok-3"},
		{"grok-3-mini", "grok-3"},
		{"grok-3-thinking", "grok-3"},
		{"grok-4", "grok-4"},
		{"grok-4-mini", "grok-4-mini"},
		{"grok-4-thinking", "grok-4"},
		{"grok-4-heavy", "grok-4"},
		{"grok-4.1-mini", "grok-4-1-thinking-1129"},
		{"grok-4.1-fast", "grok-4-1-thinking-1129"},
		{"grok-4.1-expert", "grok-4-1-thinking-1129"},
		{"grok-4.1-thinking", "grok-4-1-thinking-1129"},
		{"grok-4.20-beta", "grok-420"},
		{"grok-imagine-1.0", "grok-3"},
		{"grok-imagine-1.0-fast", "grok-3"},
		{"grok-imagine-1.0-edit", "imagine-image-edit"},
		{"grok-imagine-1.0-video", "grok-3"},
		// Defaults
		{"unknown", "grok-3"},
		{"", "grok-3"},
	}

	for _, tt := range tests {
		got := mapModel(tt.input)
		if got != tt.want {
			t.Errorf("mapModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestModelModeForModel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"grok-3", "MODEL_MODE_GROK_3"},
		{"grok-3-mini", "MODEL_MODE_GROK_3_MINI_THINKING"},
		{"grok-4", "MODEL_MODE_GROK_4"},
		{"grok-4-heavy", "MODEL_MODE_HEAVY"},
		{"grok-4.1-fast", "MODEL_MODE_FAST"},
		{"grok-4.1-expert", "MODEL_MODE_EXPERT"},
		{"grok-4.20-beta", "MODEL_MODE_GROK_420"},
		{"grok-imagine-1.0", "MODEL_MODE_FAST"},
		{"grok-imagine-1.0-edit", "MODEL_MODE_FAST"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := modelModeForModel(tt.input)
		if got != tt.want {
			t.Errorf("modelModeForModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseSSEStream_NDJSON(t *testing.T) {
	// Bare JSON lines without "data: " prefix (NDJSON format)
	input := `{"result":{"response":{"text":"Hello"}}}
{"result":{"response":{"text":"World"}}}
`
	events := make(chan StreamEvent, 10)
	c := &client{}

	err := c.parseSSEStream(strings.NewReader(input), events)
	close(events)

	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	count := 0
	for ev := range events {
		if ev.Error != nil {
			t.Errorf("Unexpected error: %v", ev.Error)
		}
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 events from NDJSON, got %d", count)
	}
}

func TestParseSSEStream_MixedFormat(t *testing.T) {
	// Mix of SSE and NDJSON lines
	input := `data: {"chunk":1}

{"chunk":2}
data: {"chunk":3}

data: [DONE]
`
	events := make(chan StreamEvent, 10)
	c := &client{}

	err := c.parseSSEStream(strings.NewReader(input), events)
	close(events)

	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	var chunks []int
	for ev := range events {
		if ev.Error != nil {
			t.Errorf("Unexpected error: %v", ev.Error)
			continue
		}
		var data struct {
			Chunk int `json:"chunk"`
		}
		if err := json.Unmarshal(ev.Data, &data); err == nil && data.Chunk > 0 {
			chunks = append(chunks, data.Chunk)
		}
	}

	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks from mixed format, got %d: %v", len(chunks), chunks)
	}
}

func TestBuildChatBody_P1PayloadFields(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Model:    "grok-3",
	}
	body, err := buildChatBody(req)
	if err != nil {
		t.Fatalf("buildChatBody error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// P1 #6: deviceEnvInfo must be present with nested fields
	dei, ok := payload["deviceEnvInfo"].(map[string]any)
	if !ok {
		t.Fatal("deviceEnvInfo missing or not an object")
	}
	wantDEI := map[string]any{
		"darkModeEnabled":  false,
		"devicePixelRatio": float64(2),
		"screenWidth":      float64(2056),
		"screenHeight":     float64(1329),
		"viewportWidth":    float64(2056),
		"viewportHeight":   float64(1083),
	}
	for k, want := range wantDEI {
		got, exists := dei[k]
		if !exists {
			t.Errorf("deviceEnvInfo.%s missing", k)
		} else if got != want {
			t.Errorf("deviceEnvInfo.%s = %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}

	// P1 #6: boolean fields must be false
	boolFields := []string{
		"disableSelfHarmShortCircuit",
		"disableTextFollowUps",
		"forceSideBySide",
		"isAsyncChat",
	}
	for _, field := range boolFields {
		val, exists := payload[field]
		if !exists {
			t.Errorf("%s missing from payload", field)
		} else if val != false {
			t.Errorf("%s = %v, want false", field, val)
		}
	}
}

func TestGenStatsigID(t *testing.T) {
	// Generate multiple IDs and verify they're valid base64
	for range 10 {
		id := genStatsigID()
		if id == "" {
			t.Error("genStatsigID returned empty string")
		}

		// Should be valid base64 - just check it's non-empty
		if len(id) < 10 {
			t.Errorf("genStatsigID returned too short: %s", id)
		}
	}

	// Verify uniqueness
	ids := make(map[string]bool)
	for range 100 {
		id := genStatsigID()
		if ids[id] {
			t.Error("genStatsigID generated duplicate ID")
		}
		ids[id] = true
	}
}
