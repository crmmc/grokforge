package openai

import (
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
)

func TestPlanChatRoute_DefaultGrokWeb(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Console.Enabled = true
	rm := &registry.ResolvedModel{
		ID:            "grok-4.20",
		Type:          modelconfig.TypeChat,
		UpstreamModel: "",
		UpstreamMode:  "auto",
		Mode:          "auto",
		PoolFloor:     modelconfig.PoolSuper,
	}

	plan := planChatRoute(rm, cfg)

	if plan.UpstreamName != "grok" {
		t.Fatalf("UpstreamName = %q, want grok for unmapped models", plan.UpstreamName)
	}
	if plan.Model != rm.ID || plan.UpstreamModel != rm.UpstreamModel || plan.UpstreamMode != rm.UpstreamMode || plan.Mode != rm.Mode || plan.PoolFloor != rm.PoolFloor {
		t.Fatalf("plan = %#v, want web route values from resolved model", plan)
	}
	if plan.ConsoleSupportsReasoningEffort || plan.ConsoleWebSearch {
		t.Fatalf("console-only flags should be false for web route: %#v", plan)
	}
}

func TestPlanChatRoute_ConsoleWhenEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Console.Enabled = true
	cfg.Console.WebSearch = true
	rm := &registry.ResolvedModel{
		ID:                             "grok-4.20",
		Type:                           modelconfig.TypeChat,
		UpstreamModel:                  "",
		UpstreamMode:                   "auto",
		Mode:                           "auto",
		PoolFloor:                      modelconfig.PoolSuper,
		ConsoleUpstreamModel:           "grok-4.20",
		ConsoleMode:                    "console",
		ConsolePoolFloor:               modelconfig.PoolBasic,
		ConsoleSupportsReasoningEffort: true,
	}

	plan := planChatRoute(rm, cfg)

	if plan.UpstreamName != "console" {
		t.Fatalf("UpstreamName = %q, want console", plan.UpstreamName)
	}
	if plan.Model != rm.ID || plan.UpstreamModel != "grok-4.20" || plan.UpstreamMode != "" || plan.Mode != "console" || plan.PoolFloor != modelconfig.PoolBasic {
		t.Fatalf("plan = %#v, want console route values", plan)
	}
	if !plan.ConsoleSupportsReasoningEffort || !plan.ConsoleWebSearch {
		t.Fatalf("console flags not propagated: %#v", plan)
	}
}

func TestPlanChatRoute_ConsoleDisabledUsesWeb(t *testing.T) {
	cfg := config.DefaultConfig()
	rm := &registry.ResolvedModel{
		ID:                   "grok-4.20",
		Type:                 modelconfig.TypeChat,
		UpstreamMode:         "auto",
		Mode:                 "auto",
		PoolFloor:            modelconfig.PoolSuper,
		ConsoleUpstreamModel: "grok-4.20",
		ConsoleMode:          "console",
		ConsolePoolFloor:     modelconfig.PoolBasic,
	}

	plan := planChatRoute(rm, cfg)

	if plan.UpstreamName != "grok" {
		t.Fatalf("UpstreamName = %q, want grok when console config is disabled", plan.UpstreamName)
	}
	if plan.UpstreamMode != rm.UpstreamMode || plan.Mode != rm.Mode || plan.PoolFloor != rm.PoolFloor {
		t.Fatalf("plan = %#v, want web route values", plan)
	}
}

func TestPlanChatRoute_ConsoleRequiresChatType(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Console.Enabled = true
	rm := &registry.ResolvedModel{
		ID:                   "grok-imagine-video",
		Type:                 modelconfig.TypeVideo,
		UpstreamModel:        "grok-3",
		UpstreamMode:         "auto",
		Mode:                 "auto",
		PoolFloor:            modelconfig.PoolSuper,
		ConsoleUpstreamModel: "grok-4.20",
		ConsoleMode:          "console",
		ConsolePoolFloor:     modelconfig.PoolBasic,
	}

	plan := planChatRoute(rm, cfg)

	if plan.UpstreamName != "grok" {
		t.Fatalf("UpstreamName = %q, want grok for non-chat models", plan.UpstreamName)
	}
	if plan.UpstreamModel != rm.UpstreamModel || plan.UpstreamMode != rm.UpstreamMode || plan.Mode != rm.Mode || plan.PoolFloor != rm.PoolFloor {
		t.Fatalf("plan = %#v, want web route values", plan)
	}
}
