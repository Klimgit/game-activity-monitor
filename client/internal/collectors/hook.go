package collectors

import (
	"context"
	"sync"

	hook "github.com/robotn/gohook"
)

type HookBus struct {
	mu          sync.RWMutex
	subscribers []chan hook.Event
	once        sync.Once
}

func newHookBus() *HookBus {
	return &HookBus{}
}

func (b *HookBus) Subscribe() <-chan hook.Event {
	ch := make(chan hook.Event, 256)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

func (b *HookBus) Start(ctx context.Context) {
	b.once.Do(func() {
		go b.run(ctx)
	})
}

func (b *HookBus) run(ctx context.Context) {
	evChan := hook.Start()
	defer hook.End()

	for {
		select {
		case ev, ok := <-evChan:
			if !ok {
				return
			}
			b.mu.RLock()
			for _, sub := range b.subscribers {
				select {
				case sub <- ev:
				default:
				}
			}
			b.mu.RUnlock()
		case <-ctx.Done():
			return
		}
	}
}
