package collectors

import (
	"context"
	"math"
	"time"

	hook "github.com/robotn/gohook"

	"game-activity-monitor/client/internal/models"
)

type mouseCollector struct {
	bus *HookBus
}

func newMouseCollector(bus *HookBus) *mouseCollector {
	return &mouseCollector{bus: bus}
}

func (m *mouseCollector) Name() string { return "mouse" }

// minMoveInterval caps mouse_move emission to 10 events/second (100 ms between
// events). Raw OS mouse events fire hundreds of times per second during gameplay;
// without throttling they dominate the SQLite buffer and network traffic.
const minMoveInterval = 100 * time.Millisecond

func (m *mouseCollector) Start(ctx context.Context, out chan<- *models.RawEvent) {
	ch := m.bus.Subscribe()

	var (
		lastX, lastY int16
		lastMoveTime time.Time
		lastEmitTime time.Time
	)

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Kind {
			case hook.MouseMove, hook.MouseDrag:
				now := time.Now()

				// Always track position/time for accurate speed calculation.
				speed := 0.0
				if !lastMoveTime.IsZero() {
					dt := now.Sub(lastMoveTime).Seconds()
					if dt > 0 {
						dx := float64(ev.X - lastX)
						dy := float64(ev.Y - lastY)
						speed = math.Sqrt(dx*dx+dy*dy) / dt
					}
				}
				lastX, lastY = ev.X, ev.Y
				lastMoveTime = now

				// Throttle: skip this event if we emitted too recently.
				if now.Sub(lastEmitTime) < minMoveInterval {
					continue
				}
				lastEmitTime = now

				out <- &models.RawEvent{
					Timestamp: now,
					EventType: models.EventMouseMove,
					Data: models.MustMarshal(models.MouseMoveData{
						X: int(ev.X), Y: int(ev.Y), Speed: speed,
					}),
				}

			case hook.MouseDown:
				btn := buttonName(ev.Button)
				out <- &models.RawEvent{
					Timestamp: time.Now(),
					EventType: models.EventMouseClick,
					Data: models.MustMarshal(models.MouseClickData{
						X: int(ev.X), Y: int(ev.Y), Button: btn,
					}),
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func buttonName(b uint16) string {
	switch b {
	case 1:
		return "left"
	case 2:
		return "right"
	case 3:
		return "middle"
	default:
		return "unknown"
	}
}
