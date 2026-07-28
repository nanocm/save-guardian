package hotkey

import "testing"

func TestParseValid(t *testing.T) {
	cases := []struct {
		spec string
		mods uint
		vk   uint
	}{
		{"Ctrl+Alt+S", modControl | modAlt, 0x53},
		{"ctrl+alt+s", modControl | modAlt, 0x53},        // case-insensitive
		{"Control+Shift+A", modControl | modShift, 0x41}, // "control" alias
		{"Win+D", modWin, 0x44},                          // win alias
		{"Cmd+D", modWin, 0x44},                          // cmd -> win
		{"Super+D", modWin, 0x44},                        // super -> win
		{"Meta+D", modWin, 0x44},                         // meta -> win
		{"Ctrl+Alt+F5", modControl | modAlt, 0x74},       // F5 = 0x70 + 4
		{"F12", 0, 0x7B},                                 // F12 = 0x70 + 11
		{"Ctrl+1", modControl, 0x31},                     // digit
		{"Alt+Space", modAlt, 0x20},                      // named key
		{"Shift+Enter", modShift, 0x0D},
		{"Ctrl+Home", modControl, 0x24},
		{"Ctrl+PgUp", modControl, 0x21}, // alias for pageup
		{"Ctrl+Alt+Shift+Win+K", modControl | modAlt | modShift | modWin, 0x4B},
	}
	for _, c := range cases {
		p, err := Parse(c.spec)
		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c.spec, err)
			continue
		}
		if p.Mods != c.mods || p.VK != c.vk {
			t.Errorf("Parse(%q) = {mods:%#x vk:%#x}, want {mods:%#x vk:%#x}",
				c.spec, p.Mods, p.VK, c.mods, c.vk)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		spec string
	}{
		{"empty string", ""},
		{"modifiers only, no main key", "Ctrl+Alt"},
		{"unsupported symbol key", "Ctrl+@"},
		{"function key out of range", "F25"},
		{"unknown named key", "Ctrl+Foo"},
	}
	for _, c := range cases {
		if _, err := Parse(c.spec); err == nil {
			t.Errorf("%s: Parse(%q) expected error, got nil", c.name, c.spec)
		}
	}
}
