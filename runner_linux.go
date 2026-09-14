//go:build linux

package svcforge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (r Runner) executePlatform(ctx context.Context, operation Operation) (Result, error) {
	app, paths, err := r.normalizedLinuxApp()
	if err != nil {
		return Result{}, err
	}
	if operation == OperationStatus {
		return r.statusLinux(app, paths)
	}
	if !IsAdministrator() {
		return Result{}, ErrAdministratorRequired
	}
	lock, err := acquireInstallLock(paths.lockPath)
	if err != nil {
		return Result{}, err
	}
	defer lock.Close()

	switch operation {
	case OperationInstall, OperationRepair:
		return r.installOrRepairLinux(ctx, app, paths, operation)
	case OperationRemove:
		return r.removeLinux(app, paths)
	default:
		return Result{}, fmt.Errorf("unsupported Linux lifecycle operation %q", operation)
	}
}

func (r Runner) normalizedLinuxApp() (App, resolvedPaths, error) {
	paths, err := resolvePathsLinux(r.App, r.Options)
	if err != nil {
		return App{}, resolvedPaths{}, err
	}
	app := r.App
	app.Executable.InstallPath = paths.installPath
	if err := app.Validate(); err != nil {
		return App{}, resolvedPaths{}, err
	}
	if pathContains(paths.stateDir, paths.installPath) || pathContains(paths.installPath, paths.stateDir) {
		return App{}, resolvedPaths{}, errors.New("executable install path and SvcForge state directory must not overlap")
	}
	return app, paths, nil
}

func validateSourceBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat source binary %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source binary %s is not a regular file", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("source binary %s is empty", path)
	}
	return nil
}

func (r Runner) statusLinux(app App, paths resolvedPaths) (Result, error) {
	manifest, err := ReadManifest(paths.manifestPath)
	if errors.Is(err, ErrNotInstalled) {
		return Result{Operation: OperationStatus, Installed: false, Message: "Application is not installed."}, nil
	}
	if err != nil {
		return Result{}, err
	}
	if manifest.AppID != app.ID {
		return Result{}, fmt.Errorf("installed manifest belongs to %q, not %q", manifest.AppID, app.ID)
	}
	result := Result{
		Operation:      OperationStatus,
		Installed:      true,
		CurrentVersion: manifest.Version,
		ServiceManager: manifest.ServiceManager,
		BinaryMatches:  false,
		ServiceRunning: false,
		ServiceEnabled: false,
		Message:        "Application is installed.",
	}
	if manifest.BinarySHA256 != "" {
		if hash, hashErr := fileSHA256(manifest.BinaryPath); hashErr == nil {
			result.BinaryMatches = hash == manifest.BinarySHA256
		}
	}
	if manifest.ServiceManager != "" {
		installedApp := appForManifest(app, manifest)
		state, inspectErr := inspectServiceLinux(installedApp, ServiceManager(manifest.ServiceManager), defaultLinuxServiceLayout())
		if inspectErr != nil {
			return result, inspectErr
		}
		result.ServiceRunning = state.Running
		result.ServiceEnabled = state.Enabled
	}
	return result, nil
}

