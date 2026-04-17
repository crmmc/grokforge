package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// ModelStoreTx wraps a transactional ModelStore view.
type ModelStoreTx struct {
	tx    *gorm.DB
	store *ModelStore
}

// BeginTx starts a transaction for model mutations.
func (s *ModelStore) BeginTx(ctx context.Context) (*ModelStoreTx, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("begin model transaction: %w", tx.Error)
	}
	return &ModelStoreTx{
		tx:    tx,
		store: NewModelStore(tx),
	}, nil
}

// Store returns the transactional ModelStore view.
func (t *ModelStoreTx) Store() *ModelStore {
	return t.store
}

// Commit commits the transaction.
func (t *ModelStoreTx) Commit() error {
	if t == nil || t.tx == nil {
		return nil
	}
	err := t.tx.Commit().Error
	if err == nil {
		t.tx = nil
	}
	return err
}

// Rollback rolls the transaction back.
func (t *ModelStoreTx) Rollback() error {
	if t == nil || t.tx == nil {
		return nil
	}
	err := t.tx.Rollback().Error
	t.tx = nil
	return err
}
