package cardtools_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderCardToolsContract(t *testing.T) {
	source := os.Getenv("HEARTHROOM_PROVIDER_SOURCE")
	if source == "" {
		t.Skip("HEARTHROOM_PROVIDER_SOURCE not set; cross-repository validator source agreement not checked")
	}
	provider, err := os.ReadFile(filepath.Join(source, "internal/service/cardtools/tools.go"))
	if err != nil {
		t.Fatal(err)
	}
	local, err := os.ReadFile("tools.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(provider, local) {
		t.Fatal("CLI and Provider card-tool validators differ")
	}
}
