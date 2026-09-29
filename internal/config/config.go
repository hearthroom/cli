// Package config owns the on-disk configuration and credential store.
//
// Layout under the config directory (os.UserConfigDir()/hearthroom, or
// HEARTHROOM_CONFIG_DIR):
//
//	config.json       provider/site defaults and registered OAuth clients
//	credentials.json  tokens per API base, mode 0600
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultSite is the community site.
	DefaultSite = "https://hearthroom.club"
	// DefaultAPI is the API base of the provider the community site lists as
	// its default ("harbor"). Override with --api or HEARTHROOM_API.
	DefaultAPI = "https://api.harperharbor.com"

	configFile      = "config.json"
	credentialsFile = "credentials.json"
)

// Config is the persisted, non-secret configuration.
type Config struct {
	API     string               `json:"api,omitempty"`
	Site    string               `json:"site,omitempty"`
	Clients map[string]ClientReg `json:"clients,omitempty"` // keyed by API base
}

// ClientReg is a dynamically registered OAuth client for one API base.
type ClientReg struct {
	ClientID     string   `json:"client_id"`
	RedirectURIs []string `json:"redirect_uris"`
	Scope        string   `json:"scope"`
	// Dynamic marks a client the CLI registered itself; it is replaced when a
	// known first-party client for the API base becomes available.
	Dynamic bool `json:"dynamic,omitempty"`
}

// Credential is one stored token set for an API base.
type Credential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	Resource     string    `json:"resource,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
}

// Store reads and writes the two files in one directory.
type Store struct {
	Dir string
}

// Open resolves the config directory. An explicit dir wins, then
// HEARTHROOM_CONFIG_DIR, then the platform user config dir.
func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = os.Getenv("HEARTHROOM_CONFIG_DIR")
	}
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("resolve config dir: %w", err)
		}
		dir = filepath.Join(base, "hearthroom")
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) ensure() error {
	return os.MkdirAll(s.Dir, 0o700)
}

// LoadConfig returns the stored config, or an empty one when none exists.
func (s *Store) LoadConfig() (Config, error) {
	var cfg Config
	err := readJSON(filepath.Join(s.Dir, configFile), &cfg)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if cfg.Clients == nil {
		cfg.Clients = map[string]ClientReg{}
	}
	return cfg, nil
}

// SaveConfig writes config.json atomically.
func (s *Store) SaveConfig(cfg Config) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.Dir, configFile), cfg, 0o644)
}

// LoadCredentials returns all stored credentials keyed by API base.
func (s *Store) LoadCredentials() (map[string]Credential, error) {
	creds := map[string]Credential{}
	err := readJSON(filepath.Join(s.Dir, credentialsFile), &creds)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return creds, nil
}

// SaveCredentials writes credentials.json with owner-only permissions.
func (s *Store) SaveCredentials(creds map[string]Credential) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.Dir, credentialsFile), creds, 0o600)
}

// NormalizeBase trims whitespace and a trailing slash from a URL base.
func NormalizeBase(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any, mode os.FileMode) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), mode); err != nil {
		return err
	}
	// WriteFile applies the umask and does not chmod an existing file.
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
