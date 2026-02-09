//go:build linux

package focus

import (
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
)

const titleMaxChars = 512

// ForegroundWindowTitle returns the active window title on X11 via EWMH (empty on Wayland or if DISPLAY is missing).
func ForegroundWindowTitle() string {
	X, err := xgbutil.NewConn()
	if err != nil {
		return ""
	}
	defer X.Conn().Close()

	wid, err := ewmh.ActiveWindowGet(X)
	if err != nil || wid == 0 {
		return ""
	}
	name, err := ewmh.WmNameGet(X, wid)
	if err != nil || name == "" {
		name, err = icccm.WmNameGet(X, wid)
		if err != nil {
			return ""
		}
	}
	return truncateTitle(name, titleMaxChars)
}
