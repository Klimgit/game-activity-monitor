package collectors

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// collectGPUMetrics probes the host for GPU utilization, temperature and
// memory usage.  It tries two detection strategies in order:
//
//  1. nvidia-smi (NVIDIA drivers must be installed on Windows or Linux).
//     Runs as a subprocess – designed to be called this way and typically
//     returns within ~100 ms.
//
//  2. Linux sysfs (/sys/class/drm/card0/device/) for AMD/Intel iGPU.
//     Only utilization percent is available via this path; temp and memory
//     require vendor-specific hwmon paths which vary per kernel version.
//
// Returns zero values when no supported GPU is detected so callers never
// need to handle an error branch.
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

// probeNvidiaSMI calls nvidia-smi with CSV output and parses the first GPU's
// utilization, temperature and used memory.  Returns ok=false if the binary
// is not found or returns a non-zero exit code (e.g. no NVIDIA GPU present).
func probeNvidiaSMI() (utilPct float64, tempC float64, memUsedMB int64, ok bool) {
	out, err := exec.Command(
		"nvidia-smi",
		"--query-gpu=utilization.gpu,temperature.gpu,memory.used",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return
	}

	// nvidia-smi separates fields with ", ".
	// For a single GPU the output is one line: "82, 71, 6144\n"
	// For multiple GPUs each GPU is on its own line; we use only the first.
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

// probeLinuxDRM reads AMD/Intel utilization from the kernel's DRM sysfs
// interface.  Checks card0 through card3 to handle systems with multiple
// devices.  Returns ok=false when no readable entry is found.
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
