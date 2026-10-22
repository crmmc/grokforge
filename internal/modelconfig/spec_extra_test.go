package modelconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDerivePublicType(t *testing.T) {
	tests := []struct {
		name         string
		internalType string
		want         string
	}{
		{name: "chat stays chat", internalType: TypeChat, want: "chat"},
		{name: "image_ws stays image_ws", internalType: TypeImageWS, want: "image_ws"},
		{name: "image_lite becomes image", internalType: TypeImageLite, want: "image"},
		{name: "image_edit stays image_edit", internalType: TypeImageEdit, want: "image_edit"},
		{name: "video stays video", internalType: TypeVideo, want: "video"},
		{name: "unknown type passes through unchanged", internalType: "mystery", want: "mystery"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, DerivePublicType(tc.internalType))
		})
	}
}
