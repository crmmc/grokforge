package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExts(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		want      map[string]bool
	}{
		{name: "image whitelist", mediaType: "image", want: imageExts},
		{name: "video whitelist", mediaType: "video", want: videoExts},
		{name: "unknown media type has no whitelist", mediaType: "audio", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, exts(tc.mediaType))
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		mediaType string
		wantErr   bool
	}{
		{name: "accepts image extension", filename: "a.png", mediaType: "image"},
		{name: "accepts uppercase extension", filename: "a.PNG", mediaType: "image"},
		{name: "accepts video extension", filename: "clip.mp4", mediaType: "video"},
		{name: "rejects empty name", filename: "", mediaType: "image", wantErr: true},
		{name: "rejects slash separator", filename: "sub/a.jpg", mediaType: "image", wantErr: true},
		{name: "rejects backslash separator", filename: `sub\a.jpg`, mediaType: "image", wantErr: true},
		{name: "rejects dot", filename: ".", mediaType: "image", wantErr: true},
		{name: "rejects dot dot", filename: "..", mediaType: "image", wantErr: true},
		{name: "rejects relative path", filename: "./a.jpg", mediaType: "image", wantErr: true},
		{name: "rejects parent traversal", filename: "../a.jpg", mediaType: "image", wantErr: true},
		{name: "rejects disallowed extension", filename: "a.txt", mediaType: "image", wantErr: true},
		{name: "rejects extension of another media type", filename: "a.mp4", mediaType: "image", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateName(tc.filename, tc.mediaType)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestIsWhitelisted(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		mediaType string
		want      bool
	}{
		{name: "image extension", filename: "x.jpg", mediaType: "image", want: true},
		{name: "case insensitive extension", filename: "x.WEBP", mediaType: "image", want: true},
		{name: "video extension", filename: "x.mkv", mediaType: "video", want: true},
		{name: "non whitelisted extension", filename: "x.txt", mediaType: "image", want: false},
		{name: "unknown media type", filename: "x.jpg", mediaType: "audio", want: false},
		{name: "missing extension", filename: "x", mediaType: "image", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isWhitelisted(tc.filename, tc.mediaType))
		})
	}
}
