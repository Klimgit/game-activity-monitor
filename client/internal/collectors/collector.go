package collectors

import (
	"context"

	hook "github.com/robotn/gohook"

	"game-activity-monitor/client/internal/config"
	"game-activity-monitor/client/internal/models"
)

type Collector interface {
	Start(ctx context.Context, out chan<- *models.RawEvent)
	Name() string
}
type Manager struct {
	collectors []Collector
	hookBus    *HookBus
}

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
	wantSystem := enabled["cpu"] || enabled["memory"] || enabled["process"] || enabled["system"] || enabled["gpu"]
	if wantSystem {
		colls = append(colls, newSystemCollector(cfg.Collectors.Intervals.SystemPolling, enabled["gpu"]))
	}

	return &Manager{collectors: colls, hookBus: hookBus}
}

func (m *Manager) SubscribeHook() <-chan hook.Event {
	return m.hookBus.Subscribe()
}

func (m *Manager) Start(ctx context.Context, out chan<- *models.RawEvent) {
	m.hookBus.Start(ctx)
	for _, c := range m.collectors {
		go c.Start(ctx, out)
	}
}
