package svcforge

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const ManifestSchema = 1

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// App describes one installable application.
// Version is optional; leave it empty to disable version ordering.
type App struct {
	ID          string
	Name        string
	DisplayName string
	Description string
	Version     string
	Executable  Executable
	Service     Service
	Upgrade     UpgradePolicy
}

// Executable describes the installed application binary.
type Executable struct {
	InstallPath string
	Mode        fs.FileMode
}

// Service describes an optional long-running system service.
type Service struct {
	Enabled          bool
	Name             string
	Description      string
	Arguments        []string
	Environment      map[string]string
	WorkingDirectory string
	User             string
	Group            string
	AutoStart        bool
	Restart          RestartPolicy
	DependOnNetwork  bool
	LogPath          string
}

// BackupTarget describes persistent state that must be copied before an upgrade.
type BackupTarget struct {
	Path     string
	Optional bool
}

// UpgradePolicy controls version ordering, data protection and post-install validation.
type UpgradePolicy struct {
	AllowDowngrade bool
	BackupTargets  []BackupTarget
	KeepBackups    int
	HealthCheck    HealthCheck
}

// HealthCheck describes optional post-start validation.
// Check takes precedence over URL when both are supplied.
type HealthCheck struct {
	Check          func(context.Context) error
	URL            string
	ExpectedStatus int
	Timeout        time.Duration
	InitialDelay   time.Duration
}

// Operation identifies a lifecycle command.
type Operation string

const (
	OperationInstall Operation = "install"
	OperationUpgrade Operation = "upgrade"
	OperationRepair  Operation = "repair"
	OperationRemove  Operation = "remove"
	OperationStatus  Operation = "status"
)

// Result summarizes one lifecycle operation.
type Result struct {
	Operation       Operation
	Changed         bool
	Installed       bool
	PreviousVersion string
	CurrentVersion  string
	ServiceManager  string
	BackupPath      string
	Message         string
}

var (
	// ErrNotInstalled indicates that no SvcForge manifest exists for the app.
	ErrNotInstalled = errors.New("application is not installed")
	// ErrSameVersion indicates that --install was requested for the installed version.
	ErrSameVersion = errors.New("the same version is already installed; use --repair to repair the installation")
	// ErrDowngradeBlocked indicates that an older version was supplied without opt-in.
	ErrDowngradeBlocked = errors.New("downgrade is blocked by policy")
)

// Validate checks the portable parts of an application specification.
func (a App) Validate() error {
	if !identifierPattern.MatchString(a.ID) {
		return fmt.Errorf("invalid app ID %q: use ASCII letters, digits, '.', '_' or '-'", a.ID)
	}
	if !identifierPattern.MatchString(a.Name) {
		return fmt.Errorf("invalid app name %q", a.Name)
	}
	if a.DisplayName == "" {
		return errors.New("display name is required")
	}
	if strings.ContainsAny(a.DisplayName, "\r\n\x00") {
		return errors.New("display name contains an invalid control character")
	}
	if strings.ContainsAny(a.Description, "\r\n\x00") {
		return errors.New("description contains an invalid control character")
	}
	if a.Version != "" {
		if _, err := ParseVersion(a.Version); err != nil {
			return fmt.Errorf("invalid version: %w", err)
		}
	}
	if strings.ContainsAny(a.Executable.InstallPath, "\r\n\x00") {
		return errors.New("executable install path contains an invalid control character")
	}
	if a.Executable.InstallPath != "" && !filepath.IsAbs(a.Executable.InstallPath) {
		return errors.New("executable install path must be absolute")
	}
	if a.Executable.Mode != 0 && a.Executable.Mode.Perm()&0o111 == 0 {
		return errors.New("executable mode must include at least one execute bit")
	}
	if a.Service.Enabled {
		name := a.Service.Name
		if name == "" {
			name = a.Name
		}
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("invalid service name %q", name)
		}
		if strings.ContainsAny(a.Service.Description, "\r\n\x00") {
			return errors.New("service description contains an invalid control character")
		}
		for _, arg := range a.Service.Arguments {
			if strings.ContainsAny(arg, "\r\n\x00") {
				return errors.New("service argument contains an invalid control character")
			}
		}
		if a.Service.User != "" && !identifierPattern.MatchString(a.Service.User) {
			return fmt.Errorf("invalid service user %q", a.Service.User)
		}
		if a.Service.Group != "" && !identifierPattern.MatchString(a.Service.Group) {
			return fmt.Errorf("invalid service group %q", a.Service.Group)
		}
		if strings.ContainsAny(a.Service.WorkingDirectory, "\r\n\x00") {
			return errors.New("service working directory contains an invalid control character")
		}
		if a.Service.WorkingDirectory != "" && !filepath.IsAbs(a.Service.WorkingDirectory) {
			return errors.New("service working directory must be absolute")
		}
		if a.Service.Restart != "" && a.Service.Restart != RestartNever && a.Service.Restart != RestartOnFailure && a.Service.Restart != RestartAlways {
			return fmt.Errorf("invalid restart policy %q", a.Service.Restart)
		}
		if strings.ContainsAny(a.Service.LogPath, "\r\n\x00") {
			return errors.New("service log path contains an invalid control character")
		}
		if a.Service.LogPath != "" && !filepath.IsAbs(a.Service.LogPath) {
			return errors.New("service log path must be absolute")
		}
		for key, value := range a.Service.Environment {
			if err := validateEnvironment(key, value); err != nil {
				return err
			}
		}
	}
	if a.Upgrade.KeepBackups < 0 {
		return errors.New("keep backups cannot be negative")
	}
	for _, target := range a.Upgrade.BackupTargets {
		if strings.ContainsAny(target.Path, "\r\n\x00") {
			return fmt.Errorf("backup target %q contains an invalid control character", target.Path)
		}
		if target.Path == "" || !filepath.IsAbs(target.Path) {
			return fmt.Errorf("backup target %q must be an absolute path", target.Path)
		}
	}
	if h := a.Upgrade.HealthCheck; h.URL != "" {
		if !strings.HasPrefix(h.URL, "http://") && !strings.HasPrefix(h.URL, "https://") {
			return errors.New("health check URL must use http or https")
		}
		if h.ExpectedStatus < 0 || h.ExpectedStatus > 999 {
			return errors.New("health check expected status is invalid")
		}
	}
	return nil
}

func validateEnvironment(key, value string) error {
	if key == "" || strings.ContainsAny(key, "=\x00\r\n") {
		return fmt.Errorf("invalid environment variable name %q", key)
	}
	for i, r := range key {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return fmt.Errorf("invalid environment variable name %q", key)
		}
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("environment variable %q contains an invalid control character", key)
	}
	return nil
}

func (a App) serviceName() string {
	if a.Service.Name != "" {
		return a.Service.Name
	}
	return a.Name
}

func (a App) executableMode() fs.FileMode {
	if a.Executable.Mode != 0 {
		return a.Executable.Mode.Perm()
	}
	return 0o755
}