func (r Runner) installOrRepairLinux(ctx context.Context, app App, paths resolvedPaths, requested Operation) (Result, error) {
	if err := validateSourceBinary(paths.sourcePath); err != nil {
		return Result{}, err
	}
	manifest, installed, err := readOptionalManifest(paths.manifestPath)
	if err != nil {
		return Result{}, err
	}
	actualOperation := requested
	if requested == OperationInstall {
		var existing *Manifest
		if installed {
			existing = &manifest
		}
		decision, decideErr := DecideInstall(app, existing)
		if decideErr != nil {
			return Result{}, decideErr
		}
		actualOperation = decision.Operation
	} else {
		if !installed {
			return Result{}, ErrNotInstalled
		}
		if !repairVersionMatches(app.Version, manifest.Version) {
			return Result{}, fmt.Errorf("%w: installed=%q requested=%q", ErrRepairVersionMismatch, manifest.Version, app.Version)
		}
	}

	if installed {
		if manifest.AppID != app.ID || manifest.Name != app.Name {
			return Result{}, fmt.Errorf("installed manifest identity does not match app specification")
		}
		if filepath.Clean(manifest.BinaryPath) != paths.installPath {
			return Result{}, fmt.Errorf("installed binary path %s differs from requested path %s; remove the old installation before moving it", manifest.BinaryPath, paths.installPath)
		}
	}

	sourceHash, err := fileSHA256(paths.sourcePath)
	if err != nil {
		return Result{}, err
	}
	oldManager, newManager, oldApp, oldState, oldDefinition, newDefinition, err := resolveServiceTransitionLinux(app, manifest, installed)
	if err != nil {
		return Result{}, err
	}
	if !installed {
		if err := ensureFreshPathsUnmanaged(paths, paths.sourcePath, newDefinition); err != nil {
			return Result{}, err
		}
	}
	if err := validateBackupTargetsLinux(app, paths, oldDefinition, newDefinition); err != nil {
		return Result{}, err
	}

	result := Result{
		Operation:       actualOperation,
		PreviousVersion: manifest.Version,
		CurrentVersion:  app.Version,
		ServiceManager:  string(newManager),
	}

	if installed && oldManager != "" && oldState.Running {
		if err := stopServiceLinux(oldApp, oldManager); err != nil {
			return result, fmt.Errorf("stop existing service before lifecycle transaction: %w", err)
		}
	}

	sources := transactionSnapshotSources(app, paths, oldDefinition, newDefinition)
	snapshot, err := createBackupSnapshot(paths.backupRoot, time.Now().UTC(), sources)
	if err != nil {
		if installed && oldManager != "" && oldState.Running {
			if restartErr := startServiceLinux(oldApp, oldManager); restartErr != nil {
				return result, errors.Join(err, fmt.Errorf("restart service after backup failure: %w", restartErr))
			}
		}
		return result, err
	}
	if snapshot != nil {
		result.BackupPath = snapshot.Path
	}

	mutated := false
	if app.Hooks.BeforeMutation != nil {
		mutated = true
		if hookErr := app.Hooks.BeforeMutation(ctx, actualOperation); hookErr != nil {
			rollbackErr := rollbackLinux(app, newManager, oldApp, oldManager, oldState, snapshot)
			if rollbackErr != nil {
				return result, errors.Join(fmt.Errorf("before-mutation hook: %w", hookErr), fmt.Errorf("rollback failed: %w", rollbackErr))
			}
			return result, fmt.Errorf("before-mutation hook: %w", hookErr)
		}
	}
	fail := func(cause error) (Result, error) {
		if !mutated {
			return result, cause
		}
		rollbackErr := rollbackLinux(app, newManager, oldApp, oldManager, oldState, snapshot)
		if rollbackErr != nil {
			return result, errors.Join(cause, fmt.Errorf("rollback failed: %w", rollbackErr))
		}
		return result, cause
	}

	if err := copyFileAtomic(paths.sourcePath, paths.installPath, app.executableMode()); err != nil {
		return fail(err)
	}
	mutated = true
	installedHash, err := fileSHA256(paths.installPath)
	if err != nil {
		return fail(err)
	}
	if installedHash != sourceHash {
		return fail(errors.New("installed binary checksum does not match source binary"))
	}

	if oldManager != "" && newManager == "" {
		if oldState.Enabled {
			if err := disableServiceLinux(oldApp, oldManager); err != nil {
				return fail(fmt.Errorf("disable removed service: %w", err))
			}
		}
		if err := removeServiceDefinitionLinux(oldApp, oldManager, defaultLinuxServiceLayout()); err != nil {
			return fail(err)
		}
	}

	managedFiles := []string{paths.installPath}
	if newManager != "" {
		definitionPath, err := installServiceDefinitionLinux(app, newManager, defaultLinuxServiceLayout())
		if err != nil {
			return fail(err)
		}
		managedFiles = append(managedFiles, definitionPath)
		if app.Service.AutoStart {
			if err := enableServiceLinux(app, newManager); err != nil {
				return fail(fmt.Errorf("enable service: %w", err))
			}
		} else if oldState.Enabled {
			if err := disableServiceLinux(app, newManager); err != nil {
				return fail(fmt.Errorf("disable service autostart: %w", err))
			}
		}
		if err := startServiceLinux(app, newManager); err != nil {
			return fail(fmt.Errorf("start service: %w", err))
		}
	}

	if err := runHealthCheck(ctx, app.Upgrade.HealthCheck); err != nil {
		return fail(err)
	}

	now := time.Now().UTC()
	installedAt := now
	if installed {
		installedAt = manifest.InstalledAt
	}
	newManifest := Manifest{
		Schema:       ManifestSchema,
		AppID:        app.ID,
		Name:         app.Name,
		Version:      app.Version,
		BinaryPath:   paths.installPath,
		BinarySHA256: installedHash,
		InstalledAt:  installedAt,
		UpdatedAt:    now,
		ManagedFiles: uniqueCleanPaths(managedFiles),
	}
	if newManager != "" {
		newManifest.ServiceManager = string(newManager)
		newManifest.ServiceName = app.serviceName()
	}
	if err := WriteManifest(paths.manifestPath, newManifest); err != nil {
		return fail(err)
	}

	result.Changed = true
	result.Installed = true
	result.BinaryMatches = true
	if newManager != "" {
		state, inspectErr := inspectServiceLinux(app, newManager, defaultLinuxServiceLayout())
		if inspectErr == nil {
			result.ServiceRunning = state.Running
			result.ServiceEnabled = state.Enabled
		}
	}
	switch actualOperation {
	case OperationInstall:
		result.Message = "Application installed successfully."
	case OperationUpgrade:
		result.Message = "Application upgraded successfully."
	case OperationRepair:
		result.Message = "Application repaired successfully."
	}

	if !installed && snapshot != nil {
		_ = os.RemoveAll(snapshot.Path)
		result.BackupPath = ""
	} else if snapshot != nil {
		if pruneErr := pruneBackups(paths.backupRoot, app.Upgrade.KeepBackups); pruneErr != nil {
			return result, fmt.Errorf("installation succeeded but backup retention cleanup failed: %w", pruneErr)
		}
	}
	return result, nil
}

