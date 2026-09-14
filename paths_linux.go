//go:build linux

package svcforge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type resolvedPaths struct {
	sourcePath   string
	installPath  string
	stateDir     string
	manifestPath string
	backupRoot   string
	lockPath     string
}

func resolvePathsLinux(app App, options Options) (resolvedPaths, error) {
	resolved := resolvedPaths{}
	resolved.installPath = app.Executable.InstallPath
	if resolved.installPath == "" {
		resolved.installPath = filepath.Join("/usr/local/bin", app.Name)
	}
	resolved.stateDir = options.StateDir
	if resolved.stateDir == "" {
		resolved.stateDir = filepath.Join("/var/lib/svcforge", app.ID)
	}
	resolved.sourcePath = options.SourcePath
	if resolved.sourcePath == "" {
		path, err := os.Executable()
		if err != nil {
			return resolvedPaths{}, err
		}
		resolved.sourcePath = path
	}
	var err error
	resolved.sourcePath, err = filepath.Abs(resolved.sourcePath)
	if err != nil {
		return resolvedPaths{}, err
	}
	resolved.installPath, err = filepath.Abs(resolved.installPath)
	if err != nil {
		return resolvedPaths{}, err
	}
	resolved.stateDir, err = filepath.Abs(resolved.stateDir)
	if err != nil {
		return resolvedPaths{}, err
	}
	if resolved.stateDir == string(filepath.Separator) {
		return resolvedPaths{}, errors.New("state directory cannot be the filesystem root")
	}
	for _, path := range []string{resolved.sourcePath, resolved.installPath, resolved.stateDir} {
		if strings.ContainsAny(path, "\r\n\x00") {
			return resolvedPaths{}, errors.New("resolved lifecycle path contains an invalid control character")
		}
	}
	resolved.manifestPath = filepath.Join(resolved.stateDir, "manifest.json")
	resolved.backupRoot = filepath.Join(resolved.stateDir, "backups")
	resolved.lockPath = filepath.Join(resolved.stateDir, "lifecycle.lock")
	return resolved, nil
}
