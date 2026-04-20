// Package token provides token lifecycle management with state machine and pool selection.
package token

import (
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
)

// QuotaCategory represents a quota consumption category.
type QuotaCategory string

const (
	CategoryChat   QuotaCategory = "chat"
	CategoryImage  QuotaCategory = "image"
	CategoryVideo  QuotaCategory = "video"
	CategoryGrok43 QuotaCategory = "grok43"
)

// GetQuota returns the quota value for the given category.
func GetQuota(t *store.Token, cat QuotaCategory) int {
	switch cat {
	case CategoryImage:
		return t.ImageQuota
	case CategoryVideo:
		return t.VideoQuota
	case CategoryGrok43:
		return t.Grok43Quota
	default:
		return t.ChatQuota
	}
}

// SetQuota sets the quota value for the given category.
func SetQuota(t *store.Token, cat QuotaCategory, val int) {
	switch cat {
	case CategoryImage:
		t.ImageQuota = val
	case CategoryVideo:
		t.VideoQuota = val
	case CategoryGrok43:
		t.Grok43Quota = val
	default:
		t.ChatQuota = val
	}
}

// CategoryFromQuotaMode maps a modelconfig quota_mode string to a QuotaCategory.
// Unknown modes default to CategoryChat.
func CategoryFromQuotaMode(mode string) QuotaCategory {
	switch mode {
	case modelconfig.QuotaGrok43:
		return CategoryGrok43
	default:
		// auto, fast, expert, heavy all consume chat quota
		return CategoryChat
	}
}
