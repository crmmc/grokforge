package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// StringSlice is a custom type for JSON-encoded []string in SQLite.
type StringSlice []string

// Value implements driver.Valuer for database storage.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner for reading from database.
func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := src.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("StringSlice.Scan: unsupported type %T", src)
	}
	return json.Unmarshal(bytes, s)
}

// APIKey represents an API key for authenticating /v1/* requests.
type APIKey struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Key            string         `gorm:"uniqueIndex;size:128" json:"key"`
	Name           string         `gorm:"size:128" json:"name"`
	Status         string         `gorm:"index;size:32;default:active" json:"status"`
	ModelWhitelist StringSlice    `gorm:"type:text" json:"model_whitelist"`
	RateLimit      int            `gorm:"default:60" json:"rate_limit"`
	DailyLimit     int            `gorm:"default:1000" json:"daily_limit"`
	DailyUsed      int            `gorm:"default:0" json:"daily_used"`
	TotalUsed      int            `gorm:"default:0" json:"total_used"`
	LastUsedAt     *time.Time     `json:"last_used_at"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// Token represents a Grok authentication token.
type Token struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	Token             string         `gorm:"uniqueIndex;size:512" json:"token"`
	Pool              string         `gorm:"index;size:32" json:"pool"`   // canonical: ssoBasic, ssoSuper, ssoHeavy
	Status            string         `gorm:"index;size:32" json:"status"` // active, disabled, expired, cooling
	ChatQuota         int            `json:"chat_quota"`
	InitialChatQuota  int            `gorm:"default:0" json:"-"`
	ImageQuota        int            `json:"image_quota"`
	InitialImageQuota int            `gorm:"default:0" json:"-"`
	VideoQuota        int            `json:"video_quota"`
	InitialVideoQuota int            `gorm:"default:0" json:"-"`
	FailCount         int            `json:"fail_count"`
	CoolUntil         *time.Time     `json:"cool_until,omitempty"`
	LastUsed          *time.Time     `json:"last_used,omitempty"`
	Remark            string         `gorm:"type:text" json:"remark,omitempty"`
	NsfwEnabled       bool           `gorm:"default:false;index" json:"nsfw_enabled"`
	StatusReason      string         `gorm:"size:256" json:"status_reason,omitempty"`
	Priority          int            `gorm:"default:0;index" json:"priority"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// ConfigEntry stores configuration key-value pairs in database.
type ConfigEntry struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Key       string         `gorm:"uniqueIndex;size:128" json:"key"`
	Value     string         `gorm:"type:text" json:"value"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// UsageLog records token usage history.
type UsageLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TokenID      uint      `gorm:"index" json:"token_id"`
	APIKeyID     uint      `gorm:"index;default:0" json:"api_key_id"`
	Model        string    `gorm:"size:64" json:"model"`
	Endpoint     string    `gorm:"size:128" json:"endpoint"`
	Status       int       `gorm:"index:idx_created_status,priority:2" json:"status"`
	DurationMs   int64     `gorm:"column:duration_ms" json:"duration_ms"`
	TTFTMs       int       `gorm:"default:0" json:"ttft_ms"`
	CacheTokens  int       `gorm:"default:0" json:"cache_tokens"`
	TokensInput  int       `gorm:"default:0" json:"tokens_input"`
	TokensOutput int       `gorm:"default:0" json:"tokens_output"`
	Estimated    bool      `gorm:"default:false" json:"estimated"`
	CreatedAt    time.Time `gorm:"index;index:idx_created_status,priority:1" json:"created_at"`
}

// AllModels returns all models for AutoMigrate.
func AllModels() []any {
	return []any{
		&Token{},
		&ConfigEntry{},
		&UsageLog{},
		&APIKey{},
	}
}

// AutoMigrate creates the current schema.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(AllModels()...)
}
