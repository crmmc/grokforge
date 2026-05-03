package flow

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

func (f *ImageFlow) generateWS(ctx context.Context, req *ImageRequest) (*ImageResponse, error) {
	start := time.Now()
	aspectRatio := xai.ParseAspectRatio(req.Size)
	apiKeyID := FlowAPIKeyIDFromContext(ctx)
	enableNSFW := resolveEnableNSFW(req.EnableNSFW, f.imageConfig())
	enablePro := f.resolveEnablePro(req.Model)

	tok, err := f.pickWSTokenForModel(req.Model)
	if err != nil {
		return nil, err
	}

	images := make([]ImageData, 0, req.N)
	usedTokenIDs := make(map[uint]struct{})
	for i := 0; i < req.N; i++ {
		result, err := f.generateWithRecovery(
			ctx,
			req.Model,
			"",
			req.CooldownSeconds,
			tok,
			req.Prompt,
			aspectRatio,
			enableNSFW,
			enablePro,
		)
		if err != nil {
			f.reportWSImageError(req.Model, tok.ID, err, req.CooldownSeconds)
			f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
			return nil, err
		}
		tok = result.token
		data, err := f.resolveImageOutput(ctx, imageOutputInput{
			B64JSON: result.b64JSON,
			Prompt:  req.Prompt,
		})
		if err != nil {
			f.reportWSImageError(req.Model, tok.ID, err, req.CooldownSeconds)
			f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
			return nil, err
		}
		images = append(images, data)
		usedTokenIDs[tok.ID] = struct{}{}
	}

	for tokenID := range usedTokenIDs {
		f.tokenSvc.ReportSuccess(tokenID)
	}
	f.recordUsage(apiKeyID, tok.ID, req.Model, 200, time.Since(start))
	return &ImageResponse{
		Created: time.Now().Unix(),
		Data:    images,
	}, nil
}

func (f *ImageFlow) pickWSTokenForModel(model string) (*store.Token, error) {
	return f.pickWSTokenForModelExcluding(model, nil)
}

func (f *ImageFlow) pickWSTokenForModelExcluding(model string, exclude map[uint]struct{}) (*store.Token, error) {
	if f.modelResolver == nil {
		return nil, tkn.ErrModelNotFound
	}
	pools, ok := tkn.GetPoolForModel(model, f.modelResolver)
	if !ok {
		return nil, tkn.ErrModelNotFound
	}

	now := time.Now()
	var lastErr error
	for _, pool := range pools {
		poolExclude := cloneExclude(exclude)
		for {
			tok, err := f.tokenSvc.PickAnyExcluding(pool, poolExclude)
			if err != nil {
				lastErr = err
				break
			}
			if f.wsCooldownActive(model, tok.ID, now) {
				f.tokenSvc.ReleaseToken(tok.ID)
				poolExclude[tok.ID] = struct{}{}
				continue
			}
			return tok, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, tkn.ErrNoTokenAvailable
}

func cloneExclude(exclude map[uint]struct{}) map[uint]struct{} {
	cloned := make(map[uint]struct{}, len(exclude))
	for id := range exclude {
		cloned[id] = struct{}{}
	}
	return cloned
}

func (f *ImageFlow) wsCooldownActive(model string, tokenID uint, now time.Time) bool {
	key := wsCooldownKey(model, tokenID)

	f.cooldownMu.Lock()
	defer f.cooldownMu.Unlock()

	until, ok := f.cooldownUntil[key]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(f.cooldownUntil, key)
	return false
}

func (f *ImageFlow) setWSCooldown(model string, tokenID uint, seconds int) {
	if seconds <= 0 {
		return
	}
	until := time.Now().Add(time.Duration(seconds) * time.Second)
	key := wsCooldownKey(model, tokenID)

	f.cooldownMu.Lock()
	f.cooldownUntil[key] = until
	f.cooldownMu.Unlock()

	slog.Debug("image_ws: token cooldown applied",
		"token_id", tokenID,
		"model", model,
		"action", "transient_cooldown",
		"cooldown_seconds", seconds,
		"until", until)
}

func wsCooldownKey(model string, tokenID uint) string {
	return model + ":" + strconv.FormatUint(uint64(tokenID), 10)
}

func (f *ImageFlow) reportWSImageError(model string, tokenID uint, err error, cooldownSeconds int) {
	reason := truncateReason(err.Error())
	switch {
	case errors.Is(err, xai.ErrInvalidToken):
		f.tokenSvc.MarkExpired(tokenID, reason)
	case errors.Is(err, xai.ErrRateLimited), isWSBusyError(err):
		f.setWSCooldown(model, tokenID, cooldownSeconds)
		f.tokenSvc.ReleaseToken(tokenID)
	default:
		f.tokenSvc.ReleaseToken(tokenID)
	}
}

func isWSBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "busy")
}
