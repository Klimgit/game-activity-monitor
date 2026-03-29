package hotkeys

import (
	"context"
	"log"
	"strings"
	"sync"

	hook "github.com/robotn/gohook"

	"game-activity-monitor/client/internal/config"
)

// Binding maps a parsed hotkey combo to its action name and callback.
type Binding struct {
	mods     modifiers
	key      string
	callback func()
}

// modifiers is a bitmask for Ctrl / Shift / Alt.
type modifiers uint8

const (
	modCtrl  modifiers = 1 << 0
	modShift modifiers = 1 << 1
	modAlt   modifiers = 1 << 2
)

// Manager listens to the global hook and fires callbacks when registered
// hotkey combinations are detected.
//
// It shares the same hook.Event stream as the input collectors by subscribing
// to a HookBus, but can also operate standalone with its own hook.Start()
// when the bus is not available (e.g., in tests).
type Manager struct {
	bindings []Binding
	evChan   <-chan hook.Event
	mu       sync.Mutex
	mods     modifiers // currently held modifier keys
}

// NewManagerFromBus creates a Manager that reads from the provided channel.
// Pass the channel returned by HookBus.Subscribe() so the global hook is
// shared with the input collectors. If evChan is nil, Start will call
// hook.Start() directly (standalone mode — use only when no HookBus is active).
func NewManagerFromBus(evChan <-chan hook.Event) *Manager {
	return &Manager{evChan: evChan}
}

// Register adds a hotkey → callback mapping.
// combo format: "ctrl+shift+s" (case-insensitive, order-independent).
func (m *Manager) Register(combo string, callback func()) {
	mods, key := parseCombo(combo)
	m.bindings = append(m.bindings, Binding{mods: mods, key: key, callback: callback})
}

// RegisterAll registers the standard set of hotkeys from config.
// The callbacks map key → func, e.g. "start_session" → func(){...}
func RegisterAll(m *Manager, cfg config.HotkeysConfig, actions map[string]func()) {
	pairs := []struct {
		combo  string
		action string
	}{
		{cfg.StartSession, "start_session"},
		{cfg.EndSession, "end_session"},
		{cfg.StartActiveGameplay, "start_active"},
		{cfg.EndActiveGameplay, "end_active"},
		{cfg.StartAFK, "start_afk"},
		{cfg.EndAFK, "end_afk"},
		{cfg.StartMenu, "start_menu"},
		{cfg.EndMenu, "end_menu"},
		{cfg.StartLoading, "start_loading"},
		{cfg.EndLoading, "end_loading"},
	}
	for _, p := range pairs {
		if fn, ok := actions[p.action]; ok {
			m.Register(p.combo, fn)
		}
	}
}

// Start processes hook events until ctx is cancelled.
// If no evChan was provided (standalone mode), it starts its own hook.
func (m *Manager) Start(ctx context.Context) {
	ch := m.evChan
	if ch == nil {
		// Standalone mode: start a dedicated hook (only safe when no HookBus is active).
		rawCh := hook.Start()
		defer hook.End()
		ch = rawCh
	}

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			m.handleEvent(ev)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) handleEvent(ev hook.Event) {
	switch ev.Kind {
	case hook.KeyDown, hook.KeyHold:
		mod := keyToModifier(ev.Keycode)
		if mod != 0 {
			m.mu.Lock()
			m.mods |= mod
			m.mu.Unlock()
			return
		}
		// Regular key press: check combos
		key := normalizeKey(ev)
		m.mu.Lock()
		held := m.mods
		m.mu.Unlock()

		for _, b := range m.bindings {
			if b.mods == held && b.key == key {
				log.Printf("hotkey triggered: %s+%s", modsString(held), key)
				go b.callback()
			}
		}

	case hook.KeyUp:
		mod := keyToModifier(ev.Keycode)
		if mod != 0 {
			m.mu.Lock()
			m.mods &^= mod
			m.mu.Unlock()
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// parseCombo parses "ctrl+shift+s" into a modifiers bitmask and the final key.
func parseCombo(combo string) (modifiers, string) {
	parts := strings.Split(strings.ToLower(combo), "+")
	var mods modifiers
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "ctrl", "control":
			mods |= modCtrl
		case "shift":
			mods |= modShift
		case "alt", "option":
			mods |= modAlt
		default:
			key = p
		}
	}
	return mods, key
}

// keyToModifier maps platform keycodes for modifier keys to bitmask values.
// These values are for the most common platform (Windows); they may need
// adjustment for macOS / Linux.
func keyToModifier(keycode uint16) modifiers {
	switch keycode {
	case 162, 163: // VK_LCONTROL, VK_RCONTROL (Windows)
		return modCtrl
	case 160, 161: // VK_LSHIFT, VK_RSHIFT (Windows)
		return modShift
	case 164, 165: // VK_LMENU, VK_RMENU (Windows)
		return modAlt
	case 29, 97: // ctrl on Linux/X11
		return modCtrl
	case 42, 54: // shift on Linux/X11
		return modShift
	case 56, 100: // alt on Linux/X11
		return modAlt
	}
	return 0
}

// normalizeKey returns a lower-case single character or key name.
func normalizeKey(ev hook.Event) string {
	if ev.Keychar != 0 && ev.Keychar != 65535 {
		return strings.ToLower(string(ev.Keychar))
	}
	// Fallback for non-printable keys (F-keys, arrows, etc.)
	switch ev.Keycode {
	case 13:
		return "enter"
	case 32:
		return "space"
	case 27:
		return "escape"
	case 9:
		return "tab"
	}
	return ""
}

func modsString(m modifiers) string {
	var parts []string
	if m&modCtrl != 0 {
		parts = append(parts, "ctrl")
	}
	if m&modShift != 0 {
		parts = append(parts, "shift")
	}
	if m&modAlt != 0 {
		parts = append(parts, "alt")
	}
	return strings.Join(parts, "+")
}
