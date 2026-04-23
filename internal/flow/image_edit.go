package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

const (
	imageEditReferenceLimit = 3
	imageEditBatchSize      = 2
	imageEditModelKind      = "imagine"
)

var (
	imageEditGeneratedIDPattern = regexp.MustCompile(`/generated/([a-f0-9-]+)/`)
	imageEditUserIDPattern      = regexp.MustCompile(`/users/[^/]+/([a-f0-9-]+)/content`)
)

// ImageEditClient defines the upstream calls required by the image edit chain.
type ImageEditClient interface {
	Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error)
	UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error)
	CreateImagePost(ctx context.Context, imageURL string) (string, error)
	DownloadURL(ctx context.Context, url string) ([]byte, error)
}

// ImageEditClientFactory creates image edit clients for a token.
type ImageEditClientFactory func(token string) ImageEditClient

// Edit edits images through the same upload + app-chat path used by grok2api.
func (f *ImageFlow) Edit(ctx context.Context, req *ImageEditRequest) (*ImageResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	if err := validateMediaUpstream(req.UpstreamModel, req.UpstreamMode); err != nil {
		return nil, err
	}
	if f.editClientFactory == nil {
		return nil, errors.New("image edit client not configured")
	}

	// Resolve mode from request or model registry
	mode := req.Mode
	if mode == "" {
		mode = f.resolveMode(req.Model)
	}

	start, apiKeyID := time.Now(), FlowAPIKeyIDFromContext(ctx)

	tok, err := f.pickTokenForModel(req.Model, mode)
	if err != nil {
		return nil, err
	}

	client := f.editClientFactory(tok.Token)
	if client == nil {
		return nil, errors.New("image edit client is nil")
	}

	handleErr := func(err error) (*ImageResponse, error) {
		reportTrackedTokenError(f.tokenSvc, tok.ID, mode, err)
		f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
		return nil, err
	}

	uploadedURLs, err := uploadImageEditReferences(ctx, client, lastImageEditReferences(req.OriginalImages))
	if err != nil {
		return handleErr(err)
	}
	f.tokenSvc.RecordFirstUse(tok.ID, mode)

	parentPostID, err := resolveImageEditParentPostID(ctx, client, uploadedURLs)
	if err != nil {
		return handleErr(err)
	}

	images, err := f.collectEditedImages(ctx, client, tok.ID, req, uploadedURLs, parentPostID)
	if err != nil {
		return handleErr(err)
	}

	f.tokenSvc.ReportSuccess(tok.ID)
	f.recordUsage(apiKeyID, tok.ID, req.Model, 200, time.Since(start))

	return &ImageResponse{
		Created: time.Now().Unix(),
		Data:    images,
	}, nil
}

func (f *ImageFlow) collectEditedImages(
	ctx context.Context,
	client ImageEditClient,
	tokenID uint,
	req *ImageEditRequest,
	imageURLs []string,
	parentPostID string,
) ([]ImageData, error) {
	callsNeeded := (req.N + imageEditBatchSize - 1) / imageEditBatchSize
	results := make([]ImageData, 0, req.N)
	seen := make(map[string]struct{})

	for callIndex := 0; callIndex < callsNeeded && len(results) < req.N; callIndex++ {
		eventCh, err := client.Chat(ctx, f.buildImageEditChatRequest(req, imageURLs, parentPostID))
		if err != nil {
			return nil, fmt.Errorf("start image edit: %w", err)
		}

		urls, err := collectImageEditURLs(eventCh)
		if err != nil {
			return nil, err
		}

		for _, rawURL := range urls {
			if _, ok := seen[rawURL]; ok {
				continue
			}
			seen[rawURL] = struct{}{}

			imageData, err := resolveImageEditOutput(ctx, client, req.ResponseFormat, req.Prompt, rawURL)
			if err != nil {
				return nil, err
			}
			results = append(results, imageData)
			if len(results) >= req.N {
				break
			}
		}
	}

	if len(results) == 0 {
		return nil, errors.New("no edited images received")
	}
	return results, nil
}

func (f *ImageFlow) buildImageEditChatRequest(
	req *ImageEditRequest,
	imageURLs []string,
	parentPostID string,
) *xai.ChatRequest {
	imageEditConfig := map[string]any{
		"imageReferences": imageURLs,
	}
	if parentPostID != "" {
		imageEditConfig["parentPostId"] = parentPostID
	}

	xaiReq := &xai.ChatRequest{
		Messages: []xai.Message{{
			Role:    "user",
			Content: req.Prompt,
		}},
		Model:         req.Model,
		Stream:        true,
		IsImageEdit:   true,
		ToolOverrides: map[string]any{"imageGen": true},
		UpstreamModel: req.UpstreamModel,
		UpstreamMode:  req.UpstreamMode,
		ModelConfig: map[string]any{
			"modelMap": map[string]any{
				"imageEditModel":       imageEditModelKind,
				"imageEditModelConfig": imageEditConfig,
			},
		},
	}

	if appCfg := f.appConfig(); appCfg != nil {
		xaiReq.Temporary = appCfg.Temporary
		xaiReq.DisableMemory = appCfg.DisableMemory
		xaiReq.CustomInstruction = appCfg.CustomInstruction
	}
	return xaiReq
}

