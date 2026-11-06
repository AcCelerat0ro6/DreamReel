package applicationrecommendation

import (
	"context"
	"sync"
	"time"

	domainrecommendation "DreamReel/internal/domain/recommendation"
)

type memorySession struct {
	state     SessionState
	items     []*domainrecommendation.Candidate
	expiresAt time.Time
}

// memorySessionStore 在未启用 Redis 时提供单进程分页，限制驻留的会话数量。
type memorySessionStore struct {
	mu      sync.Mutex
	entries map[string]*memorySession
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{entries: make(map[string]*memorySession)}
}

// lookup 在持锁期间查询会话，并清理已过期的条目。
func (m *memorySessionStore) lookup(id string) *memorySession {
	entry := m.entries[id]
	if entry != nil && !time.Now().Before(entry.expiresAt) {
		delete(m.entries, id)
		return nil
	}
	return entry
}

func (m *memorySessionStore) Read(_ context.Context, id string, offset, count int) (SessionState, []*domainrecommendation.Candidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.lookup(id)
	if entry == nil {
		return SessionState{}, nil, domainrecommendation.ErrCursorExpired
	}
	start := min(offset, len(entry.items))
	size := min(count, len(entry.items)-start)
	items := make([]*domainrecommendation.Candidate, 0, size)
	for _, v := range entry.items[start : start+size] {
		item := *v
		items = append(items, &item)
	}
	return entry.state, items, nil
}

func (m *memorySessionStore) EnqueuedIDs(_ context.Context, id string) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.lookup(id)
	if entry == nil {
		return nil, domainrecommendation.ErrCursorExpired
	}
	ids := make([]int64, 0, len(entry.items))
	for _, item := range entry.items {
		ids = append(ids, item.VideoID)
	}
	return ids, nil
}

func (m *memorySessionStore) Append(_ context.Context, id string, expectedVersion int, next SessionState, items []*domainrecommendation.Candidate) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.lookup(id)
	if entry == nil {
		if expectedVersion != 0 {
			return false, domainrecommendation.ErrCursorExpired
		}
		// 容量已满时回收最早到期的会话，客户端按游标过期流程刷新。
		if len(m.entries) >= 512 {
			var oldest string
			for key, v := range m.entries {
				if oldest == "" || v.expiresAt.Before(m.entries[oldest].expiresAt) {
					oldest = key
				}
			}
			delete(m.entries, oldest)
		}
		entry = &memorySession{expiresAt: time.Now().Add(SessionTTL)}
		m.entries[id] = entry
	}
	if entry.state.Version != expectedVersion {
		return false, nil
	}
	entry.items = append(entry.items, items...)
	entry.state = next
	return true, nil
}
