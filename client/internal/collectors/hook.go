package collectors

import (
	"context"
	"sync"

	hook "github.com/robotn/gohook"
)

// HookBus starts a single global input hook (required by gohook — only one
// hook may be running per process) and fans its events out to all subscribers.
type HookBus struct {
	mu          sync.RWMutex
	subscribers []chan hook.Event
	once        sync.Once
}

func newHookBus() *HookBus {
	return &HookBus{}
}

// Subscribe returns a channel that will receive all hook events.
// The channel is buffered (256) so a slow subscriber only drops events, not
// the entire pipeline.
func (b *HookBus) Subscribe() <-chan hook.Event {
	ch := make(chan hook.Event, 256)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Start launches the hook goroutine exactly once regardless of how many
// collectors call it.
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
				// Non-blocking send: drop event if subscriber is too slow.
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
