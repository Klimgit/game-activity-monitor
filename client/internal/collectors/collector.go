package collectors

import (
	"context"

	hook "github.com/robotn/gohook"

	"game-activity-monitor/client/internal/config"
	"game-activity-monitor/client/internal/models"
)

// Collector is the interface every metric source must implement.
type Collector interface {
	// Start begins collection and sends events to out until ctx is cancelled.
	Start(ctx context.Context, out chan<- *models.RawEvent)
	// Name returns a unique identifier for this collector (used in logs and config).
	Name() string
}

// Manager owns a set of collectors and the shared input hook.
type Manager struct {
	collectors []Collector
	hookBus    *HookBus
}

// NewManager creates a Manager, enabling only the collectors listed in cfg.
// "cpu", "memory", "process", and "system" are all aliases for the single
// merged systemCollector that emits one complete system_metrics event per tick.
func NewManager(cfg *config.Config) *Manager {
	hookBus := newHookBus()

	enabled := make(map[string]bool, len(cfg.Collectors.Enabled))
	for _, name := range cfg.Collectors.Enabled {
		enabled[name] = true
	}

	var colls []Collector

	if enabled["mouse"] {
		colls = append(colls, newMouseCollector(hookBus))
	}
	if enabled["keyboard"] {
		colls = append(colls, newKeyboardCollector(hookBus))
	}
	// Any of the legacy names or "system" enables the merged collector.
	if enabled["cpu"] || enabled["memory"] || enabled["process"] || enabled["system"] {
		colls = append(colls, newSystemCollector(cfg.Collectors.Intervals.SystemPolling))
	}
	if enabled["gpu"] {
		colls = append(colls, newGPUCollector(cfg.Collectors.Intervals.SystemPolling))
	}

	return &Manager{collectors: colls, hookBus: hookBus}
}

// SubscribeHook returns a channel that receives every raw OS input event from
// the shared hook bus. Call this BEFORE Start() so no early events are dropped.
// The primary use case is wiring the HotkeyManager to the same hook as the
// collectors, avoiding a second concurrent hook.Start() call.
func (m *Manager) SubscribeHook() <-chan hook.Event {
	return m.hookBus.Subscribe()
}

// Start launches the global input hook and all enabled collectors.
// Events are written to out; the caller is responsible for draining it.
func (m *Manager) Start(ctx context.Context, out chan<- *models.RawEvent) {
	m.hookBus.Start(ctx)
	for _, c := range m.collectors {
		go c.Start(ctx, out)
	}
}
