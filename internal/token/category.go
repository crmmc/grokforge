// Package token provides token lifecycle management with state machine and pool selection.
package token

import (
	"strconv"
	"strings"

	"github.com/crmmc/grokforge/internal/store"
)

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

// ParseModelEntry parses "model#cost" format, returning (modelName, cost).
// Without #cost suffix, cost defaults to 1.
func ParseModelEntry(entry string) (string, int) {
	if i := strings.LastIndex(entry, "#"); i > 0 {
		if c, err := strconv.Atoi(entry[i+1:]); err == nil && c > 0 {
			return entry[:i], c
		}
	}
	return entry, 1
}

// CostForModel looks up the cost for a model from the resolver.
// Returns 1 as default when resolver is nil or model is not found.
func CostForModel(model string, resolver ModelResolver) int {
	if resolver == nil {
		return 1
	}
	_, cost, ok := resolver.ResolvePoolFloor(model)
	if !ok {
		return 1
	}
	if cost <= 0 {
		return 1
	}
	return cost
}
