package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Game is a single game profile. Source is the exact folder to back up as a
// whole unit (e.g. a save slot folder); BackupRoot is where backups are stored.
type Game struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	BackupRoot string `json:"backupRoot"`

	// LegacySourceRoot accepts the old "sourceRoot" key so existing configs
	// keep loading; it is migrated into Source on load.
	LegacySourceRoot string `json:"sourceRoot,omitempty"`
}

// Config is the persisted application configuration.
type Config struct {
	ActiveGame string `json:"activeGame"`
	Hotkey     string `json:"hotkey"`
	Port       int    `json:"port"`
	Games      []Game `json:"games"`

	path string
	mu   sync.Mutex
}

const (
	DefaultHotkey = "Ctrl+Alt+S"
	DefaultPort   = 8787
)

// Load reads config from path, creating a default config if the file is absent.
func Load(path string) (*Config, error) {
	c := &Config{path: path, Hotkey: DefaultHotkey, Port: DefaultPort}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, c.Save()
		}
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	c.path = path
	if c.Hotkey == "" {
		c.Hotkey = DefaultHotkey
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	for i := range c.Games {
		if c.Games[i].Source == "" && c.Games[i].LegacySourceRoot != "" {
			c.Games[i].Source = c.Games[i].LegacySourceRoot
		}
		c.Games[i].LegacySourceRoot = ""
	}
	return c, nil
}

// Save writes the config atomically to disk.
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Path returns the config file path.
func (c *Config) Path() string { return c.path }

// Active returns the active game name under the lock, safe to call
// concurrently with UpsertGame/DeleteGame/SetActive.
func (c *Config) Active() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ActiveGame
}

// MarshalJSON serializes the config under the lock so that encoding it (e.g.
// for the /api/config response or Save) never races with concurrent mutations
// of the Games slice or ActiveGame. The unexported mutex/path fields are
// naturally excluded.
func (c *Config) MarshalJSON() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	type alias struct {
		ActiveGame string `json:"activeGame"`
		Hotkey     string `json:"hotkey"`
		Port       int    `json:"port"`
		Games      []Game `json:"games"`
	}
	return json.Marshal(alias{
		ActiveGame: c.ActiveGame,
		Hotkey:     c.Hotkey,
		Port:       c.Port,
		Games:      c.Games,
	})
}

// Game returns the game profile with the given name.
func (c *Config) Game(name string) (Game, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, g := range c.Games {
		if g.Name == name {
			return g, true
		}
	}
	return Game{}, false
}

// UpsertGame adds a new game profile or updates an existing one by name.
func (c *Config) UpsertGame(g Game) error {
	if g.Name == "" {
		return errors.New("game name is required")
	}
	if g.Source == "" {
		return errors.New("source folder is required")
	}
	if g.BackupRoot == "" {
		return errors.New("backupRoot is required")
	}
	c.mu.Lock()
	found := false
	for i := range c.Games {
		if c.Games[i].Name == g.Name {
			c.Games[i] = g
			found = true
			break
		}
	}
	if !found {
		c.Games = append(c.Games, g)
	}
	if c.ActiveGame == "" {
		c.ActiveGame = g.Name
	}
	c.mu.Unlock()
	return c.Save()
}

// DeleteGame removes a game profile by name.
func (c *Config) DeleteGame(name string) error {
	c.mu.Lock()
	out := c.Games[:0]
	for _, g := range c.Games {
		if g.Name != name {
			out = append(out, g)
		}
	}
	c.Games = out
	if c.ActiveGame == name {
		c.ActiveGame = ""
		if len(c.Games) > 0 {
			c.ActiveGame = c.Games[0].Name
		}
	}
	c.mu.Unlock()
	return c.Save()
}

// SetHotkey updates the global backup hotkey and persists it. Validation of
// the hotkey string format is done by the caller (via hotkey.Parse).
func (c *Config) SetHotkey(h string) error {
	c.mu.Lock()
	c.Hotkey = h
	c.mu.Unlock()
	return c.Save()
}

// SetPort validates and persists the web UI port. It takes effect on the next
// launch, since the listener is bound at startup.
func (c *Config) SetPort(p int) error {
	if p < 1 || p > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	c.mu.Lock()
	c.Port = p
	c.mu.Unlock()
	return c.Save()
}

// SetActive sets the active game after verifying it exists.
func (c *Config) SetActive(name string) error {
	if _, ok := c.Game(name); !ok {
		return errors.New("game not found: " + name)
	}
	c.mu.Lock()
	c.ActiveGame = name
	c.mu.Unlock()
	return c.Save()
}