func (r Runner) removeLinux(app App, paths resolvedPaths) (Result, error) {
	manifest, err := ReadManifest(paths.manifestPath)
	if err != nil {
		return Result{}, err
	}
	if manifest.AppID != app.ID || manifest.Name != app.Name {
		return Result{}, errors.New("installed manifest identity does not match app specification")
	}
	installedApp := appForManifest(app, manifest)
	manager := ServiceManager(manifest.ServiceManager)
	state := ServiceState{}
	definitionPath := ""
	if manager != "" {
		state, err = inspectServiceLinux(installedApp, manager, defaultLinuxServiceLayout())
		if err != nil {
			return Result{}, err
		}
		definitionPath, err = serviceDefinitionPathLinux(installedApp, manager, defaultLinuxServiceLayout())
		if err != nil {
			return Result{}, err
		}
		if state.Running {
			if err := stopServiceLinux(installedApp, manager); err != nil {
				return Result{}, err
			}
		}
	}

	sources := []snapshotSource{{Path: manifest.BinaryPath, Optional: true}}
	if definitionPath != "" {
		sources = append(sources, snapshotSource{Path: definitionPath, Optional: true})
	}
	snapshot, err := createBackupSnapshot(paths.backupRoot, time.Now().UTC(), dedupeSnapshotSources(sources))
	if err != nil {
		if manager != "" && state.Running {
			_ = startServiceLinux(installedApp, manager)
		}
		return Result{}, err
	}
	result := Result{
		Operation:       OperationRemove,
		Installed:       true,
		PreviousVersion: manifest.Version,
		ServiceManager:  manifest.ServiceManager,
		BackupPath:      snapshot.Path,
	}
	mutated := false
	fail := func(cause error) (Result, error) {
		if !mutated {
			return result, cause
		}
		rollbackErr := rollbackLinux(App{}, "", installedApp, manager, state, snapshot)
		if rollbackErr != nil {
			return result, errors.Join(cause, fmt.Errorf("rollback failed: %w", rollbackErr))
		}
		return result, cause
	}
	if manager != "" {
		if state.Enabled {
			if err := disableServiceLinux(installedApp, manager); err != nil {
				return fail(err)
			}
		}
		mutated = true
		if err := removeServiceDefinitionLinux(installedApp, manager, defaultLinuxServiceLayout()); err != nil {
			return fail(err)
		}
	}
	if err := os.Remove(manifest.BinaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("remove installed binary: %w", err))
	}
	mutated = true
	if err := os.Remove(paths.manifestPath); err != nil {
		return fail(fmt.Errorf("remove install manifest: %w", err))
	}
	if err := pruneBackups(paths.backupRoot, app.Upgrade.KeepBackups); err != nil {
		return result, fmt.Errorf("application removed but backup retention cleanup failed: %w", err)
	}
	result.Changed = true
	result.Installed = false
	result.Message = "Application removed successfully. Persistent application data was preserved."
	return result, nil
}

