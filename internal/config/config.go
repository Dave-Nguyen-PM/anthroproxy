package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Token struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	BearerToken string `json:"bearer_token"`
}

type Config struct {
	Tokens          []Token `json:"tokens"`
	Listen          string  `json:"listen"`
	CooldownMinutes int     `json:"cooldown_minutes"`
}

func DefaultConfig() *Config {
	return &Config{
		Tokens:          []Token{},
		Listen:          "0.0.0.0:8080",
		CooldownMinutes: 30,
	}
}

func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "anthropool-proxy")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "anthropool-proxy")
	}
	return filepath.Join(home, ".config", "anthropool-proxy")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	path := ConfigPath()

	// Open or create lock file
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer lockFile.Close()
	defer os.Remove(lockPath)

	// Exclusive lock
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	// Atomic write via tmp + rename
	tmp, err := os.CreateTemp(dir, "config.*.tmp")
	if err != nil {
		return fmt.Errorf("creating tmp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // no-op if rename succeeded
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing tmp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing tmp file: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming tmp file: %w", err)
	}
	return nil
}

func MaskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}
