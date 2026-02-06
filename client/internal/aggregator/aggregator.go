// Package aggregator reduces the volume of events forwarded to the server by
// accumulating mouse-move and keyboard events into fixed-duration windows and
// emitting one compact window_metrics event per window.
//
// Pass-through policy (events forwarded immediately without buffering):
//   - mouse_click  — individual positions are required by the heatmap.
//   - system_metrics — low-volume (one per system_polling tick) and needed by
//     the real-time dashboard.
//
// Aggregated (consumed, never forwarded individually):
//   - mouse_move   — up to 10/s; replaced by per-window avg/max speed.
//   - key_press / key_release — replaced by per-window keystroke count and
//     average hold duration.
package aggregator

import (
	"context"
	"encoding/json"
	"time"

	"game-activity-monitor/client/internal/models"
)

// Aggregator sits between the collector output channel and the forwardEvents
// goroutine.  It reads from In and writes to Out.
type Aggregator struct {
	window time.Duration
	In     <-chan *models.RawEvent
	Out    chan<- *models.RawEvent
}

// New creates an Aggregator.  window is the aggregation interval (e.g. 30 s).
// in and out are the raw-event channel from collectors and the downstream
// channel to the API client respectively.
func New(window time.Duration, in <-chan *models.RawEvent, out chan<- *models.RawEvent) *Aggregator {
	return &Aggregator{window: window, In: in, Out: out}
}

// Run processes events until ctx is cancelled.  It must be called in its own
// goroutine.  On cancellation it flushes the current partial window so no
// data is silently dropped.
func (a *Aggregator) Run(ctx context.Context) {
	ticker := time.NewTicker(a.window)
	defer ticker.Stop()

	var acc windowAccumulator
	windowStart := time.Now()

	flush := func() {
		now := time.Now()
		if acc.hasData() {
			a.Out <- acc.toEvent(windowStart, now)
		}
		acc.reset()
		windowStart = now
	}

	for {
		select {
		case ev, ok := <-a.In:
			if !ok {
				flush()
				return
			}
			switch ev.EventType {
			case models.EventMouseClick, models.EventSystemMetrics:
				// Forward immediately.
				a.Out <- ev
				if ev.EventType == models.EventMouseClick {
					acc.mouseClicks++
				} else {
					// Extract the active process name so the window summary can
					// carry it even though system_metrics events pass through.
					var sys models.SystemMetricsData
					if err := json.Unmarshal(ev.Data, &sys); err == nil && sys.ActiveProcess != "" {
						acc.lastProcess = sys.ActiveProcess
					}
				}
			default:
				acc.ingest(ev)
			}

		case <-ticker.C:
			flush()

		case <-ctx.Done():
			flush()
			return
		}
	}
}

// ─── window accumulator ───────────────────────────────────────────────────────

type windowAccumulator struct {
	mouseMoves  int
	mouseClicks int // clicks forwarded individually; counted here for the window summary
	speedSum    float64
	speedMax    float64
	keystrokes  int
	holdMsSum   float64
	holdCount   int
	lastProcess string
}

func (w *windowAccumulator) reset() {
	*w = windowAccumulator{}
}

func (w *windowAccumulator) hasData() bool {
	return w.mouseMoves > 0 || w.keystrokes > 0 || w.mouseClicks > 0
}

func (w *windowAccumulator) ingest(ev *models.RawEvent) {
	switch ev.EventType {
	case models.EventMouseMove:
		var d models.MouseMoveData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return
		}
		w.mouseMoves++
		w.speedSum += d.Speed
		if d.Speed > w.speedMax {
			w.speedMax = d.Speed
		}

	case models.EventKeyPress:
		// Count each initial key-down as one keystroke.  The keyboard collector
		// does not populate HoldMs on press — that arrives with key_release.
		w.keystrokes++

	case models.EventKeyRelease:
		// Hold duration is measured by the keyboard collector on key-up.
		var d models.KeyEventData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return
		}
		if d.HoldMs > 0 {
			w.holdMsSum += float64(d.HoldMs)
			w.holdCount++
		}
	}
}

func (w *windowAccumulator) toEvent(start, end time.Time) *models.RawEvent {
	var speedAvg float64
	if w.mouseMoves > 0 {
		speedAvg = w.speedSum / float64(w.mouseMoves)
	}

	var holdAvg float64
	if w.holdCount > 0 {
		holdAvg = w.holdMsSum / float64(w.holdCount)
	}

	return &models.RawEvent{
		Timestamp: end,
		EventType: models.EventWindowMetrics,
		Data: models.MustMarshal(models.WindowMetricsData{
			WindowStart:   start,
			WindowEnd:     end,
			DurationS:     end.Sub(start).Seconds(),
			MouseMoves:    w.mouseMoves,
			MouseClicks:   w.mouseClicks,
			SpeedAvg:      speedAvg,
			SpeedMax:      w.speedMax,
			Keystrokes:    w.keystrokes,
			KeyHoldAvgMs:  holdAvg,
			ActiveProcess: w.lastProcess,
		}),
	}
}
