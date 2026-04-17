// Package token provides token lifecycle management with state machine and pool selection.
package token

import "github.com/crmmc/grokforge/internal/store"

// QuotaCategory represents a quota consumption category.
type QuotaCategory string

const (
	CategoryChat  QuotaCategory = "chat"
	CategoryImage QuotaCategory = "image"
	CategoryVideo QuotaCategory = "video"
)

// GetQuota returns the quota value for the given category.
func GetQuota(t *store.Token, cat QuotaCategory) int {
	switch cat {
	case CategoryImage:
		return t.ImageQuota
	case CategoryVideo:
		return t.VideoQuota
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
	default:
		t.ChatQuota = val
	}
}
