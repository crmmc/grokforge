package openai

import (
	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/registry"
)

// Handler holds dependencies for OpenAI-compatible API endpoints.
type Handler struct {
	ChatFlow      *flow.ChatFlow
	VideoFlow     *flow.VideoFlow
	ImageFlow     *flow.ImageFlow
	CacheService  *cache.Service
	Cfg           *config.Config
	Runtime       *config.Runtime
	ModelRegistry *registry.ModelRegistry
}

func (h *Handler) currentConfig() *config.Config {
	if h == nil {
		return nil
	}
	if h.Runtime != nil {
		return h.Runtime.Get()
	}
	return h.Cfg
}

func (h *Handler) imageOutputFormat() string {
	cfg := h.currentConfig()
	if cfg == nil {
		return config.ImageFormatBase64
	}
	return config.EffectiveImageFormat(&cfg.Image)
}
