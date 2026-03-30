package collectors

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func collectGPUMetrics() (utilPct float64, tempC float64, memUsedMB int64) {
	if u, t, m, ok := probeNvidiaSMI(); ok {
		return u, t, m
	}
	if runtime.GOOS == "linux" {
		if u, ok := probeLinuxDRM(); ok {
			return u, 0, 0
		}
	}
	return 0, 0, 0
}

const nvidiaSMITimeout = 3 * time.Second

func probeNvidiaSMI() (utilPct float64, tempC float64, memUsedMB int64, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), nvidiaSMITimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx,
		"nvidia-smi",
		"--query-gpu=utilization.gpu,temperature.gpu,memory.used",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return
	}

	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	fields := strings.Split(line, ", ")
	if len(fields) < 3 {
		return
	}

	u, err1 := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
	t, err2 := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
	m, err3 := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}

	return u, t, int64(m), true
}

func probeLinuxDRM() (utilPct float64, ok bool) {
	for _, card := range []string{"card0", "card1", "card2", "card3"} {
		path := "/sys/class/drm/" + card + "/device/gpu_busy_percent"
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return
}
