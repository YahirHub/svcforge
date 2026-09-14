//go:build linux

package svcforge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func transactionTestRunner(t *testing.T, version, content string) (Runner, string, string, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source-"+strings.ReplaceAll(version, ".", "-")+".bin")
	if err := os.WriteFile(source, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	installPath := filepath.Join(root, "installed", "demo")
	stateDir := filepath.Join(root, "state")
	dataPath := filepath.Join(root, "data", "database.txt")
	app := App{
		ID:          "com.example.transaction-demo",
		Name:        "transaction-demo",
		DisplayName: "Transaction Demo",
		Version:     version,
		Executable: Executable{
			InstallPath: installPath,
			Mode:        0o755,
		},
		Upgrade: UpgradePolicy{KeepBackups: 2},
	}
	return Runner{App: app, Options: Options{SourcePath: source, StateDir: stateDir}}, installPath, stateDir, dataPath
}

func executeTransactionForTest(t *testing.T, runner Runner, operation Operation) (Result, error) {
	t.Helper()
	app, paths, err := runner.normalizedLinuxApp()
	if err != nil {
		return Result{}, err
	}
	switch operation {
	case OperationInstall, OperationRepair:
		return runner.installOrRepairLinux(context.Background(), app, paths, operation)
	case OperationRemove:
		return runner.removeLinux(app, paths)
	case OperationStatus:
		return runner.statusLinux(app, paths)
	default:
		t.Fatalf("unsupported test operation %s", operation)
		return Result{}, nil
	}
}

func TestTransactionFreshInstallSameVersionRepairUpgradeRemove(t *testing.T) {
	runner, installPath, stateDir, dataPath := transactionTestRunner(t, "1.0.0", "binary-v1")
	installResult, err := executeTransactionForTest(t, runner, OperationInstall)
	if err != nil {
		t.Fatal(err)
	}
	if installResult.Operation != OperationInstall || !installResult.Changed || !installResult.BinaryMatches {
		t.Fatalf("install result = %+v", installResult)
	}
	assertFileContent(t, installPath, "binary-v1")

	status, err := executeTransactionForTest(t, runner, OperationStatus)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || !status.BinaryMatches || status.CurrentVersion != "1.0.0" {
		t.Fatalf("status = %+v", status)
	}

	if _, err := executeTransactionForTest(t, runner, OperationInstall); !errors.Is(err, ErrSameVersion) {
		t.Fatalf("same-version install error = %v", err)
	}

	repairSource := filepath.Join(filepath.Dir(runner.Options.SourcePath), "repair.bin")
	if err := os.WriteFile(repairSource, []byte("binary-v1-repaired"), 0o700); err != nil {
		t.Fatal(err)
	}
	repairRunner := runner
	repairRunner.Options.SourcePath = repairSource
	repairResult, err := executeTransactionForTest(t, repairRunner, OperationRepair)
	if err != nil {
		t.Fatal(err)
	}
	if repairResult.Operation != OperationRepair || repairResult.BackupPath == "" {
		t.Fatalf("repair result = %+v", repairResult)
	}
	assertFileContent(t, installPath, "binary-v1-repaired")

	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("db-v1"), 0o640); err != nil {
		t.Fatal(err)
	}
	upgradeSource := filepath.Join(filepath.Dir(runner.Options.SourcePath), "upgrade.bin")
	if err := os.WriteFile(upgradeSource, []byte("binary-v2"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgradeRunner := runner
	upgradeRunner.App.Version = "2.0.0"
	upgradeRunner.App.Upgrade.BackupTargets = []BackupTarget{{Path: dataPath}}
	upgradeRunner.Options.SourcePath = upgradeSource
	upgradeResult, err := executeTransactionForTest(t, upgradeRunner, OperationInstall)
	if err != nil {
		t.Fatal(err)
	}
	if upgradeResult.Operation != OperationUpgrade || upgradeResult.PreviousVersion != "1.0.0" || upgradeResult.CurrentVersion != "2.0.0" {
		t.Fatalf("upgrade result = %+v", upgradeResult)
	}
	assertFileContent(t, installPath, "binary-v2")
	assertFileContent(t, dataPath, "db-v1")

	status, err = executeTransactionForTest(t, upgradeRunner, OperationStatus)
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentVersion != "2.0.0" || !status.BinaryMatches {
		t.Fatalf("upgraded status = %+v", status)
	}

	removeResult, err := executeTransactionForTest(t, upgradeRunner, OperationRemove)
	if err != nil {
		t.Fatal(err)
	}
	if !removeResult.Changed || removeResult.Installed || removeResult.BackupPath == "" {
		t.Fatalf("remove result = %+v", removeResult)
	}
	if _, err := os.Stat(installPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installed binary should be removed, stat err=%v", err)
	}
	assertFileContent(t, dataPath, "db-v1")
	if _, err := os.Stat(filepath.Join(stateDir, "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest should be removed, stat err=%v", err)
	}
}

func TestTransactionRollbackRestoresBinaryAndDataAfterHealthFailure(t *testing.T) {
	runner, installPath, stateDir, dataPath := transactionTestRunner(t, "1.0.0", "binary-v1")
	if _, err := executeTransactionForTest(t, runner, OperationInstall); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("database-before-upgrade"), 0o640); err != nil {
		t.Fatal(err)
	}

	upgradeSource := filepath.Join(filepath.Dir(runner.Options.SourcePath), "broken-upgrade.bin")
	if err := os.WriteFile(upgradeSource, []byte("binary-v2-broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgrade := runner
	upgrade.App.Version = "2.0.0"
	upgrade.Options.SourcePath = upgradeSource
	upgrade.App.Upgrade.BackupTargets = []BackupTarget{{Path: dataPath}}
	upgrade.App.Upgrade.HealthCheck = HealthCheck{
		Timeout: 30 * time.Millisecond,
		Check: func(context.Context) error {
			_ = os.WriteFile(dataPath, []byte("corrupted-by-new-version"), 0o600)
			return errors.New("simulated health failure")
		},
	}
	result, err := executeTransactionForTest(t, upgrade, OperationInstall)
	if err == nil || !strings.Contains(err.Error(), "health check") {
		t.Fatalf("expected health failure, result=%+v err=%v", result, err)
	}
	if result.BackupPath == "" {
		t.Fatalf("rollback result did not expose backup path: %+v", result)
	}
	assertFileContent(t, installPath, "binary-v1")
	assertFileContent(t, dataPath, "database-before-upgrade")
	manifest, err := ReadManifest(filepath.Join(stateDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.0.0" {
		t.Fatalf("manifest version after rollback = %s", manifest.Version)
	}
}

func TestTransactionRepairRejectsDifferentVersionAndDowngrade(t *testing.T) {
	runner, _, _, _ := transactionTestRunner(t, "2.0.0", "binary-v2")
	if _, err := executeTransactionForTest(t, runner, OperationInstall); err != nil {
		t.Fatal(err)
	}

	other := runner
	other.App.Version = "2.1.0"
	if _, err := executeTransactionForTest(t, other, OperationRepair); !errors.Is(err, ErrRepairVersionMismatch) {
		t.Fatalf("repair mismatch error = %v", err)
	}

	downgradeSource := filepath.Join(filepath.Dir(runner.Options.SourcePath), "downgrade.bin")
	if err := os.WriteFile(downgradeSource, []byte("binary-v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	downgrade := runner
	downgrade.App.Version = "1.0.0"
	downgrade.Options.SourcePath = downgradeSource
	if _, err := executeTransactionForTest(t, downgrade, OperationInstall); !errors.Is(err, ErrDowngradeBlocked) {
		t.Fatalf("downgrade error = %v", err)
	}
}

func TestStatusDetectsTamperedBinary(t *testing.T) {
	runner, installPath, _, _ := transactionTestRunner(t, "1.0.0", "binary-v1")
	if _, err := executeTransactionForTest(t, runner, OperationInstall); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installPath, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	status, err := executeTransactionForTest(t, runner, OperationStatus)
	if err != nil {
		t.Fatal(err)
	}
	if status.BinaryMatches {
		t.Fatalf("tampered binary reported as matching: %+v", status)
	}
}

func TestFreshInstallRefusesUnmanagedBinary(t *testing.T) {
	runner, installPath, _, _ := transactionTestRunner(t, "1.0.0", "candidate")
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installPath, []byte("unmanaged"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := executeTransactionForTest(t, runner, OperationInstall); !errors.Is(err, ErrPathConflict) {
		t.Fatalf("path conflict error = %v", err)
	}
	assertFileContent(t, installPath, "unmanaged")
}

func TestUpgradeRequiredBackupMissingLeavesInstallationUntouched(t *testing.T) {
	runner, installPath, _, dataPath := transactionTestRunner(t, "1.0.0", "binary-v1")
	if _, err := executeTransactionForTest(t, runner, OperationInstall); err != nil {
		t.Fatal(err)
	}
	upgradeSource := filepath.Join(filepath.Dir(runner.Options.SourcePath), "v2.bin")
	if err := os.WriteFile(upgradeSource, []byte("binary-v2"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgrade := runner
	upgrade.App.Version = "2.0.0"
	upgrade.Options.SourcePath = upgradeSource
	upgrade.App.Upgrade.BackupTargets = []BackupTarget{{Path: dataPath}}
	if _, err := executeTransactionForTest(t, upgrade, OperationInstall); err == nil || !strings.Contains(err.Error(), "required backup target") {
		t.Fatalf("missing backup error = %v", err)
	}
	assertFileContent(t, installPath, "binary-v1")
}

func TestBackupTargetCannotContainSvcForgeState(t *testing.T) {
	runner, _, stateDir, _ := transactionTestRunner(t, "1.0.0", "binary-v1")
	runner.App.Upgrade.BackupTargets = []BackupTarget{{Path: filepath.Dir(stateDir), Optional: true}}
	app, paths, err := runner.normalizedLinuxApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBackupTargetsLinux(app, paths); err == nil {
		t.Fatal("expected state directory overlap to be rejected")
	}
}

func TestInstallLockRejectsConcurrentLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "lifecycle.lock")
	first, err := acquireInstallLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := acquireInstallLock(path)
	if !errors.Is(err, ErrOperationInProgress) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("second lock error = %v", err)
	}
}

func TestBackupRoundTripPreservesModeAndTimestamp(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "data", "config.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(source, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	snapshot, err := createBackupSnapshot(filepath.Join(root, "backups"), time.Now(), []snapshotSource{{Path: source}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, source, "original")
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("restored mode = %o", info.Mode().Perm())
	}
	if !info.ModTime().Equal(stamp) {
		t.Fatalf("restored mtime = %s, want %s", info.ModTime(), stamp)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func TestBeforeMutationHookIsSkippedForSameVersionAndRollbackCovered(t *testing.T) {
	runner, _, _, dataPath := transactionTestRunner(t, "1.0.0", "binary-v1")
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	runner.App.Upgrade.BackupTargets = []BackupTarget{{Path: dataPath}}
	calls := 0
	runner.App.Hooks.BeforeMutation = func(context.Context, Operation) error {
		calls++
		return os.WriteFile(dataPath, []byte("installed"), 0o640)
	}
	if _, err := executeTransactionForTest(t, runner, OperationInstall); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("hook calls after install = %d", calls)
	}
	if _, err := executeTransactionForTest(t, runner, OperationInstall); !errors.Is(err, ErrSameVersion) {
		t.Fatalf("same version error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("same-version install unexpectedly ran hook; calls=%d", calls)
	}

	repairSource := filepath.Join(filepath.Dir(runner.Options.SourcePath), "repair-hook.bin")
	if err := os.WriteFile(repairSource, []byte("binary-v1-repair"), 0o700); err != nil {
		t.Fatal(err)
	}
	repair := runner
	repair.Options.SourcePath = repairSource
	repair.App.Hooks.BeforeMutation = func(context.Context, Operation) error {
		if err := os.WriteFile(dataPath, []byte("hook-corruption"), 0o600); err != nil {
			return err
		}
		return errors.New("hook failed")
	}
	if _, err := executeTransactionForTest(t, repair, OperationRepair); err == nil || !strings.Contains(err.Error(), "before-mutation hook") {
		t.Fatalf("hook failure error = %v", err)
	}
	assertFileContent(t, dataPath, "installed")
}

func TestFreshInstallHookFailureRestoresOptionalPersistentTarget(t *testing.T) {
	runner, _, _, dataPath := transactionTestRunner(t, "1.0.0", "binary-v1")
	runner.App.Upgrade.BackupTargets = []BackupTarget{{Path: dataPath, Optional: true}}
	runner.App.Hooks.BeforeMutation = func(context.Context, Operation) error {
		if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dataPath, []byte("created-by-hook"), 0o600); err != nil {
			return err
		}
		return errors.New("stop fresh install")
	}
	if _, err := executeTransactionForTest(t, runner, OperationInstall); err == nil {
		t.Fatal("expected hook failure")
	}
	if _, err := os.Stat(dataPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh hook-created target should be rolled back, stat err=%v", err)
	}
}
