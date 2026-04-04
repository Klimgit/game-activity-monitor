//go:build linux

package focus

import (
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
)

const titleMaxChars = 512

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
