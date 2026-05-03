package token

import "time"

// ReleaseInflightOnly decrements the inflight counter without any state change.
func (m *TokenManager) ReleaseInflightOnly(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseInflight(id)
}

// GetInflight returns the current inflight count for a token.
func (m *TokenManager) GetInflight(id uint) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.inflight[id]
}

// releaseInflight decrements the inflight counter. Caller must hold m.mu.
func (m *TokenManager) releaseInflight(id uint) {
	if m.inflight[id] > 0 {
		m.inflight[id]--
	}
	if m.inflight[id] == 0 {
		delete(m.inflight, id)
	}
}

// addInflightExcludes returns exclude with tokens at max inflight added.
func (m *TokenManager) addInflightExcludes(exclude map[uint]struct{}) map[uint]struct{} {
	if m.cfg.MaxInflight <= 0 {
		return exclude
	}
	for id, count := range m.inflight {
		if count < m.cfg.MaxInflight {
			continue
		}
		if exclude == nil {
			exclude = make(map[uint]struct{})
		}
		exclude[id] = struct{}{}
	}
	return exclude
}

// addRecentUseExcludes returns a copy with recently picked tokens excluded.
func (m *TokenManager) addRecentUseExcludes(exclude map[uint]struct{}) map[uint]struct{} {
	window := m.cfg.RecentUsePenaltySec
	if window <= 0 {
		return exclude
	}
	penalized := m.recentlyPickedIDs(time.Now().Add(-time.Duration(window) * time.Second))
	if len(penalized) == 0 {
		return exclude
	}
	result := make(map[uint]struct{}, len(exclude)+len(penalized))
	for id := range exclude {
		result[id] = struct{}{}
	}
	for _, id := range penalized {
		result[id] = struct{}{}
	}
	return result
}

func (m *TokenManager) recentlyPickedIDs(cutoff time.Time) []uint {
	var penalized []uint
	for id, pickedAt := range m.lastPickedAt {
		if pickedAt.After(cutoff) {
			penalized = append(penalized, id)
			continue
		}
		delete(m.lastPickedAt, id)
	}
	return penalized
}
