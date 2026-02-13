package aggregator

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/vcaesar/keycode"

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
						if sys.ForegroundWindowTitle != "" {
							acc.lastForegroundTitle = sys.ForegroundWindowTitle
						}
						acc.addHardwareSample(sys.CPUPercent, sys.MemPercent, sys.GPUPercent, sys.GPUTempC, sys.GPUMemUsedMB)
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

type windowAccumulator struct {
	mouseMoves          int
	mouseClicks         int
	speedSum            float64
	speedMax            float64
	keystrokes          int
	holdMsSum           float64
	holdCount           int
	lastKeyPress        time.Time
	keyGapSumMs         float64
	keyGapCount         int
	keyW                int
	keyA                int
	keyS                int
	keyD                int
	lastProcess         string
	lastForegroundTitle string
	accelSum            float64
	accelMaxAbs         float64
	cpuSum              float64
	cpuMax              float64
	memSum              float64
	gpuUtilSum          float64
	gpuTempSum          float64
	gpuMemSumMB         float64
	hwCount             int
}

func (w *windowAccumulator) reset() {
	*w = windowAccumulator{}
}

func (w *windowAccumulator) hasData() bool {
	return w.mouseMoves > 0 || w.keystrokes > 0 || w.mouseClicks > 0 || w.hwCount > 0
}

func (w *windowAccumulator) addHardwareSample(cpu, mem, gpuUtil, gpuTemp float64, gpuMemMB int64) {
	w.hwCount++
	w.cpuSum += cpu
	if cpu > w.cpuMax {
		w.cpuMax = cpu
	}
	w.memSum += mem
	w.gpuUtilSum += gpuUtil
	w.gpuTempSum += gpuTemp
	w.gpuMemSumMB += float64(gpuMemMB)
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
		w.accelSum += d.Acceleration
		a := math.Abs(d.Acceleration)
		if a > w.accelMaxAbs {
			w.accelMaxAbs = a
		}

	case models.EventKeyPress:
		var d models.KeyEventData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return
		}
		w.keystrokes++
		countWASD(d.Key, &w.keyW, &w.keyA, &w.keyS, &w.keyD)
		if !w.lastKeyPress.IsZero() {
			gap := ev.Timestamp.Sub(w.lastKeyPress)
			w.keyGapSumMs += float64(gap.Milliseconds())
			w.keyGapCount++
		}
		w.lastKeyPress = ev.Timestamp

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

func countWASD(key string, w, a, s, d *int) {
	k := strings.TrimSpace(strings.ToLower(key))
	if len(k) == 1 {
		switch k[0] {
		case 'w':
			*w++
		case 'a':
			*a++
		case 's':
			*s++
		case 'd':
			*d++
		}
		return
	}
	if !strings.HasPrefix(k, "key_") {
		return
	}
	u, err := strconv.ParseUint(strings.TrimPrefix(k, "key_"), 10, 16)
	if err != nil {
		return
	}
	code := uint16(u)
	switch code {
	case keycode.Keycode["w"]:
		*w++
	case keycode.Keycode["a"]:
		*a++
	case keycode.Keycode["s"]:
		*s++
	case keycode.Keycode["d"]:
		*d++
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

	var keyPressIntervalAvg float64
	if w.keyGapCount > 0 {
		keyPressIntervalAvg = w.keyGapSumMs / float64(w.keyGapCount)
	}

	var cursorAccelAvg float64
	if w.mouseMoves > 0 {
		cursorAccelAvg = w.accelSum / float64(w.mouseMoves)
	}

	var cpuAvg, memAvg, gpuUtilAvg, gpuTempAvg, gpuMemAvgMB float64
	if w.hwCount > 0 {
		n := float64(w.hwCount)
		cpuAvg = w.cpuSum / n
		memAvg = w.memSum / n
		gpuUtilAvg = w.gpuUtilSum / n
		gpuTempAvg = w.gpuTempSum / n
		gpuMemAvgMB = w.gpuMemSumMB / n
	}

	return &models.RawEvent{
		Timestamp: end,
		EventType: models.EventWindowMetrics,
		Data: models.MustMarshal(models.WindowMetricsData{
			WindowStart:           start,
			WindowEnd:             end,
			DurationS:             end.Sub(start).Seconds(),
			MouseMoves:            w.mouseMoves,
			MouseClicks:           w.mouseClicks,
			SpeedAvg:              speedAvg,
			SpeedMax:              w.speedMax,
			Keystrokes:            w.keystrokes,
			KeyHoldAvgMs:          holdAvg,
			KeyPressIntervalAvgMs: keyPressIntervalAvg,
			KeyW:                  w.keyW,
			KeyA:                  w.keyA,
			KeyS:                  w.keyS,
			KeyD:                  w.keyD,
			ActiveProcess:         w.lastProcess,
			ForegroundWindowTitle: w.lastForegroundTitle,
			CursorAccelAvg:        cursorAccelAvg,
			CursorAccelMax:        w.accelMaxAbs,
			CPUAvg:                cpuAvg,
			CPUMax:                w.cpuMax,
			MemAvg:                memAvg,
			GPUUtilAvg:            gpuUtilAvg,
			GPUTempAvg:            gpuTempAvg,
			GPUMemAvgMB:           gpuMemAvgMB,
		}),
	}
}
