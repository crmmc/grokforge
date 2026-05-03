package flow

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/crmmc/grokforge/internal/config"
)

const (
	imageFormatBase64   = "base64"
	imageFormatLocalURL = "local_url"
)

type imageOutputInput struct {
	B64JSON  string       // WS path: already have base64 blob
	RawURL   string       // lite/edit path: upstream CDN URL
	Prompt   string       // RevisedPrompt
	Download DownloadFunc // authenticated download function
}

func (f *ImageFlow) resolveImageOutput(ctx context.Context, in imageOutputInput) (ImageData, error) {
	format := f.imageFormat()
	switch format {
	case imageFormatBase64:
		return f.resolveBase64(ctx, in)
	case imageFormatLocalURL:
		return f.resolveLocalURL(ctx, in)
	default:
		return ImageData{}, fmt.Errorf("image output: unsupported format %q", format)
	}
}

func (f *ImageFlow) imageFormat() string {
	return config.EffectiveImageFormat(f.imageConfig())
}

func (f *ImageFlow) resolveBase64(ctx context.Context, in imageOutputInput) (ImageData, error) {
	if in.B64JSON != "" {
		return ImageData{B64JSON: in.B64JSON, RevisedPrompt: in.Prompt}, nil
	}
	if in.RawURL == "" {
		return ImageData{}, fmt.Errorf("image output: no data to encode")
	}
	if in.Download == nil {
		return ImageData{}, fmt.Errorf("image output: download not available")
	}
	imgBytes, err := in.Download(ctx, in.RawURL)
	if err != nil {
		return ImageData{}, fmt.Errorf("image output: download: %w", err)
	}
	return ImageData{B64JSON: base64.StdEncoding.EncodeToString(imgBytes), RevisedPrompt: in.Prompt}, nil
}

func (f *ImageFlow) resolveLocalURL(ctx context.Context, in imageOutputInput) (ImageData, error) {
	if f.cacheSvc == nil {
		return ImageData{}, fmt.Errorf("image output: local_url requires cache service")
	}
	var imgBytes []byte
	if in.B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(in.B64JSON)
		if err != nil {
			return ImageData{}, fmt.Errorf("image output: decode base64: %w", err)
		}
		imgBytes = decoded
	} else if in.RawURL != "" {
		if in.Download == nil {
			return ImageData{}, fmt.Errorf("image output: download not available")
		}
		downloaded, err := in.Download(ctx, in.RawURL)
		if err != nil {
			return ImageData{}, fmt.Errorf("image output: download: %w", err)
		}
		imgBytes = downloaded
	} else {
		return ImageData{}, fmt.Errorf("image output: no data to cache")
	}
	ext := detectImageExt(imgBytes)
	name, err := f.cacheSvc.SaveFile("image", imgBytes, ext)
	if err != nil {
		return ImageData{}, fmt.Errorf("image output: cache save: %w", err)
	}
	return ImageData{URL: "/api/files/image/" + name, RevisedPrompt: in.Prompt}, nil
}

func detectImageExt(data []byte) string {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}
