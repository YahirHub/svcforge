package svcforge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManifestAtomicRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(t.TempDir(), "state", "manifest.json")
	manifest := Manifest{
		Schema:         ManifestSchema,
		AppID:          "com.example.demo",
		Name:           "demo",
		Version:        "1.2.3",
		BinaryPath:     filepath.Join(t.TempDir(), "demo"),
		ServiceManager: "openrc",
		ServiceName:    "demo",
		InstalledAt:    now,
		UpdatedAt:      now,
		ManagedFiles:   []string{filepath.Join(t.TempDir(), "demo.conf")},
	}
	if err := WriteManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("manifest mode = %o, want 600", got)
	}
	loaded, err := ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AppID != manifest.AppID || loaded.Version != manifest.Version || !loaded.UpdatedAt.Equal(now) {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
}

func TestReadManifestNotInstalled(t *testing.T) {
	_, err := ReadManifest(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("error = %v", err)
	}
}
