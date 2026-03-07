package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

const (
	videoResolutionStandard = "480p"
	videoResolutionHigh     = "720p"
)

var videoGeneratedIDPattern = regexp.MustCompile(`/generated/([0-9a-fA-F-]{32,36})/`)

type videoStreamState struct {
	postID      string
	videoURL    string
	streamError []string
}

func (f *VideoFlow) generateVideoViaChat(ctx context.Context, tok *store.Token, req *VideoRequest) (string, error) {
	client := f.clientFactory(tok.Token)
	if client == nil {
		return "", errors.New("video client is nil")
	}

	parentPostID, err := f.resolveVideoSeedPost(ctx, client, req)
	if err != nil {
		return "", err
	}

	eventCh, err := client.Chat(ctx, f.buildVideoChatRequest(req, parentPostID, videoGenerationResolution(tok.Pool, req.Quality)))
	if err != nil {
		return "", fmt.Errorf("start video generation: %w", err)
	}

	result, err := collectVideoStreamState(eventCh)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.videoURL) == "" {
		return "", fmt.Errorf("video generation missing final url: %s", strings.Join(result.streamError, "; "))
	}

	videoURL := result.videoURL
	if shouldUpscaleVideo(tok.Pool, videoResolutionFromQuality(req.Quality)) {
		videoURL = f.upscaleVideoURL(ctx, client, videoURL)
	}

	return f.cacheVideo(ctx, client, videoURL), nil
}

func (f *VideoFlow) resolveVideoSeedPost(
	ctx context.Context,
	client VideoClient,
	req *VideoRequest,
) (string, error) {
	if len(req.ReferenceImage) == 0 {
		return client.CreateVideoPost(ctx, req.Prompt)
	}

	mimeType := detectImageEditMIME(req.ReferenceImage)
	content := base64.StdEncoding.EncodeToString(req.ReferenceImage)
	fileName := "video-reference" + extensionForMIME(mimeType)
	_, fileURI, err := client.UploadFile(ctx, fileName, mimeType, content)
	if err != nil {
		return "", fmt.Errorf("upload video reference: %w", err)
	}
	return client.CreateImagePost(ctx, normalizeUploadedImageURL(fileURI))
}

func (f *VideoFlow) buildVideoChatRequest(req *VideoRequest, parentPostID, resolution string) *xai.ChatRequest {
	xaiReq := &xai.ChatRequest{
		Messages: []xai.Message{{
			Role:    "user",
			Content: buildVideoModePrompt(req.Prompt, req.Preset),
		}},
		Model:         req.Model,
		Stream:        true,
		ToolOverrides: map[string]any{"videoGen": true},
		ModelConfig: map[string]any{
			"modelMap": map[string]any{
				"videoGenModelConfig": map[string]any{
					"aspectRatio":    resolveVideoAspectRatio(req.AspectRatio, req.Size),
					"parentPostId":   parentPostID,
					"resolutionName": resolution,
					"videoLength":    req.Seconds,
				},
			},
		},
	}
	f.applyVideoAppConfig(xaiReq)
	return xaiReq
}

func (f *VideoFlow) applyVideoAppConfig(req *xai.ChatRequest) {
	if f.appCfg == nil {
		return
	}
	req.Temporary = f.appCfg.Temporary
	req.DisableMemory = f.appCfg.DisableMemory
	req.CustomInstruction = f.appCfg.CustomInstruction
}

func collectVideoStreamState(eventCh <-chan xai.StreamEvent) (*videoStreamState, error) {
	state := &videoStreamState{}

	for event := range eventCh {
		if event.Error != nil {
			return nil, fmt.Errorf("video stream: %w", event.Error)
		}
		if err := updateVideoStreamState(state, event.Data); err != nil {
			return nil, err
		}
	}

	return state, nil
}

