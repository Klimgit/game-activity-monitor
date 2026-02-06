package collectors

import (
	"context"
	"time"

	"game-activity-monitor/client/internal/models"
)

// gpuCollector is a stub.
//
// Real GPU metrics require platform-specific APIs:
//   - Windows NVIDIA: NVML via github.com/NVIDIA/go-nvml
//   - Windows AMD/Intel: DXGI or WMI
//   - Linux: NVML or /sys/class/drm
//
// Implement and swap in a real provider when targeting a specific platform.
type gpuCollector struct {
	interval time.Duration
}

func newGPUCollector(interval time.Duration) *gpuCollector {
	return &gpuCollector{interval: interval}
}

func (g *gpuCollector) Name() string { return "gpu" }

func (g *gpuCollector) Start(ctx context.Context, out chan<- *models.RawEvent) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Emit zeroed metrics until a real provider is wired in.
			out <- &models.RawEvent{
				Timestamp: time.Now(),
				EventType: models.EventSystemMetrics,
				Data: models.MustMarshal(models.SystemMetricsData{
					GPUPercent:   0,
					GPUTempC:     0,
					GPUMemUsedMB: 0,
				}),
			}
		case <-ctx.Done():
			return
		}
	}
}
