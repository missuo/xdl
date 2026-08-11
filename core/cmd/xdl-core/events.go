package main

import (
	"encoding/json"
	"sync"
)

// event is one entry in the ring buffer the app polls via GET /events.
type event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// eventBuffer keeps absolute indices (base offset) so cursors stay valid as
// the ring drops old entries.
type eventBuffer struct {
	mu    sync.Mutex
	base  int64
	items []event
	max   int
}

func newEventBuffer(max int) *eventBuffer {
	return &eventBuffer{max: max}
}

func (b *eventBuffer) emit(typ string, data interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, event{Type: typ, Data: data})
	if over := len(b.items) - b.max; over > 0 {
		b.items = b.items[over:]
		b.base += int64(over)
	}
}

// since returns events at absolute index >= cursor and the next cursor.
// A negative cursor returns nothing but teaches the caller the current end,
// so a relaunched app doesn't replay the backlog.
func (b *eventBuffer) since(cursor int64) ([]event, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	end := b.base + int64(len(b.items))
	if cursor < 0 {
		return nil, end
	}
	if cursor < b.base {
		cursor = b.base
	}
	if cursor >= end {
		return nil, end
	}
	out := make([]event, end-cursor)
	copy(out, b.items[cursor-b.base:])
	return out, end
}

func (b *eventBuffer) marshalSince(cursor int64) ([]byte, error) {
	events, next := b.since(cursor)
	if events == nil {
		events = []event{}
	}
	return json.Marshal(map[string]interface{}{"events": events, "cursor": next})
}