func updateVideoStreamState(state *videoStreamState, data json.RawMessage) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode video stream: %w", err)
	}

	response, ok := extractVideoResponse(payload)
	if !ok {
		return nil
	}

	appendVideoStreamErrors(&state.streamError, response["streamErrors"])
	if modelResponse, ok := response["modelResponse"].(map[string]any); ok {
		appendVideoStreamErrors(&state.streamError, modelResponse["streamErrors"])
		if state.postID == "" {
			if fileAttachments, ok := modelResponse["fileAttachments"].([]any); ok && len(fileAttachments) > 0 {
				if first, ok := fileAttachments[0].(string); ok {
					state.postID = strings.TrimSpace(first)
				}
			}
		}
	}

	if videoResponse, ok := response["streamingVideoGenerationResponse"].(map[string]any); ok {
		if url, ok := videoResponse["videoUrl"].(string); ok && strings.TrimSpace(url) != "" {
			state.videoURL = strings.TrimSpace(url)
		}
		if state.postID == "" {
			if postID, ok := videoResponse["videoPostId"].(string); ok && strings.TrimSpace(postID) != "" {
				state.postID = strings.TrimSpace(postID)
			}
		}
		if state.postID == "" {
			if postID, ok := videoResponse["postId"].(string); ok && strings.TrimSpace(postID) != "" {
				state.postID = strings.TrimSpace(postID)
			}
		}
	}

	if state.postID == "" {
		if post, ok := response["post"].(map[string]any); ok {
			if postID, ok := post["id"].(string); ok && strings.TrimSpace(postID) != "" {
				state.postID = strings.TrimSpace(postID)
			}
		}
	}

	return nil
}

func extractVideoResponse(payload map[string]any) (map[string]any, bool) {
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return nil, false
	}
	response, ok := result["response"].(map[string]any)
	return response, ok
}

func appendVideoStreamErrors(target *[]string, raw any) {
	switch typed := raw.(type) {
	case string:
		appendVideoStreamError(target, typed)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				appendVideoStreamError(target, text)
			}
		}
	}
}

func appendVideoStreamError(target *[]string, text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	for _, existing := range *target {
		if existing == trimmed {
			return
		}
	}
	*target = append(*target, trimmed)
}

func buildVideoModePrompt(prompt, preset string) string {
	return strings.TrimSpace(prompt + " " + videoModeFlag(preset))
}

func videoResolutionFromQuality(quality string) string {
	if strings.EqualFold(strings.TrimSpace(quality), "high") {
		return videoResolutionHigh
	}
	return videoResolutionStandard
}

func videoModeFlag(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "fun":
		return "--mode=extremely-crazy"
	case "normal":
		return "--mode=normal"
	case "spicy":
		return "--mode=extremely-spicy-or-crazy"
	default:
		return "--mode=custom"
	}
}

func shouldUpscaleVideo(pool, requested string) bool {
	return pool == tkn.PoolBasic && strings.EqualFold(strings.TrimSpace(requested), videoResolutionHigh)
}

func videoGenerationResolution(pool, quality string) string {
	if shouldUpscaleVideo(pool, videoResolutionFromQuality(quality)) {
		return videoResolutionStandard
	}
	return videoResolutionFromQuality(quality)
}

func (f *VideoFlow) upscaleVideoURL(ctx context.Context, client VideoClient, videoURL string) string {
	videoID := extractGeneratedVideoID(videoURL)
	if videoID == "" {
		slog.Warn("video: skip upscale, missing generated id", "url", videoURL)
		return videoURL
	}

	interval := time.Duration(f.cfg.PollIntervalSeconds) * time.Second
	upscaledURL, err := client.PollUpscale(ctx, videoID, interval)
	if err != nil || strings.TrimSpace(upscaledURL) == "" {
		slog.Warn("video: upscale failed, fallback to original", "error", err, "video_id", videoID)
		return videoURL
	}
	return upscaledURL
}

func extractGeneratedVideoID(videoURL string) string {
	matches := videoGeneratedIDPattern.FindStringSubmatch(strings.TrimSpace(videoURL))
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func resolveVideoAspectRatio(aspectRatio, size string) string {
	if ar := strings.TrimSpace(aspectRatio); ar != "" {
		return ar
	}
	return xai.ParseAspectRatio(size)
}
