package flow

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

// ChatFlow orchestrates chat completion with retry logic.
type ChatFlow struct {
	tokenSvc         TokenServicer
	clientFactory    XAIClientFactory
	cfg              *ChatFlowConfig
	usageLog         UsageRecorder
	apiKeyUsageInc   func(ctx context.Context, apiKeyID uint)
	cfRefreshTrigger func() // called on 403 to trigger immediate CF refresh
}

// NewChatFlow creates a new chat flow orchestrator.
func NewChatFlow(tokenSvc TokenServicer, clientFactory XAIClientFactory, cfg *ChatFlowConfig) *ChatFlow {
	if cfg == nil {
		cfg = DefaultChatFlowConfig()
	}
	return &ChatFlow{
		tokenSvc:      tokenSvc,
		clientFactory: clientFactory,
		cfg:           cfg,
	}
}

// SetUsageRecorder sets the usage recorder for logging API usage.
func (f *ChatFlow) SetUsageRecorder(ur UsageRecorder) {
	f.usageLog = ur
}

// SetAPIKeyUsageInc sets the callback to increment API key daily usage on success.
func (f *ChatFlow) SetAPIKeyUsageInc(fn func(ctx context.Context, apiKeyID uint)) {
	f.apiKeyUsageInc = fn
}

// SetCFRefreshTrigger sets a callback invoked on 403 to trigger immediate CF cookie refresh.
func (f *ChatFlow) SetCFRefreshTrigger(fn func()) {
	f.cfRefreshTrigger = fn
}

// MapReasoningEffort maps OpenAI reasoning_effort to Grok thinking parameter.
func MapReasoningEffort(effort string) (grokThinking string, enabled bool) {
	switch effort {
	case "", "none":
		return "", false
	case "low":
		return "low", true
	case "medium":
		return "medium", true
	case "high":
		return "high", true
	default:
		return "medium", true // default to medium for unknown values
	}
}

// Complete executes a chat completion with retry logic.
// Returns a channel of StreamEvents. The channel is closed when done.
func (f *ChatFlow) Complete(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	if f.cfg == nil || f.cfg.ModelResolver == nil {
		outCh := make(chan StreamEvent, 1)
		outCh <- StreamEvent{Error: tkn.ErrModelNotFound}
		close(outCh)
		return outCh, nil
	}
	pools, ok := tkn.GetPoolForModel(req.Model, f.cfg.ModelResolver)
	if !ok {
		outCh := make(chan StreamEvent, 1)
		outCh <- StreamEvent{Error: tkn.ErrModelNotFound}
		close(outCh)
		return outCh, nil
	}
	slog.Debug("flow: chat complete start",
		"model", req.Model, "pools", pools,
		"msg_count", len(req.Messages), "stream", req.Stream,
		"has_tools", len(req.Tools) > 0)
	outCh := make(chan StreamEvent, 64)

	SafeGo("chat_execute_with_retry", func() {
		f.executeWithRetry(ctx, req, pools, outCh)
	})

	return outCh, nil
}

