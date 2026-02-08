package collectors

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"

	"game-activity-monitor/client/internal/models"
)

type systemCollector struct {
	interval   time.Duration
	collectGPU bool
}

func newSystemCollector(interval time.Duration, collectGPU bool) *systemCollector {
	return &systemCollector{interval: interval, collectGPU: collectGPU}
}

func (s *systemCollector) Name() string { return "system" }

func (s *systemCollector) Start(ctx context.Context, out chan<- *models.RawEvent) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			data, err := s.collectMetrics(ctx)
			if err != nil {
				log.Printf("system collector: %v", err)
				continue
			}
			out <- &models.RawEvent{
				Timestamp: time.Now(),
				EventType: models.EventSystemMetrics,
				Data:      models.MustMarshal(data),
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *systemCollector) collectMetrics(ctx context.Context) (models.SystemMetricsData, error) {
	var data models.SystemMetricsData

	if percents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(percents) > 0 {
		data.CPUPercent = percents[0]
	}

	if vmem, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		data.MemPercent = vmem.UsedPercent
	}

	if name, err := topCPUProcess(ctx); err == nil {
		data.ActiveProcess = name
	}

	if s.collectGPU {
		data.GPUPercent, data.GPUTempC, data.GPUMemUsedMB = collectGPUMetrics()
	}

	return data, nil
}

// topCPUProcess returns the name of the non-system process currently using
// the most CPU. Returns "idle" when no significant user processes are running.
func topCPUProcess(ctx context.Context) (string, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return "", err
	}

	type entry struct {
		name string
		pct  float64
	}
	entries := make([]entry, 0, len(procs))

	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil || isSystemProcess(name) {
			continue
		}
		pct, err := p.CPUPercentWithContext(ctx)
		if err != nil || pct < 0.1 {
			continue
		}
		entries = append(entries, entry{name, pct})
	}

	if len(entries) == 0 {
		return "idle", nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].pct > entries[j].pct
	})
	return entries[0].name, nil
}

var systemProcesses = map[string]bool{
	"System":         true,
	"Idle":           true,
	"kernel_task":    true,
	"kthreadd":       true,
	"systemd":        true,
	"launchd":        true,
	"svchost.exe":    true,
	"csrss.exe":      true,
	"wininit.exe":    true,
	"services.exe":   true,
	"lsass.exe":      true,
	"smss.exe":       true,
	"Registry":       true,
	"MemCompression": true,
}

func isSystemProcess(name string) bool {
	return systemProcesses[name]
}
