package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
)

const (
	defaultBlockedParallelAttempts = 5
	maxBlockedParallelAttempts     = 10
)

var errImageGenerationBlocked = errors.New("content blocked by safety filter")

type imageGenerationResult struct {
	data  *ImageData
	token *store.Token
}

type imageAttemptResult struct {
	data  *ImageData
	token *store.Token
	err   error
}

func (f *ImageFlow) generateWithRecovery(
	ctx context.Context,
	model string,
	mode string,
	cooldownSeconds int,
	token *store.Token,
	prompt, aspectRatio string,
	enableNSFW, enablePro bool,
) (*imageGenerationResult, error) {
	data, err := f.generateSingle(ctx, f.clientFactory(token.Token), prompt, aspectRatio, enableNSFW, enablePro)
	if !errors.Is(err, errImageGenerationBlocked) {
		if err != nil {
			return nil, err
		}
		return &imageGenerationResult{data: data, token: token}, nil
	}
	if !blockedParallelEnabled(f.imageConfig()) {
		f.tokenSvc.ReleaseToken(token.ID)
		return nil, err
	}
	// Release initial token's inflight before recovery with different tokens.
	f.tokenSvc.ReleaseToken(token.ID)
	return f.generateBlockedRecovery(ctx, model, mode, cooldownSeconds, token.ID, prompt, aspectRatio, enableNSFW, enablePro)
}

func (f *ImageFlow) generateSingle(
	ctx context.Context,
	client ImagineGenerator,
	prompt, aspectRatio string,
	enableNSFW, enablePro bool,
) (*ImageData, error) {
	if client == nil {
		return nil, errors.New("image client is nil")
	}
	eventCh, err := client.Generate(ctx, prompt, aspectRatio, enableNSFW, enablePro)
	if err != nil {
		return nil, fmt.Errorf("start generation: %w", err)
	}

	var finalImage string
	for event := range eventCh {
		switch event.Type {
		case xai.ImageEventFinal:
			finalImage = event.ImageData
		case xai.ImageEventBlocked:
			return nil, errImageGenerationBlocked
		case xai.ImageEventError:
			if event.Error != nil {
				return nil, fmt.Errorf("generation error: %w", event.Error)
			}
			return nil, errors.New("unknown generation error")
		}
	}
	if finalImage == "" {
		return nil, errors.New("no final image received")
	}

	return &ImageData{B64JSON: finalImage, RevisedPrompt: prompt}, nil
}

func (f *ImageFlow) generateBlockedRecovery(
	ctx context.Context,
	model string,
	mode string,
	cooldownSeconds int,
	initialTokenID uint,
	prompt, aspectRatio string,
	enableNSFW, enablePro bool,
) (*imageGenerationResult, error) {
	attempts := blockedParallelAttempts(f.imageConfig())
	if attempts == 0 {
		return nil, errImageGenerationBlocked
	}
	recoveryTokens := f.selectRecoveryTokens(model, mode, initialTokenID, attempts)
	if len(recoveryTokens) == 0 {
		return nil, errImageGenerationBlocked
	}

	retryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan imageAttemptResult, len(recoveryTokens))
	var wg sync.WaitGroup
	for _, recoveryToken := range recoveryTokens {
		tok := recoveryToken
		wg.Add(1)
		SafeGo("image_blocked_recovery_attempt", func() {
			defer wg.Done()
			result := imageAttemptResult{token: tok}
			result.data, result.err = f.generateSingle(
				retryCtx,
				f.clientFactory(tok.Token),
				prompt,
				aspectRatio,
				enableNSFW,
				enablePro,
			)
			// Always send — resultCh is buffered to len(recoveryTokens),
			// so this never blocks. Ensures every token gets finalized.
			resultCh <- result
		})
	}

	SafeGo("image_blocked_recovery_wait", func() {
		wg.Wait()
		close(resultCh)
	})

	return f.selectImageRecoveryResult(model, resultCh, cancel, mode, cooldownSeconds)
}

func (f *ImageFlow) selectRecoveryTokens(model, mode string, initialTokenID uint, attempts int) []*store.Token {
	exclude := map[uint]struct{}{initialTokenID: {}}
	tokens := make([]*store.Token, 0, attempts)
	for i := 0; i < attempts; i++ {
		var (
			tok *store.Token
			err error
		)
		if mode == "" {
			tok, err = f.pickWSTokenForModelExcluding(model, exclude)
		} else {
			tok, err = f.pickTokenForModelExcluding(model, mode, exclude)
		}
		if err != nil {
			break
		}
		exclude[tok.ID] = struct{}{}
		tokens = append(tokens, tok)
	}
	return tokens
}

func (f *ImageFlow) selectImageRecoveryResult(
	model string,
	resultCh <-chan imageAttemptResult,
	cancel context.CancelFunc,
	mode string,
	cooldownSeconds int,
) (*imageGenerationResult, error) {
	var firstErr error
	var winner *imageGenerationResult
	for result := range resultCh {
		if result.err == nil && winner == nil {
			cancel()
			winner = &imageGenerationResult{
				data:  result.data,
				token: result.token,
			}
			if mode != "" {
				f.tokenSvc.ReportSuccess(result.token.ID)
			}
		} else if result.err == nil {
			// Successful loser — winner already chosen, just release inflight.
			f.tokenSvc.ReleaseToken(result.token.ID)
		} else if result.err != nil {
			if mode == "" {
				f.reportWSImageError(model, result.token.ID, result.err, cooldownSeconds)
			} else if !errors.Is(result.err, errImageGenerationBlocked) {
				recoverable := isTransportError(result.err) || isServerError(result.err)
				f.tokenSvc.ReportError(result.token.ID, mode, recoverable, truncateReason(result.err.Error()))
			} else {
				// Blocked result — no token penalty, just release inflight.
				f.tokenSvc.ReleaseToken(result.token.ID)
			}
			if firstErr == nil && !errors.Is(result.err, errImageGenerationBlocked) {
				firstErr = result.err
			}
		}
	}
	if winner != nil {
		return winner, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errImageGenerationBlocked
}

func resolveEnableNSFW(v *bool, cfg *config.ImageConfig) bool {
	if v != nil {
		return *v
	}
	if cfg == nil {
		return false
	}
	return cfg.NSFW
}

func blockedParallelAttempts(cfg *config.ImageConfig) int {
	if cfg == nil || cfg.BlockedParallelAttempts <= 0 {
		return defaultBlockedParallelAttempts
	}
	if cfg.BlockedParallelAttempts > maxBlockedParallelAttempts {
		return maxBlockedParallelAttempts
	}
	return cfg.BlockedParallelAttempts
}

func blockedParallelEnabled(cfg *config.ImageConfig) bool {
	return config.EffectiveBlockedParallelEnabled(cfg)
}
