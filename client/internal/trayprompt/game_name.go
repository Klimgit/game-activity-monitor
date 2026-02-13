package trayprompt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const entryTitle = "Game Activity Monitor"
const entryLabel = "Game name (optional; you can set or edit it in the web dashboard later):"

func psSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func GameName(ctx context.Context, defaultName string) (value string, ok bool, err error) {
	switch runtime.GOOS {
	case "linux":
		return gameNameLinux(ctx, defaultName)
	case "windows":
		return gameNameWindows(ctx, defaultName)
	case "darwin":
		return gameNameDarwin(ctx, defaultName)
	default:
		return "", false, errors.New("game name dialog: unsupported OS")
	}
}

func gameNameLinux(ctx context.Context, defaultName string) (string, bool, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		return zenityEntry(ctx, path, defaultName)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return kdialogEntry(ctx, path, defaultName)
	}
	return "", false, errors.New("install zenity or kdialog for the optional game name dialog on Linux")
}

func zenityEntry(ctx context.Context, zenityPath, defaultName string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, zenityPath,
		"--entry",
		"--title="+entryTitle,
		"--text="+entryLabel,
		"--entry-text="+defaultName,
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", false, nil // user cancelled
		}
		return "", false, err
	}
	s := strings.TrimSpace(string(out))
	return s, true, nil
}

func kdialogEntry(ctx context.Context, kdialogPath, defaultName string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, kdialogPath,
		"--title", entryTitle,
		"--inputbox", entryLabel, defaultName,
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	s := strings.TrimSpace(string(out))
	return s, true, nil
}

func powershellExecutable() (string, error) {
	for _, name := range []string{"powershell.exe", "powershell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("powershell not found in PATH")
}

func gameNameWindows(ctx context.Context, defaultName string) (string, bool, error) {
	psPath, err := powershellExecutable()
	if err != nil {
		return "", false, err
	}

	safe := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, defaultName)

	prompt := psSingleQuoted(entryLabel)
	title := psSingleQuoted(entryTitle)
	psScript := "Add-Type -AssemblyName Microsoft.VisualBasic; " +
		"$d = $env:GMS_DEFAULT; " +
		"if ($null -eq $d) { $d = '' }; " +
		"[Microsoft.VisualBasic.Interaction]::InputBox('" + prompt + "','" + title + "',$d)"

	cmd := exec.CommandContext(ctx, psPath, "-NoProfile", "-STA", "-Command", psScript)
	cmd.Env = append(os.Environ(), "GMS_DEFAULT="+safe)

	out, err := cmd.Output()
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

func gameNameDarwin(ctx context.Context, defaultName string) (string, bool, error) {
	script := `on run argv
	set def to item 1 of argv
	set r to display dialog "Game name (optional, or set later in the web dashboard):" default answer def with title "Game Activity Monitor"
	return text returned of r
end run`

	cmd := exec.CommandContext(ctx, "osascript", "-e", script, defaultName)
	out, err := cmd.Output()
	if err != nil {
		// User cancelled the dialog (-128) or osascript error.
		return "", false, nil
	}
	return strings.TrimSpace(string(out)), true, nil
}
