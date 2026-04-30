package cache

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Whitelisted extensions per media type.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".bmp": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".m4v": true,
	".webm": true, ".avi": true, ".mkv": true,
}

// exts returns the extension whitelist for the given media type.
func exts(mediaType string) map[string]bool {
	switch mediaType {
	case "image":
		return imageExts
	case "video":
		return videoExts
	default:
		return nil
	}
}

// isWhitelisted checks if the file extension is allowed for the media type.
func isWhitelisted(name, mediaType string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	wl := exts(mediaType)
	return wl != nil && wl[ext]
}

func isCacheFile(name, mediaType string) bool {
	return !strings.HasPrefix(name, ".tmp-") && isWhitelisted(name, mediaType)
}

// validateName ensures the filename is safe and has an allowed extension.
func validateName(name, mediaType string) error {
	if name == "" {
		return errors.New("empty filename")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid filename: contains path separator")
	}
	cleaned := filepath.Base(name)
	if cleaned != name || cleaned == "." || cleaned == ".." {
		return fmt.Errorf("invalid filename: %q", name)
	}
	if !isWhitelisted(name, mediaType) {
		return fmt.Errorf("invalid extension for %s: %q", mediaType, name)
	}
	return nil
}
