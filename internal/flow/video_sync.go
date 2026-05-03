package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

var (
	errVideoClientNil       = errors.New("video client is nil")
	videoGeneratedIDPattern = regexp.MustCompile(`/generated/([0-9a-fA-F-]{32,36})/`)
)

type videoStreamState struct {
	videoURL  string
	moderated bool
}

func (f *VideoFlow) generateVideoViaChat(ctx context.Context, tok *store.Token, req *VideoRequest, mode string) (string, error) {
	client := f.clientFactory(tok.Token)
	if client == nil {
		return "", errVideoClientNil
	}

	parentPostID, imageURLs, err := f.resolveVideoSeedPost(ctx, client, req)
	if err != nil {
		return "", err
	}

	eventCh, err := client.Chat(ctx, f.buildVideoChatRequest(req, parentPostID, imageURLs, videoGenerationResolution(tok.Pool, req.Quality)))
	if err != nil {
		return "", fmt.Errorf("start video generation: %w", err)
	}
	result, err := collectVideoStreamState(eventCh)
	if err != nil {
		return "", err
	}
	if result.moderated {
		return "", errors.New("video generation moderated by upstream")
	}
	if strings.TrimSpace(result.videoURL) == "" {
		return "", errors.New("video generation missing final url")
	}

	videoURL := result.videoURL
	if shouldUpscaleVideo(tok.Pool, videoResolutionFromQuality(req.Quality)) {
		videoURL, err = f.upscaleVideoURL(ctx, client, videoURL)
		if err != nil {
			return "", err
		}
	}

	return f.cacheVideo(ctx, client, videoURL)
}

func (f *VideoFlow) resolveVideoSeedPost(
	ctx context.Context,
	client VideoClient,
	req *VideoRequest,
) (string, []string, error) {
	if len(req.ReferenceImages) == 0 {
		postID, err := client.CreateVideoPost(ctx, req.Prompt)
		return postID, nil, err
	}

	var parentPostID string
	imageURLs := make([]string, 0, len(req.ReferenceImages))
	for i, img := range req.ReferenceImages {
		mimeType := detectImageEditMIME(img)
		fileName := fmt.Sprintf("video-reference-%d%s", i, extensionForMIME(mimeType))
		content := base64.StdEncoding.EncodeToString(img)
		_, fileURI, err := client.UploadFile(ctx, fileName, mimeType, content)
		if err != nil {
			return "", nil, fmt.Errorf("upload video reference %d: %w", i, err)
		}
		contentURL := normalizeUploadedImageURL(fileURI)
		imageURLs = append(imageURLs, contentURL)

		postID, err := client.CreateImagePost(ctx, contentURL)
		if err != nil {
			return "", nil, fmt.Errorf("create video reference post %d: %w", i, err)
		}
		if i == 0 {
			parentPostID = postID
		}
	}
	return parentPostID, imageURLs, nil
}

func (f *VideoFlow) buildVideoChatRequest(req *VideoRequest, parentPostID string, imageURLs []string, resolution string) *xai.ChatRequest {
	videoConfig := map[string]any{
		"aspectRatio":    resolveVideoAspectRatio(req.AspectRatio, req.Size),
		"parentPostId":   parentPostID,
		"resolutionName": resolution,
		"videoLength":    req.Seconds,
	}
	if len(imageURLs) > 0 {
		videoConfig["imageReferences"] = imageURLs
		videoConfig["isReferenceToVideo"] = true
		videoConfig["isVideoEdit"] = false
	}

	xaiReq := &xai.ChatRequest{
		Messages: []xai.Message{{
			Role:    "user",
			Content: buildVideoModePrompt(req.Prompt, req.Preset),
		}},
		Model:         req.Model,
		Stream:        true,
		ToolOverrides: map[string]any{"videoGen": true},
		UpstreamModel: req.UpstreamModel,
		UpstreamMode:  req.UpstreamMode,
		ModelConfig: map[string]any{
			"modelMap": map[string]any{
				"videoGenModelConfig": videoConfig,
			},
		},
	}
	f.applyVideoAppConfig(xaiReq)
	return xaiReq
}

func (f *VideoFlow) applyVideoAppConfig(req *xai.ChatRequest) {
	appCfg := f.appConfig()
	if appCfg == nil {
		return
	}
	req.Temporary = appCfg.Temporary
	req.DisableMemory = appCfg.DisableMemory
	req.CustomInstruction = appCfg.CustomInstruction
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

	// Check moderated flag (grok2api: moderated → treat as failure)
	if moderated, ok := response["moderated"].(bool); ok && moderated {
		state.moderated = true
	}

	if videoResponse, ok := response["streamingVideoGenerationResponse"].(map[string]any); ok {
		if url, ok := videoResponse["videoUrl"].(string); ok && strings.TrimSpace(url) != "" {
			state.videoURL = strings.TrimSpace(url)
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

func (f *VideoFlow) upscaleVideoURL(ctx context.Context, client VideoClient, videoURL string) (string, error) {
	videoID := extractGeneratedVideoID(videoURL)
	if videoID == "" {
		return "", fmt.Errorf("%w: missing generated video id", ErrVideoPostProcess)
	}

	interval := time.Duration(f.cfg.PollIntervalSeconds) * time.Second
	upscaledURL, err := client.PollUpscale(ctx, videoID, interval)
	if err != nil {
		return "", fmt.Errorf("%w: upscale video: %w", ErrVideoPostProcess, err)
	}
	if strings.TrimSpace(upscaledURL) == "" {
		return "", fmt.Errorf("%w: upscale video returned empty url", ErrVideoPostProcess)
	}
	return upscaledURL, nil
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
