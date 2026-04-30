package token

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"gorm.io/gorm"
)

const (
	// defaultFlushInterval is the default interval for dirty token persistence.
	defaultFlushInterval = 30 * time.Second
)

// Persister handles batch persistence of dirty tokens.
type Persister struct {
	manager  *TokenManager
	db       *gorm.DB
	wg       sync.WaitGroup
	stopOnce sync.Once
	stopped  chan struct{}
}

// NewPersister creates a new token persister.
func NewPersister(manager *TokenManager, db *gorm.DB) *Persister {
	return &Persister{
		manager: manager,
		db:      db,
		stopped: make(chan struct{}),
	}
}

// Start begins the periodic flush loop.
func (p *Persister) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultFlushInterval
	}
	p.wg.Add(1)
	safeGo("token_persister", func() {
		p.run(ctx, interval)
	})
}

// Stop waits for the flush loop to complete.
func (p *Persister) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopped)
	})
	p.wg.Wait()
}

// run is the main flush loop.
func (p *Persister) run(ctx context.Context, interval time.Duration) {
	defer p.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush before exit
			p.FlushDirty(context.Background())
			return
		case <-p.stopped:
			// Final flush before exit
			p.FlushDirty(context.Background())
			return
		case <-ticker.C:
			if _, err := p.FlushDirty(ctx); err != nil {
				slog.Error("failed to flush dirty tokens", "error", err)
			}
		}
	}
}

// FlushDirty persists all dirty tokens to the database.
// Returns the number of tokens flushed.
func (p *Persister) FlushDirty(ctx context.Context) (int, error) {
	dirty := p.manager.GetDirtyTokens()
	if len(dirty) == 0 {
		return 0, nil
	}

	slog.Debug("flushing dirty tokens", "count", len(dirty))

	// Use transaction for batch update
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, snapshot := range dirty {
			if err := p.updateToken(tx, snapshot); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		slog.Error("failed to persist dirty tokens", "error", err)
		// Do NOT clear dirty set — tokens remain dirty for next flush attempt
		return 0, err
	}

	// Clear dirty set only after successful persistence
	ids := make([]uint, len(dirty))
	for i, s := range dirty {
		ids[i] = s.ID
	}
	p.manager.ClearDirty(ids)

	slog.Debug("flushed dirty tokens", "count", len(dirty))
	return len(dirty), nil
}

// updateToken updates a single token in the database.
func (p *Persister) updateToken(tx *gorm.DB, snapshot TokenSnapshot) error {
	return tx.Model(&store.Token{}).Where("id = ?", snapshot.ID).Updates(map[string]interface{}{
		"status":        snapshot.Status,
		"status_reason": snapshot.StatusReason,
		"quotas":        snapshot.Quotas,
		"limit_quotas":  snapshot.LimitQuotas,
		"fail_count":    snapshot.FailCount,
		"last_used":     snapshot.LastUsed,
		"cool_untils":   snapshot.CoolUntils,
	}).Error
}
