package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

const (
	defaultBlockedParallelAttempts = 5
	maxBlockedParallelAttempts     = 10
)

var errImageGenerationBlocked = errors.New("content blocked by safety filter")

type imageGenerationResult struct {
	data     *ImageData
	token    *store.Token
	consumed bool // true if quota was already consumed (e.g. by blocked recovery)
}

type imageAttemptResult struct {
	data  *ImageData
	token *store.Token
	err   error
}

func (f *ImageFlow) generateWithRecovery(
	ctx context.Context,
	model string,
	token *store.Token,
	prompt, aspectRatio string,
	enableNSFW bool,
) (*imageGenerationResult, error) {
	data, err := f.generateSingle(ctx, f.clientFactory(token.Token), prompt, aspectRatio, enableNSFW)
	if !errors.Is(err, errImageGenerationBlocked) {
		if err != nil {
			return nil, err
		}
		return &imageGenerationResult{data: data, token: token}, nil
	}
	if !blockedParallelEnabled(f.imageConfig()) {
		return nil, err
	}
	return f.generateBlockedRecovery(ctx, model, token.ID, prompt, aspectRatio, enableNSFW)
}

func (f *ImageFlow) generateSingle(
	ctx context.Context,
	client ImagineGenerator,
	prompt, aspectRatio string,
	enableNSFW bool,
) (*ImageData, error) {
	if client == nil {
		return nil, errors.New("image client is nil")
	}
	eventCh, err := client.Generate(ctx, prompt, aspectRatio, enableNSFW)
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
	initialTokenID uint,
	prompt, aspectRatio string,
	enableNSFW bool,
) (*imageGenerationResult, error) {
	attempts := blockedParallelAttempts(f.imageConfig())
	if attempts == 0 {
		return nil, errImageGenerationBlocked
	}
	recoveryTokens := f.selectRecoveryTokens(model, initialTokenID, attempts)
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
			)
			if result.err == nil {
				// Consume quota only on success
				if _, consumeErr := f.tokenSvc.Consume(tok.ID, tkn.CategoryImage, 1); consumeErr != nil {
					result.err = fmt.Errorf("token quota exhausted: %w", consumeErr)
					result.data = nil
				}
			}
			select {
			case resultCh <- result:
			case <-retryCtx.Done():
			}
		})
	}

	SafeGo("image_blocked_recovery_wait", func() {
		wg.Wait()
		close(resultCh)
	})

	return selectImageRecoveryResult(resultCh, cancel, f.tokenSvc)
}

func (f *ImageFlow) selectRecoveryTokens(model string, initialTokenID uint, attempts int) []*store.Token {
	exclude := map[uint]struct{}{initialTokenID: {}}
	tokens := make([]*store.Token, 0, attempts)
	for i := 0; i < attempts; i++ {
		tok, err := f.pickTokenForModelExcluding(model, exclude)
		if err != nil {
			break
		}
		exclude[tok.ID] = struct{}{}
		tokens = append(tokens, tok)
	}
	return tokens
}

func selectImageRecoveryResult(resultCh <-chan imageAttemptResult, cancel context.CancelFunc, tokenSvc TokenServicer) (*imageGenerationResult, error) {
	var firstErr error
	var winner *imageGenerationResult
	for result := range resultCh {
		if result.err == nil && winner == nil {
			cancel()
			winner = &imageGenerationResult{
				data:     result.data,
				token:    result.token,
				consumed: true,
			}
			tokenSvc.ReportSuccess(result.token.ID)
		} else if result.err != nil {
			if !errors.Is(result.err, errImageGenerationBlocked) && !isTransportError(result.err) {
				tokenSvc.ReportError(result.token.ID, result.err.Error())
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
