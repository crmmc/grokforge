package xai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func (c *ImagineClient) streamImages(
	ctx context.Context,
	conn *websocket.Conn,
	requestID, prompt, aspectRatio string,
	enableNSFW, enablePro bool,
	eventCh chan<- ImageEvent,
) {
	defer close(eventCh)
	defer conn.Close()
	doneCh := make(chan struct{})
	defer close(doneCh)
	safeSend := func(event ImageEvent) {
		select {
		case <-doneCh:
			return
		case eventCh <- event:
		}
	}

	// Send reset message first (per grok2api protocol)
	resetMsg := map[string]any{
		"type":      "conversation.item.create",
		"timestamp": time.Now().UnixMilli(),
		"item": map[string]any{
			"type": "message",
			"content": []map[string]string{{"type": "reset"}},
		},
	}
	if err := conn.WriteJSON(resetMsg); err != nil {
		safeSend(ImageEvent{Type: ImageEventError, Error: fmt.Errorf("write reset: %w", err)})
		return
	}

	// Send request message
	req := buildImagineRequest(requestID, prompt, aspectRatio, enableNSFW, enablePro)
	if err := conn.WriteJSON(req); err != nil {
		safeSend(ImageEvent{Type: ImageEventError, Error: fmt.Errorf("write request: %w", err)})
		return
	}

	// Track state for blocked detection
	var (
		mu              sync.Mutex
		hasMedium       bool
		hasFinal        bool
		blockedTimer    *time.Timer
		blockedTimerSet bool
	)
	// Cleanup blocked timer on exit
	defer func() {
		mu.Lock()
		if blockedTimer != nil {
			blockedTimer.Stop()
		}
		mu.Unlock()
	}()

	// Read messages
	for {
		select {
		case <-ctx.Done():
			safeSend(ImageEvent{Type: ImageEventError, Error: ctx.Err()})
			return
		default:
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(defaultWSReadDeadline))

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return
			}
			mu.Lock()
			finalReceived := hasFinal
			mu.Unlock()
			if finalReceived {
				return
			}
			safeSend(ImageEvent{Type: ImageEventError, Error: fmt.Errorf("read message: %w", err)})
			return
		}

		resp, err := parseImagineResponse(msg)
		if err != nil {
			continue // Skip malformed messages
		}

		switch resp.Type {
		case "image.preview":
			safeSend(ImageEvent{
				Type:      ImageEventPreview,
				RequestID: resp.Item.RequestID,
				ImageData: resp.Item.ImageData,
			})
		case "image.medium":
			mu.Lock()
			hasMedium = true
			// Start blocked detection timer
			if !blockedTimerSet {
				blockedTimerSet = true
				graceDuration := time.Duration(c.blockedGraceMillis) * time.Millisecond
				blockedTimer = time.AfterFunc(graceDuration, func() {
					mu.Lock()
					defer mu.Unlock()
					if hasMedium && !hasFinal {
						safeSend(ImageEvent{
							Type:      ImageEventBlocked,
							RequestID: resp.Item.RequestID,
						})
					}
				})
			}
			mu.Unlock()

			safeSend(ImageEvent{
				Type:      ImageEventMedium,
				RequestID: resp.Item.RequestID,
				ImageData: resp.Item.ImageData,
			})

		case "image.final":
			mu.Lock()
			hasFinal = true
			if blockedTimer != nil {
				blockedTimer.Stop()
			}
			mu.Unlock()

			safeSend(ImageEvent{
				Type:      ImageEventFinal,
				RequestID: resp.Item.RequestID,
				ImageData: resp.Item.ImageData,
			})
			return

		case "done":
			return

		case "error":
			safeSend(ImageEvent{
				Type:  ImageEventError,
				Error: resp.Error,
			})
			return
		}
	}
}

func parseImagineResponse(msg []byte) (*ImagineResponse, error) {
	var resp ImagineResponse
	if err := json.Unmarshal(msg, &resp); err != nil {
		return nil, err
	}
	if resp.Type == "image" {
		converted, ok := parseLegacyImageResponse(msg)
		if !ok {
			return nil, fmt.Errorf("legacy image payload invalid")
		}
		return converted, nil
	}
	if resp.Type == "error" {
		var legacyErr struct {
			ErrMsg  string `json:"err_msg"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(msg, &legacyErr)
		errMsg := strings.TrimSpace(legacyErr.ErrMsg)
		if errMsg == "" {
			errMsg = strings.TrimSpace(legacyErr.Message)
		}
		if errMsg == "" {
			errMsg = strings.TrimSpace(legacyErr.Error)
		}
		if errMsg == "" {
			errMsg = "server error"
		}
		resp.Error = fmt.Errorf("%s", errMsg)
	}
	return &resp, nil
}

func parseLegacyImageResponse(msg []byte) (*ImagineResponse, bool) {
	var legacy struct {
		RequestID string `json:"requestId"`
		URL       string `json:"url"`
		Blob      string `json:"blob"`
	}
	if err := json.Unmarshal(msg, &legacy); err != nil {
		return nil, false
	}
	imageData := normalizeLegacyBlob(legacy.Blob)
	if imageData == "" {
		return nil, false
	}
	reqID := strings.TrimSpace(legacy.RequestID)
	if reqID == "" {
		reqID = parseLegacyImageID(legacy.URL)
	}
	stage := classifyLegacyImageStage(imageData)
	return &ImagineResponse{
		Type: stage,
		Item: ImagineResponseItem{
			RequestID: reqID,
			ImageData: imageData,
		},
	}, true
}

func normalizeLegacyBlob(blob string) string {
	if blob == "" {
		return ""
	}
	if idx := strings.Index(blob, ","); idx >= 0 && strings.Contains(blob[:idx], "base64") {
		return strings.TrimSpace(blob[idx+1:])
	}
	return strings.TrimSpace(blob)
}

func parseLegacyImageID(rawURL string) string {
	match := imagineImageURLPattern.FindStringSubmatch(rawURL)
	if len(match) == 3 {
		return match[1]
	}
	return ""
}

func classifyLegacyImageStage(imageData string) string {
	size := len(imageData)
	if size >= legacyFinalMinBytes {
		return "image.final"
	}
	if size > legacyMediumMinBytes {
		return "image.medium"
	}
	return "image.preview"
}

func buildImagineRequest(requestID, prompt, aspectRatio string, enableNSFW, enablePro bool) *ImagineRequest {
	props := ImagineRequestProperties{
		SectionCount:     0,
		IsKidsMode:       false,
		EnableNSFW:       enableNSFW,
		SkipUpsampler:    false,
		IsInitial:        false,
		AspectRatio:      aspectRatio,
		EnableSideBySide: true,
		EnablePro:        enablePro,
	}

	return &ImagineRequest{
		Type:      "conversation.item.create",
		Timestamp: time.Now().UnixMilli(),
		Item: ImagineRequestItem{
			Type: "message",
			Content: []ImagineRequestContent{
				{
					RequestID:  requestID,
					Text:       prompt,
					Type:       "input_text",
					Properties: props,
				},
			},
		},
	}
}

// ParseAspectRatio converts OpenAI size format to Grok aspect ratio.
func ParseAspectRatio(size string) string {
	switch size {
	case "1280x720":
		return "16:9"
	case "720x1280":
		return "9:16"
	case "1792x1024":
		return "3:2"
	case "1024x1792":
		return "2:3"
	case "1024x1024":
		return "1:1"
	default:
		return "2:3"
	}
}
