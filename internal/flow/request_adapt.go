package flow

import (
	"fmt"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/upstream"
)

func (f *ChatFlow) resolveUpstream(req *ChatRequest) (upstream.Upstream, string, error) {
	name := req.UpstreamName
	if name == "" {
		name = "grok"
	}
	us := f.upstreams[name]
	if us == nil {
		return nil, name, fmt.Errorf("upstream %q not registered", name)
	}
	return us, name, nil
}

func toUpstreamRequest(req *ChatRequest, appCfg *config.AppConfig) *upstream.ChatRequest {
	model := req.UpstreamModel
	if model == "" {
		model = req.Model
	}
	effort := req.ReasoningEffort
	if req.ForceThinking && effort == "" {
		effort = "high"
	}
	out := &upstream.ChatRequest{
		Messages:                req.Messages,
		Model:                   model,
		Temperature:             req.Temperature,
		TopP:                    req.TopP,
		MaxTokens:               req.MaxTokens,
		ReasoningEffort:         effort,
		Tools:                   req.Tools,
		ToolChoice:              req.ToolChoice,
		ParallelToolCalls:       req.ParallelToolCalls,
		UpstreamMode:            req.UpstreamMode,
		DeepSearch:              req.DeepSearch,
		WebSearch:               req.ConsoleWebSearch,
		SupportsReasoningEffort: req.ConsoleSupportsReasoningEffort,
	}
	if appCfg != nil {
		out.Temporary = appCfg.Temporary
		out.DisableMemory = appCfg.DisableMemory
		out.CustomInstruction = appCfg.CustomInstruction
	}
	return out
}
