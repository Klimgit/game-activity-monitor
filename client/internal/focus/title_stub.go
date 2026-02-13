//go:build !windows && !linux

package focus

func ForegroundWindowTitle() string {
	return ""
}
