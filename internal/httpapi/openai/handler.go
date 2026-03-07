package openai

import (
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
)

// Handler holds dependencies for OpenAI-compatible API endpoints.
type Handler struct {
	ChatFlow  *flow.ChatFlow
	VideoFlow *flow.VideoFlow
	ImageFlow *flow.ImageFlow
	Cfg       *config.Config
}
