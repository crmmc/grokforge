package token

import (
	"log/slog"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/crmmc/grokforge/internal/store"
)

// Algorithm constants for token selection.
const (
	AlgoHighQuotaFirst = "high_quota_first"
	AlgoRandom         = "random"
	AlgoRoundRobin     = "round_robin"
)

// ValidAlgorithm checks if the given string is a valid selection algorithm.
func ValidAlgorithm(algo string) bool {
	switch algo {
	case AlgoHighQuotaFirst, AlgoRandom, AlgoRoundRobin:
		return true
	default:
		return false
	}
}

// TokenPool manages a collection of tokens with thread-safe access.
type TokenPool struct {
	name    string
	tokens  map[uint]*store.Token
	mu      sync.RWMutex
	rrIndex atomic.Uint64 // round-robin counter
}

// NewTokenPool creates a new token pool with the given name.
func NewTokenPool(name string) *TokenPool {
	return &TokenPool{
		name:   name,
		tokens: make(map[uint]*store.Token),
	}
}

// Name returns the pool name.
func (p *TokenPool) Name() string {
	return p.name
}

// Add adds a token to the pool.
func (p *TokenPool) Add(token *store.Token) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens[token.ID] = token
}

// Remove removes a token from the pool by ID.
func (p *TokenPool) Remove(id uint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tokens, id)
}

// Get returns a token by ID, or nil if not found.
func (p *TokenPool) Get(id uint) *store.Token {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tokens[id]
}

// Select returns an active token using the specified algorithm with priority-tier grouping.
// Tokens are grouped by priority (descending), and the algorithm is applied within each tier.
// Falls through to lower priority tiers when higher tiers have no active tokens.
// Returns nil if no active tokens are available.
func (p *TokenPool) Select(algorithm string, mode string) *store.Token {
	return p.selectWithExclude(algorithm, mode, nil)
}

// SelectExcluding returns an active token while skipping excluded token IDs.
func (p *TokenPool) SelectExcluding(algorithm string, mode string, exclude map[uint]struct{}) *store.Token {
	return p.selectWithExclude(algorithm, mode, exclude)
}

func (p *TokenPool) selectWithExclude(algorithm string, mode string, exclude map[uint]struct{}) *store.Token {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Collect active tokens with remaining quota, grouped by priority
	tiers := make(map[int][]*store.Token)
	for _, t := range p.tokens {
		if _, skipped := exclude[t.ID]; skipped {
			continue
		}
		if Status(t.Status) != StatusActive {
			continue
		}
		// 检查 mode quota
		if t.Quotas == nil || t.Quotas[mode] <= 0 {
			continue
		}
		tiers[t.Priority] = append(tiers[t.Priority], t)
	}
	if len(tiers) == 0 {
		return nil
	}

	// Sort priority keys descending
	priorities := sortedKeysDesc(tiers)

	// Try each tier in priority order
	for _, pri := range priorities {
		candidates := tiers[pri]
		if selected := p.selectByAlgorithm(algorithm, mode, candidates); selected != nil {
			slog.Debug("token: selected from pool",
				"pool", p.name, "token_id", selected.ID,
				"priority", pri, "quota", selected.Quotas[mode],
				"mode", mode, "algo", algorithm, "candidates", len(candidates))
			return selected
		}
	}
	slog.Debug("token: no token available in pool",
		"pool", p.name, "total_tokens", len(p.tokens))
	return nil
}

// Count returns the count of tokens by status.
func (p *TokenPool) Count() (active, disabled, expired int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, t := range p.tokens {
		switch Status(t.Status) {
		case StatusActive:
			active++
		case StatusDisabled:
			disabled++
		case StatusExpired:
			expired++
		}
	}
	return
}

// All returns all tokens in the pool.
func (p *TokenPool) All() []*store.Token {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*store.Token, 0, len(p.tokens))
	for _, t := range p.tokens {
		result = append(result, t)
	}
	return result
}

// selectByAlgorithm applies the specified algorithm to select a token from candidates.
func (p *TokenPool) selectByAlgorithm(algo string, mode string, candidates []*store.Token) *store.Token {
	if len(candidates) == 0 {
		return nil
	}

	switch algo {
	case AlgoRandom:
		return candidates[rand.Intn(len(candidates))]

	case AlgoRoundRobin:
		// Sort candidates by ID for deterministic order
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].ID < candidates[j].ID
		})
		idx := p.rrIndex.Add(1) - 1
		return candidates[idx%uint64(len(candidates))]

	default: // AlgoHighQuotaFirst and unknown algorithms
		return selectHighQuotaFirst(candidates, mode)
	}
}

// selectHighQuotaFirst selects the token with the highest quota for the given mode.
// If multiple tokens have the same quota, one is selected randomly.
func selectHighQuotaFirst(candidates []*store.Token, mode string) *store.Token {
	if len(candidates) == 0 {
		return nil
	}

	var best []*store.Token
	maxQuota := -1

	for _, t := range candidates {
		q := 0
		if t.Quotas != nil {
			q = t.Quotas[mode]
		}
		if q > maxQuota {
			maxQuota = q
			best = []*store.Token{t}
		} else if q == maxQuota {
			best = append(best, t)
		}
	}

	if len(best) == 1 {
		return best[0]
	}
	return best[rand.Intn(len(best))]
}

// sortedKeysDesc returns map keys sorted in descending order.
func sortedKeysDesc(m map[int][]*store.Token) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	return keys
}