func uploadImageEditReferences(
	ctx context.Context,
	client ImageEditClient,
	images [][]byte,
) ([]string, error) {
	urls := make([]string, 0, len(images))
	for index, imageData := range images {
		mimeType := detectImageEditMIME(imageData)
		fileName := fmt.Sprintf("image-%d%s", index, extensionForMIME(mimeType))
		content := base64.StdEncoding.EncodeToString(imageData)
		_, fileURI, err := client.UploadFile(ctx, fileName, mimeType, content)
		if err != nil {
			return nil, fmt.Errorf("upload image reference: %w", err)
		}
		urls = append(urls, normalizeUploadedImageURL(fileURI))
	}
	if len(urls) == 0 {
		return nil, errors.New("image upload failed")
	}
	return urls, nil
}

func resolveImageEditParentPostID(
	ctx context.Context,
	client ImageEditClient,
	imageURLs []string,
) (string, error) {
	if len(imageURLs) == 0 {
		return "", errors.New("image references are required")
	}

	postID, err := client.CreateImagePost(ctx, imageURLs[0])
	if err == nil && strings.TrimSpace(postID) != "" {
		return postID, nil
	}

	for _, imageURL := range imageURLs {
		if matches := imageEditGeneratedIDPattern.FindStringSubmatch(imageURL); len(matches) == 2 {
			return matches[1], nil
		}
		if matches := imageEditUserIDPattern.FindStringSubmatch(imageURL); len(matches) == 2 {
			return matches[1], nil
		}
	}
	return "", nil
}

func collectImageEditURLs(eventCh <-chan xai.StreamEvent) ([]string, error) {
	urls := make([]string, 0)
	seen := make(map[string]struct{})

	for event := range eventCh {
		if event.Error != nil {
			return nil, fmt.Errorf("image edit stream: %w", event.Error)
		}

		found, err := extractImageEditURLs(event.Data)
		if err != nil {
			return nil, err
		}
		for _, rawURL := range found {
			if _, ok := seen[rawURL]; ok {
				continue
			}
			seen[rawURL] = struct{}{}
			urls = append(urls, rawURL)
		}
	}
	return urls, nil
}

func extractImageEditURLs(data json.RawMessage) ([]string, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode image edit stream: %w", err)
	}
	return collectImageURLs(payload), nil
}

func collectImageURLs(payload any) []string {
	urls := make([]string, 0)
	seen := make(map[string]struct{})

	var walk func(value any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				if key == "generatedImageUrls" || key == "imageUrls" || key == "imageURLs" {
					switch candidate := item.(type) {
					case string:
						appendImageURL(&urls, seen, candidate)
					case []any:
						for _, entry := range candidate {
							if url, ok := entry.(string); ok {
								appendImageURL(&urls, seen, url)
							}
						}
					}
					continue
				}
				walk(item)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}

	walk(payload)
	return urls
}

func resolveImageEditOutput(
	ctx context.Context,
	client ImageEditClient,
	responseFormat string,
	prompt string,
	rawURL string,
) (ImageData, error) {
	if responseFormat == "url" {
		return ImageData{URL: rawURL, RevisedPrompt: prompt}, nil
	}

	content, err := client.DownloadURL(ctx, rawURL)
	if err != nil {
		return ImageData{}, fmt.Errorf("download edited image: %w", err)
	}

	return ImageData{
		B64JSON:       base64.StdEncoding.EncodeToString(content),
		RevisedPrompt: prompt,
	}, nil
}

func appendImageURL(urls *[]string, seen map[string]struct{}, rawURL string) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return
	}
	if _, ok := seen[trimmed]; ok {
		return
	}
	seen[trimmed] = struct{}{}
	*urls = append(*urls, trimmed)
}

func normalizeUploadedImageURL(fileURI string) string {
	trimmed := strings.TrimSpace(fileURI)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return "https://assets.grok.com/" + strings.TrimLeft(trimmed, "/")
}

func detectImageEditMIME(data []byte) string {
	mimeType := http.DetectContentType(data)
	if strings.TrimSpace(mimeType) == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func extensionForMIME(mimeType string) string {
	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ".bin"
	}
	return filepath.Ext(exts[0])
}

func lastImageEditReferences(images [][]byte) [][]byte {
	if len(images) <= imageEditReferenceLimit {
		return images
	}
	return images[len(images)-imageEditReferenceLimit:]
}
