//go:build !windows && !linux

package focus

// ForegroundWindowTitle returns empty on platforms without a supported implementation.
func ForegroundWindowTitle() string {
	return ""
}