func readOptionalManifest(path string) (Manifest, bool, error) {
	manifest, err := ReadManifest(path)
	if errors.Is(err, ErrNotInstalled) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	return manifest, true, nil
}

func repairVersionMatches(requested, installed string) bool {
	if requested == "" || installed == "" {
		return requested == installed
	}
	comparison, err := CompareVersions(requested, installed)
	return err == nil && comparison == 0
}

func resolveServiceTransitionLinux(app App, manifest Manifest, installed bool) (ServiceManager, ServiceManager, App, ServiceState, string, string, error) {
	var oldManager ServiceManager
	oldApp := app
	oldState := ServiceState{}
	oldDefinition := ""
	if installed && manifest.ServiceManager != "" {
		oldManager = ServiceManager(manifest.ServiceManager)
		oldApp = appForManifest(app, manifest)
		var err error
		oldDefinition, err = serviceDefinitionPathLinux(oldApp, oldManager, defaultLinuxServiceLayout())
		if err != nil {
			return "", "", App{}, ServiceState{}, "", "", err
		}
		oldState, err = inspectServiceLinux(oldApp, oldManager, defaultLinuxServiceLayout())
		if err != nil {
			return "", "", App{}, ServiceState{}, "", "", err
		}
	}

	var newManager ServiceManager
	newDefinition := ""
	if app.Service.Enabled {
		manager, err := DetectServiceManager()
		if err != nil {
			return "", "", App{}, ServiceState{}, "", "", err
		}
		newManager = manager
		if oldManager != "" && oldManager != newManager {
			return "", "", App{}, ServiceState{}, "", "", fmt.Errorf("service manager migration from %s to %s is not automatic; remove and reinstall the service", oldManager, newManager)
		}
		if installed && manifest.ServiceName != "" && manifest.ServiceName != app.serviceName() {
			return "", "", App{}, ServiceState{}, "", "", fmt.Errorf("service rename from %s to %s is not automatic; remove and reinstall the service", manifest.ServiceName, app.serviceName())
		}
		newDefinition, err = serviceDefinitionPathLinux(app, newManager, defaultLinuxServiceLayout())
		if err != nil {
			return "", "", App{}, ServiceState{}, "", "", err
		}
	}
	return oldManager, newManager, oldApp, oldState, oldDefinition, newDefinition, nil
}

func appForManifest(app App, manifest Manifest) App {
	installedApp := app
	installedApp.Executable.InstallPath = manifest.BinaryPath
	if manifest.ServiceManager != "" {
		installedApp.Service.Enabled = true
		installedApp.Service.Name = manifest.ServiceName
	}
	return installedApp
}

