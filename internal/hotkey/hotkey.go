package hotkey

import (
	"fmt"
	"strings"
)

// Windows modifier flags for RegisterHotKey.
const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008
)

// Parsed represents a hotkey combination.
type Parsed struct {
	Mods uint // combination of mod* flags
	VK   uint // virtual key code
}

// Parse converts a string like "Ctrl+Alt+S" into modifiers and a virtual key.
func Parse(s string) (Parsed, error) {
	var p Parsed
	parts := strings.Split(s, "+")
	if len(parts) == 0 {
		return p, fmt.Errorf("empty hotkey")
	}
	for _, raw := range parts {
		key := strings.TrimSpace(strings.ToLower(raw))
		switch key {
		case "ctrl", "control":
			p.Mods |= modControl
		case "alt":
			p.Mods |= modAlt
		case "shift":
			p.Mods |= modShift
		case "win", "cmd", "super", "meta":
			p.Mods |= modWin
		case "":
			continue
		default:
			vk, ok := virtualKey(key)
			if !ok {
				return p, fmt.Errorf("unsupported key: %q", raw)
			}
			p.VK = vk
		}
	}
	if p.VK == 0 {
		return p, fmt.Errorf("hotkey has no main key: %q", s)
	}
	return p, nil
}

// virtualKey maps a key name to a Windows virtual key code.
func virtualKey(key string) (uint, bool) {
	if len(key) == 1 {
		c := key[0]
		if c >= 'a' && c <= 'z' {
			return uint(c-'a') + 0x41, true
		}
		if c >= '0' && c <= '9' {
			return uint(c-'0') + 0x30, true
		}
	}
	if len(key) >= 2 && key[0] == 'f' {
		// Function keys F1-F24 -> 0x70..0x87
		var n int
		if _, err := fmt.Sscanf(key, "f%d", &n); err == nil && n >= 1 && n <= 24 {
			return uint(0x70 + (n - 1)), true
		}
	}
	switch key {
	case "space":
		return 0x20, true
	case "enter", "return":
		return 0x0D, true
	case "insert", "ins":
		return 0x2D, true
	case "home":
		return 0x24, true
	case "end":
		return 0x23, true
	case "pageup", "pgup":
		return 0x21, true
	case "pagedown", "pgdn":
		return 0x22, true
	}
	return 0, false
}
