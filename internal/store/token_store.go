package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Token status constants.
const (
	TokenStatusActive   = "active"
	TokenStatusDisabled = "disabled"
	TokenStatusExpired  = "expired"
	TokenStatusCooling  = "cooling"
)

// TokenFilter holds filter criteria for listing tokens.
type TokenFilter struct {
	Status      *string
	NsfwEnabled *bool
}

// BatchUpdateRequest holds parameters for batch updating tokens.
type BatchUpdateRequest struct {
	IDs          []uint
	Status       *string
	StatusReason *string
	NsfwEnabled  *bool
}

// ErrNotFound is returned when a record is not found.
var ErrNotFound = errors.New("record not found")

// ErrConflict is returned when a derived request name conflicts with existing models.
var ErrConflict = errors.New("request name conflict")

// ErrInvalidInput is returned when a request violates model definition rules.
var ErrInvalidInput = errors.New("invalid input")

// TokenStore provides CRUD operations for Token.
type TokenStore struct {
	db *gorm.DB
}

// NewTokenStore creates a new TokenStore.
func NewTokenStore(db *gorm.DB) *TokenStore {
	return &TokenStore{db: db}
}

// ListTokens returns all active tokens.
func (s *TokenStore) ListTokens(ctx context.Context) ([]*Token, error) {
	var tokens []*Token
	err := s.db.WithContext(ctx).Find(&tokens).Error
	return tokens, err
}

// ListTokensFiltered returns tokens matching the filter criteria.
func (s *TokenStore) ListTokensFiltered(ctx context.Context, filter TokenFilter) ([]*Token, error) {
	query := s.db.WithContext(ctx)
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.NsfwEnabled != nil {
		query = query.Where("nsfw_enabled = ?", *filter.NsfwEnabled)
	}
	var tokens []*Token
	return tokens, query.Find(&tokens).Error
}

// BatchUpdateTokens updates multiple tokens in a transaction.
func (s *TokenStore) BatchUpdateTokens(ctx context.Context, req BatchUpdateRequest) (int, error) {
	updates := map[string]any{}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.StatusReason != nil {
		updates["status_reason"] = *req.StatusReason
	}
	if req.NsfwEnabled != nil {
		updates["nsfw_enabled"] = *req.NsfwEnabled
	}
	if len(updates) == 0 {
		return 0, nil
	}

	var count int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Token{}).Where("id IN ?", req.IDs).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		count = result.RowsAffected
		return nil
	})
	return int(count), err
}

// TokenSnapshotData holds the mutable fields of a token for batch updates.
// This mirrors token.TokenSnapshot to avoid circular imports.
type TokenSnapshotData struct {
	ID                uint
	Status            string
	StatusReason      string
	ChatQuota         int
	InitialChatQuota  int
	ImageQuota        int
	InitialImageQuota int
	VideoQuota        int
	InitialVideoQuota int
	FailCount         int
	CoolUntil         *time.Time
	LastUsed          *time.Time
}

// UpdateTokenSnapshots batch updates token snapshots.
func (s *TokenStore) UpdateTokenSnapshots(ctx context.Context, snapshots []TokenSnapshotData) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, snap := range snapshots {
			updates := map[string]any{
				"status":              snap.Status,
				"status_reason":       snap.StatusReason,
				"chat_quota":          snap.ChatQuota,
				"initial_chat_quota":  snap.InitialChatQuota,
				"image_quota":         snap.ImageQuota,
				"initial_image_quota": snap.InitialImageQuota,
				"video_quota":         snap.VideoQuota,
				"initial_video_quota": snap.InitialVideoQuota,
				"fail_count":          snap.FailCount,
				"cool_until":          snap.CoolUntil,
				"last_used":           snap.LastUsed,
			}
			if err := tx.Model(&Token{}).Where("id = ?", snap.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetToken returns a token by ID.
func (s *TokenStore) GetToken(ctx context.Context, id uint) (*Token, error) {
	var token Token
	err := s.db.WithContext(ctx).First(&token, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &token, err
}

// CreateToken creates a new token.
func (s *TokenStore) CreateToken(ctx context.Context, token *Token) error {
	return s.db.WithContext(ctx).Create(token).Error
}

// UpdateToken updates an existing token.
func (s *TokenStore) UpdateToken(ctx context.Context, token *Token) error {
	result := s.db.WithContext(ctx).Save(token)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListTokenIDs returns token IDs matching the filter criteria.
func (s *TokenStore) ListTokenIDs(ctx context.Context, filter TokenFilter) ([]uint, error) {
	query := s.db.WithContext(ctx).Model(&Token{})
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.NsfwEnabled != nil {
		query = query.Where("nsfw_enabled = ?", *filter.NsfwEnabled)
	}
	var ids []uint
	return ids, query.Pluck("id", &ids).Error
}

// DeleteToken soft-deletes a token by ID.
func (s *TokenStore) DeleteToken(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&Token{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
