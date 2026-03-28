package collectors

import (
	"context"
	"fmt"
	"time"

	hook "github.com/robotn/gohook"

	"game-activity-monitor/client/internal/models"
)

type keyboardCollector struct {
	bus *HookBus
}

func newKeyboardCollector(bus *HookBus) *keyboardCollector {
	return &keyboardCollector{bus: bus}
}

func (k *keyboardCollector) Name() string { return "keyboard" }

func (k *keyboardCollector) Start(ctx context.Context, out chan<- *models.RawEvent) {
	ch := k.bus.Subscribe()

	// Track press times to compute hold duration on key-up.
	pressedAt := make(map[uint16]time.Time)

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Kind {
			case hook.KeyDown, hook.KeyHold:
				if _, already := pressedAt[ev.Keycode]; !already {
					pressedAt[ev.Keycode] = time.Now()
				}
				out <- &models.RawEvent{
					Timestamp: time.Now(),
					EventType: models.EventKeyPress,
					Data: models.MustMarshal(models.KeyEventData{
						Key: keyName(ev),
					}),
				}

			case hook.KeyUp:
				holdMs := 0
				if t, ok := pressedAt[ev.Keycode]; ok {
					holdMs = int(time.Since(t).Milliseconds())
					delete(pressedAt, ev.Keycode)
				}
				out <- &models.RawEvent{
					Timestamp: time.Now(),
					EventType: models.EventKeyRelease,
					Data: models.MustMarshal(models.KeyEventData{
						Key: keyName(ev), HoldMs: holdMs,
					}),
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

// keyName returns a human-readable name for the key in the event.
// Prefers the Unicode character if printable, otherwise falls back to the keycode.
func keyName(ev hook.Event) string {
	if ev.Keychar != 0 && ev.Keychar != 65535 {
		return string(ev.Keychar)
	}
	return fmt.Sprintf("key_%d", ev.Keycode)
}
