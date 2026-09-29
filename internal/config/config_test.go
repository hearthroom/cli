package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOpenHonoursEnvAndExplicitDir(t *testing.T) {
	t.Setenv("HEARTHROOM_CONFIG_DIR", "/tmp/from-env")
	s, err := Open("")
	if err != nil || s.Dir != "/tmp/from-env" {
		t.Fatalf("env dir: %v %v", s, err)
	}
	s, _ = Open("/tmp/explicit")
	if s.Dir != "/tmp/explicit" {
		t.Fatalf("explicit dir lost: %s", s.Dir)
	}
}

func TestConfigRoundTripAndMissingFile(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "nested", "dir")}
	cfg, err := s.LoadConfig()
	if err != nil || cfg.Clients == nil {
		t.Fatalf("empty load: %+v %v", cfg, err)
	}
	cfg.API = "https://api.example.test"
	cfg.Clients["https://api.example.test"] = ClientReg{ClientID: "c1", RedirectURIs: []string{"http://127.0.0.1:1/callback"}, Scope: "a b"}
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	back, err := s.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if back.API != cfg.API || back.Clients["https://api.example.test"].ClientID != "c1" {
		t.Fatalf("round trip lost data: %+v", back)
	}
}

func TestCredentialsAreOwnerOnly(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	creds := map[string]Credential{"https://a": {AccessToken: "x", ExpiresAt: time.Now().Round(time.Second)}}
	if err := s.SaveCredentials(creds); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.Dir, credentialsFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %o", info.Mode().Perm())
	}
	// Re-saving keeps the mode even if the file already exists.
	if err := s.SaveCredentials(creds); err != nil {
		t.Fatal(err)
	}
	back, err := s.LoadCredentials()
	if err != nil || back["https://a"].AccessToken != "x" {
		t.Fatalf("round trip: %+v %v", back, err)
	}
}

func TestNormalizeBase(t *testing.T) {
	if got := NormalizeBase("  https://x.test/  "); got != "https://x.test" {
		t.Fatalf("got %q", got)
	}
}

// The community site's primary domain is sukisuki.ai; hearthroom.club and sukisuki.chat
// keep serving but the CLI should talk to the canonical one by default.
func TestDefaultSiteIsThePrimaryDomain(t *testing.T) {
	if DefaultSite != "https://sukisuki.ai" {
		t.Fatalf("DefaultSite = %q, want the primary domain https://sukisuki.ai", DefaultSite)
	}
}
