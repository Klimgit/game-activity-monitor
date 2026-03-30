package aggregator

import (
	"context"
	"encoding/json"
	"time"

	"game-activity-monitor/client/internal/models"
)

type Aggregator struct {
	window time.Duration
	In     <-chan *models.RawEvent
	Out    chan<- *models.RawEvent
}

func New(window time.Duration, in <-chan *models.RawEvent, out chan<- *models.RawEvent) *Aggregator {
	return &Aggregator{window: window, In: in, Out: out}
}

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
				a.Out <- ev
				if ev.EventType == models.EventMouseClick {
					acc.mouseClicks++
				} else {
					var sys models.SystemMetricsData
					if err := json.Unmarshal(ev.Data, &sys); err == nil {
						if sys.ActiveProcess != "" {
							acc.lastProcess = sys.ActiveProcess
						}
						acc.addHardwareSample(sys.CPUPercent, sys.MemPercent, sys.GPUPercent, sys.GPUTempC)
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
	mouseClicks int
	speedSum    float64
	speedMax    float64
	keystrokes  int
	holdMsSum   float64
	holdCount   int
	lastProcess string
	cpuSum      float64
	cpuMax      float64
	memSum      float64
	gpuUtilSum  float64
	gpuTempSum  float64
	hwCount     int
}

func (w *windowAccumulator) reset() {
	*w = windowAccumulator{}
}

func (w *windowAccumulator) hasData() bool {
	return w.mouseMoves > 0 || w.keystrokes > 0 || w.mouseClicks > 0 || w.hwCount > 0
}

func (w *windowAccumulator) addHardwareSample(cpu, mem, gpuUtil, gpuTemp float64) {
	w.hwCount++
	w.cpuSum += cpu
	if cpu > w.cpuMax {
		w.cpuMax = cpu
	}
	w.memSum += mem
	w.gpuUtilSum += gpuUtil
	w.gpuTempSum += gpuTemp
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
		w.keystrokes++

	case models.EventKeyRelease:
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

	var cpuAvg, memAvg, gpuUtilAvg, gpuTempAvg float64
	if w.hwCount > 0 {
		n := float64(w.hwCount)
		cpuAvg = w.cpuSum / n
		memAvg = w.memSum / n
		gpuUtilAvg = w.gpuUtilSum / n
		gpuTempAvg = w.gpuTempSum / n
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
			CPUAvg:        cpuAvg,
			CPUMax:        w.cpuMax,
			MemAvg:        memAvg,
			GPUUtilAvg:    gpuUtilAvg,
			GPUTempAvg:    gpuTempAvg,
		}),
	}
}