func ensureFreshPathsUnmanaged(paths resolvedPaths, sourcePath, serviceDefinition string) error {
	if filepath.Clean(sourcePath) != paths.installPath {
		if _, err := os.Lstat(paths.installPath); err == nil {
			return fmt.Errorf("%w: %s", ErrPathConflict, paths.installPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if serviceDefinition != "" {
		if _, err := os.Lstat(serviceDefinition); err == nil {
			return fmt.Errorf("%w: %s", ErrPathConflict, serviceDefinition)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func validateBackupTargetsLinux(app App, paths resolvedPaths, serviceDefinitions ...string) error {
	seen := map[string]struct{}{}
	for _, target := range app.Upgrade.BackupTargets {
		clean := filepath.Clean(target.Path)
		if clean == string(filepath.Separator) {
			return errors.New("backup target cannot be the filesystem root")
		}
		if _, exists := seen[clean]; exists {
			return fmt.Errorf("backup target %s is declared more than once", clean)
		}
		seen[clean] = struct{}{}
		if pathContains(clean, paths.stateDir) || pathContains(paths.stateDir, clean) {
			return fmt.Errorf("backup target %s overlaps SvcForge state directory %s", clean, paths.stateDir)
		}
		if pathContains(clean, paths.installPath) || clean == paths.installPath {
			return fmt.Errorf("backup target %s contains the managed executable %s", clean, paths.installPath)
		}
		for _, definition := range serviceDefinitions {
			if definition != "" && (clean == definition || pathContains(clean, definition)) {
				return fmt.Errorf("backup target %s contains the managed service definition %s", clean, definition)
			}
		}
	}
	return nil
}

func transactionSnapshotSources(app App, paths resolvedPaths, serviceDefinitions ...string) []snapshotSource {
	sources := []snapshotSource{{Path: paths.installPath, Optional: true}}
	for _, definition := range serviceDefinitions {
		if definition != "" {
			sources = append(sources, snapshotSource{Path: definition, Optional: true})
		}
	}
	for _, target := range app.Upgrade.BackupTargets {
		sources = append(sources, snapshotSource{Path: target.Path, Optional: target.Optional})
	}
	return dedupeSnapshotSources(sources)
}

func dedupeSnapshotSources(sources []snapshotSource) []snapshotSource {
	seen := map[string]int{}
	result := make([]snapshotSource, 0, len(sources))
	for _, source := range sources {
		clean := filepath.Clean(source.Path)
		if index, ok := seen[clean]; ok {
			if !source.Optional {
				result[index].Optional = false
			}
			continue
		}
		seen[clean] = len(result)
		source.Path = clean
		result = append(result, source)
	}
	return result
}

func uniqueCleanPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	sort.Strings(result)
	return result
}

func rollbackLinux(newApp App, newManager ServiceManager, oldApp App, oldManager ServiceManager, oldState ServiceState, snapshot *backupSnapshot) error {
	var rollbackErrors []error
	if newManager != "" {
		newState, inspectErr := inspectServiceLinux(newApp, newManager, defaultLinuxServiceLayout())
		if inspectErr != nil {
			rollbackErrors = append(rollbackErrors, inspectErr)
		} else if newState.Installed {
			if newState.Running {
				if err := stopServiceLinux(newApp, newManager); err != nil {
					rollbackErrors = append(rollbackErrors, err)
				}
			}
			if newState.Enabled {
				if err := disableServiceLinux(newApp, newManager); err != nil {
					rollbackErrors = append(rollbackErrors, err)
				}
			}
		}
	}
	if err := snapshot.Restore(); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	for _, manager := range uniqueManagers(oldManager, newManager) {
		if err := reloadServiceManagerLinux(manager); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if oldManager != "" && oldState.Installed {
		if oldState.Enabled {
			if err := enableServiceLinux(oldApp, oldManager); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		} else {
			if err := disableServiceLinux(oldApp, oldManager); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
		if oldState.Running {
			if err := startServiceLinux(oldApp, oldManager); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
	}
	return errors.Join(rollbackErrors...)
}

func uniqueManagers(managers ...ServiceManager) []ServiceManager {
	seen := map[ServiceManager]struct{}{}
	var result []ServiceManager
	for _, manager := range managers {
		if manager == "" {
			continue
		}
		if _, ok := seen[manager]; ok {
			continue
		}
		seen[manager] = struct{}{}
		result = append(result, manager)
	}
	return result
}
