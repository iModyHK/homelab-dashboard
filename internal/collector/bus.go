package collector

import (
	"sync"
)

type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Bus struct {
	mu   sync.RWMutex
	subs map[chan Message]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: map[chan Message]struct{}{}}
}

func (b *Bus) Subscribe(buffer int) (<-chan Message, func()) {
	ch := make(chan Message, buffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
		})
	}
	return ch, cancel
}

func (b *Bus) Publish(msgType string, data any) {
	msg := Message{Type: msgType, Data: data}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Bus) Subscribers() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
