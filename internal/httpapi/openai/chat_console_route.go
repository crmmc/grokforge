package openai

import (
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
)

type chatRoutePlan struct {
	Model                          string
	UseConsole                     bool
	UpstreamModel                  string
	UpstreamMode                   string
	Mode                           string
	PoolFloor                      string
	ConsoleSupportsReasoningEffort bool
	ConsoleWebSearch               bool
}

func planChatRoute(rm *registry.ResolvedModel, cfg *config.Config) chatRoutePlan {
	if rm == nil {
		return chatRoutePlan{}
	}

	plan := chatRoutePlan{
		Model:         rm.ID,
		UpstreamModel: rm.UpstreamModel,
		UpstreamMode:  rm.UpstreamMode,
		Mode:          rm.Mode,
		PoolFloor:     rm.PoolFloor,
	}

	if cfg != nil && cfg.Console.Enabled && rm.Type == modelconfig.TypeChat && rm.ConsoleUpstreamModel != "" {
		plan.UseConsole = true
		plan.UpstreamModel = rm.ConsoleUpstreamModel
		plan.UpstreamMode = ""
		plan.Mode = rm.ConsoleMode
		plan.PoolFloor = rm.ConsolePoolFloor
		plan.ConsoleSupportsReasoningEffort = rm.ConsoleSupportsReasoningEffort
		plan.ConsoleWebSearch = cfg.Console.WebSearch
	}

	return plan
}