func (f *ChatFlow) executeWithRetry(ctx context.Context, req *ChatRequest, pools []string, outCh chan<- StreamEvent) {
	defer close(outCh)

	// Hot-reload: read config from provider if available
	cfg := f.cfg.RetryConfig
	if f.cfg.RetryConfigProvider != nil {
		cfg = f.cfg.RetryConfigProvider()
	}
	budgetDeadline := retryBudgetDeadline(cfg)

	apiKeyID := FlowAPIKeyIDFromContext(ctx)
	var lastErr error
	tokenRetries := 0
	var currentToken *store.Token
	var client xai.Client

	for attempt := 0; attempt < cfg.MaxTokens*cfg.PerTokenRetries; attempt++ {
		if retryBudgetExceeded(budgetDeadline) {
			slog.Debug("flow: retry budget exceeded", "attempt", attempt)
			if currentToken != nil {
				f.tokenSvc.ReleaseToken(currentToken.ID)
				currentToken = nil
			}
			lastErr = ErrRetryBudgetExceeded
			break
		}
		attemptStart := time.Now()
		// Check context
		if ctx.Err() != nil {
			if currentToken != nil {
				f.tokenSvc.ReleaseToken(currentToken.ID)
			}
			outCh <- StreamEvent{Error: ctx.Err()}
			return
		}

		// Pick new token if needed — try pools in order
		if currentToken == nil || tokenRetries >= cfg.PerTokenRetries {
			// Release old token's inflight when swapping due to retry exhaustion
			// (e.g. stream returned no data and no error).
			if currentToken != nil {
				f.tokenSvc.ReleaseToken(currentToken.ID)
				currentToken = nil
			}
			var tok *store.Token
			var pickErr error
			for _, pool := range pools {
				tok, pickErr = f.tokenSvc.Pick(pool, req.Mode)
				if pickErr == nil {
					break
				}
				slog.Debug("flow: pool exhausted, trying next",
					"pool", pool, "error", pickErr)
			}
			if pickErr != nil {
				slog.Debug("flow: no token available", "pools", pools, "error", pickErr)
				lastErr = pickErr
				break
			}
			maskedTok := tok.Token
			if len(maskedTok) > 16 {
				maskedTok = maskedTok[:8] + "..." + maskedTok[len(maskedTok)-4:]
			}
			slog.Debug("flow: token picked",
				"token_id", tok.ID, "token", maskedTok,
				"pools", pools, "quota", tok.Quotas[req.Mode],
				"priority", tok.Priority, "attempt", attempt)
			currentToken = tok
			tokenRetries = 0
			client = f.clientFactory(tok.Token)
			if client == nil {
				f.tokenSvc.ReleaseToken(tok.ID)
				outCh <- StreamEvent{Error: errors.New("chat client is nil")}
				return
			}
		}

		// Build xai request
		xaiReq, err := f.buildXAIRequest(ctx, req, client)
		if err != nil {
			f.tokenSvc.ReleaseToken(currentToken.ID)
			outCh <- StreamEvent{Error: err}
			return
		}

		// Execute chat
		eventCh, err := client.Chat(ctx, xaiReq)
		if err != nil {
			lastErr = err
			slog.Debug("flow: chat execution error",
				"attempt", attempt, "token_id", currentToken.ID,
				"error", err, "token_retries", tokenRetries)
			if resetErr := f.resetSessionIfNeeded(err, cfg, client); resetErr != nil {
				f.tokenSvc.ReleaseToken(currentToken.ID)
				outCh <- StreamEvent{Error: resetErr}
				return
			}
			tokenRetries++
			sameTokenRetry := shouldRetrySameToken(err, cfg, tokenRetries)
			if sameTokenRetry {
				f.handleErrorKeepInflight(currentToken.ID, req.Mode, err, cfg)
			} else {
				f.handleErrorAndRelease(currentToken.ID, req.Mode, err, cfg)
			}

			if IsNonRecoverable(err) {
				slog.Debug("flow: error not recoverable, giving up", "error", err)
				outCh <- StreamEvent{Error: err}
				return
			}

			// Force swap token on 429 quota exhaustion or ErrInvalidToken.
			if !sameTokenRetry {
				slog.Debug("flow: forcing token swap", "token_id", currentToken.ID, "error", err)
				currentToken = nil
			}

			// Backoff before retry
			delay := BackoffWithJitter(attempt, cfg)
			slog.Debug("flow: backing off before retry",
				"attempt", attempt, "delay_ms", delay.Milliseconds())
			if retryDelayExceedsBudget(budgetDeadline, delay) {
				if sameTokenRetry {
					f.tokenSvc.ReleaseToken(currentToken.ID)
					currentToken = nil
				}
				lastErr = ErrRetryBudgetExceeded
				break
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				if sameTokenRetry {
					f.tokenSvc.ReleaseToken(currentToken.ID)
				}
				outCh <- StreamEvent{Error: ctx.Err()}
				return
			case <-timer.C:
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue
		}

		f.tokenSvc.RecordFirstUse(currentToken.ID, req.Mode)

		// Stream events
		success, usage, estimated, ttft, streamErr := f.streamEvents(ctx, eventCh, outCh, client.DownloadURL, req.Tools)
		if success {
			// Estimate prompt tokens from request messages if not set by upstream.
			if usage != nil && usage.PromptTokens == 0 {
				usage.PromptTokens = f.estimatePromptTokens(req)
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
				estimated = true
			}
			f.tokenSvc.ReportSuccess(currentToken.ID)
			var tokIn, tokOut int
			if usage != nil {
				tokIn = usage.PromptTokens
				tokOut = usage.CompletionTokens
			}
			f.recordUsage(apiKeyID, currentToken.ID, req.Model, "chat", 200, time.Since(attemptStart), ttft, tokIn, tokOut, estimated)
			slog.Debug("flow: chat success",
				"token_id", currentToken.ID, "model", req.Model,
				"latency_ms", time.Since(attemptStart).Milliseconds(),
				"tokens_in", tokIn, "tokens_out", tokOut)
			// Increment API key daily usage on success
			if f.apiKeyUsageInc != nil && apiKeyID > 0 {
				f.apiKeyUsageInc(ctx, apiKeyID)
			}
			return
		}

		// Stream failed — capture error for potential final report
		if streamErr != nil {
			lastErr = streamErr
			tokenRetries++
			sameTokenRetry := shouldRetrySameToken(streamErr, cfg, tokenRetries)
			if sameTokenRetry {
				f.handleErrorKeepInflight(currentToken.ID, req.Mode, streamErr, cfg)
			} else {
				f.handleErrorAndRelease(currentToken.ID, req.Mode, streamErr, cfg)
				currentToken = nil
			}
			if IsNonRecoverable(streamErr) {
				outCh <- StreamEvent{Error: streamErr}
				return
			}
			continue
		}
		tokenRetries++
	}

	// All retries exhausted — always send error to client
	if lastErr == nil {
		lastErr = errors.New("all retries exhausted")
	}
	outCh <- StreamEvent{Error: lastErr}
}

func (f *ChatFlow) appConfig() *config.AppConfig {
	if f.cfg == nil {
		return nil
	}
	if f.cfg.AppConfigProvider != nil {
		return f.cfg.AppConfigProvider()
	}
	return f.cfg.AppConfig
}

func (f *ChatFlow) filterTags() []string {
	if f.cfg == nil {
		return nil
	}
	if f.cfg.FilterTagsProvider != nil {
		return f.cfg.FilterTagsProvider()
	}
	return f.cfg.FilterTags
}

func shouldRetrySameToken(err error, cfg *RetryConfig, tokenRetries int) bool {
	if err == nil || cfg == nil {
		return false
	}
	if IsNonRecoverable(err) || tokenRetries >= cfg.PerTokenRetries {
		return false
	}
	return !ShouldSwapToken(err, cfg)
}

func (f *ChatFlow) handleErrorAndRelease(tokenID uint, mode string, err error, cfg *RetryConfig) {
	f.handleError(tokenID, mode, err, cfg, false)
}

func (f *ChatFlow) handleErrorKeepInflight(tokenID uint, mode string, err error, cfg *RetryConfig) {
	f.handleError(tokenID, mode, err, cfg, true)
}

func (f *ChatFlow) handleError(tokenID uint, mode string, err error, cfg *RetryConfig, keepInflight bool) {
	reason := truncateReason(err.Error())
	if errors.Is(err, xai.ErrInvalidToken) {
		slog.Debug("flow: marking token expired (401)", "token_id", tokenID)
		f.tokenSvc.MarkExpired(tokenID, reason)
		return
	}
	if errors.Is(err, xai.ErrForbidden) {
		slog.Debug("flow: 403 without token penalty", "token_id", tokenID)
		if !keepInflight {
			f.tokenSvc.ReleaseToken(tokenID)
		}
		return
	}
	if errors.Is(err, xai.ErrCFChallenge) {
		// CF challenge — token is fine, don't penalize
		slog.Debug("flow: CF challenge, no token penalty", "token_id", tokenID)
		if !keepInflight {
			f.tokenSvc.ReleaseToken(tokenID)
		}
		return
	}
	if isTransportError(err) {
		// Transport error — recoverable, refund quota
		slog.Debug("flow: transport error, refunding quota", "token_id", tokenID)
		f.reportTokenError(tokenID, mode, true, reason, keepInflight)
		return
	}
	if ShouldCoolToken(err, cfg) {
		slog.Debug("flow: reporting rate limit", "token_id", tokenID, "error", err)
		f.tokenSvc.ReportRateLimit(tokenID, mode, reason)
	} else {
		// 5xx → recoverable (refund); other client errors → not recoverable
		recoverable := isServerError(err)
		slog.Debug("flow: reporting error", "token_id", tokenID, "error", err, "recoverable", recoverable)
		f.reportTokenError(tokenID, mode, recoverable, reason, keepInflight)
	}
}

func (f *ChatFlow) reportTokenError(tokenID uint, mode string, recoverable bool, reason string, keepInflight bool) {
	if keepInflight {
		f.tokenSvc.ReportErrorKeepInflight(tokenID, mode, recoverable, reason)
		return
	}
	f.tokenSvc.ReportError(tokenID, mode, recoverable, reason)
}

// truncateReason truncates a reason string to 256 characters max.
func truncateReason(s string) string {
	if len(s) <= 256 {
		return s
	}
	return s[:256]
}

func (f *ChatFlow) resetSessionIfNeeded(err error, cfg *RetryConfig, client xai.Client) error {
	if client == nil || !ShouldResetSession(err, cfg) {
		return nil
	}
	// Trigger CF refresh only on CF challenge (not token-level 403).
	if errors.Is(err, xai.ErrCFChallenge) && f.cfRefreshTrigger != nil {
		SafeGo("chat_cf_refresh_trigger", func() {
			f.cfRefreshTrigger()
		})
	}
	slog.Debug("flow: resetting session due to error", "error", err)
	if resetErr := client.ResetSession(); resetErr != nil {
		slog.Warn("flow: session reset failed", "error", resetErr)
		return nil
	}
	slog.Debug("flow: session reset successful")
	return nil
}

func retryBudgetDeadline(cfg *RetryConfig) time.Time {
	if cfg == nil || cfg.RetryBudget <= 0 {
		return time.Time{}
	}
	return time.Now().Add(cfg.RetryBudget)
}

func retryBudgetExceeded(deadline time.Time) bool {
	return !deadline.IsZero() && time.Now().After(deadline)
}

func retryDelayExceedsBudget(deadline time.Time, delay time.Duration) bool {
	return !deadline.IsZero() && time.Now().Add(delay).After(deadline)
}
