package svcforge

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Manifest records the installation state owned by SvcForge.
type Manifest struct {
	Schema         int       `json:"schema"`
	AppID          string    `json:"app_id"`
	Name           string    `json:"name"`
	Version        string    `json:"version,omitempty"`
	BinaryPath     string    `json:"binary_path"`
	BinarySHA256   string    `json:"binary_sha256,omitempty"`
	ServiceManager string    `json:"service_manager,omitempty"`
	ServiceName    string    `json:"service_name,omitempty"`
	InstalledAt    time.Time `json:"installed_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ManagedFiles   []string  `json:"managed_files,omitempty"`
}

// ReadManifest loads and validates an install manifest.
func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{}, ErrNotInstalled
		}
		return Manifest{}, fmt.Errorf("read install manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode install manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("invalid install manifest: %w", err)
	}
	return manifest, nil
}

// WriteManifest atomically writes an install manifest with restrictive permissions.
func WriteManifest(path string, manifest Manifest) error {
	if !filepath.IsAbs(path) {
		return errors.New("manifest path must be absolute")
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("refuse to write invalid install manifest: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".svcforge-manifest-*")
	if err != nil {
		return fmt.Errorf("create temporary manifest: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		_ = tmp.Close()
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod temporary manifest: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("encode install manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary manifest: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish install manifest: %w", err)
	}
	removeTemp = false
	if runtime.GOOS != "windows" {
		if dirHandle, err := os.Open(dir); err == nil {
			_ = dirHandle.Sync()
			_ = dirHandle.Close()
		}
	}
	return nil
}

// Validate checks manifest schema and required identity fields.
func (m Manifest) Validate() error {
	if m.Schema != ManifestSchema {
		return fmt.Errorf("unsupported manifest schema %d", m.Schema)
	}
	if !identifierPattern.MatchString(m.AppID) {
		return errors.New("manifest app ID is invalid")
	}
	if !identifierPattern.MatchString(m.Name) {
		return errors.New("manifest app name is invalid")
	}
	if m.Version != "" {
		if _, err := ParseVersion(m.Version); err != nil {
			return fmt.Errorf("manifest version: %w", err)
		}
	}
	if m.BinarySHA256 != "" {
		if len(m.BinarySHA256) != 64 {
			return errors.New("manifest binary SHA-256 is invalid")
		}
		if _, err := hex.DecodeString(m.BinarySHA256); err != nil {
			return errors.New("manifest binary SHA-256 is invalid")
		}
	}
	if m.BinaryPath == "" || !filepath.IsAbs(m.BinaryPath) {
		return errors.New("manifest binary path must be absolute")
	}
	if (m.ServiceManager == "") != (m.ServiceName == "") {
		return errors.New("manifest service manager and service name must be set together")
	}
	if m.ServiceManager != "" {
		switch ServiceManager(m.ServiceManager) {
		case ServiceManagerSystemd, ServiceManagerOpenRC, ServiceManagerWindows:
		default:
			return fmt.Errorf("manifest service manager %q is unsupported", m.ServiceManager)
		}
		if !identifierPattern.MatchString(m.ServiceName) {
			return errors.New("manifest service name is invalid")
		}
	}
	if m.InstalledAt.IsZero() || m.UpdatedAt.IsZero() {
		return errors.New("manifest timestamps are required")
	}
	if m.UpdatedAt.Before(m.InstalledAt) {
		return errors.New("manifest updated_at predates installed_at")
	}
	for _, path := range m.ManagedFiles {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("managed file %q must be absolute", path)
		}
	}
	return nil
}
