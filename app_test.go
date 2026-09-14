package svcforge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func validApp(t *testing.T) App {
	t.Helper()
	return App{
		ID:          "com.example.demo",
		Name:        "demo",
		DisplayName: "Demo Service",
		Version:     "1.2.3",
		Executable: Executable{
			InstallPath: filepath.Join(t.TempDir(), "demo"),
			Mode:        0o755,
		},
		Service: Service{
			Enabled:     true,
			Name:        "demo",
			AutoStart:   true,
			Environment: map[string]string{"DEMO_MODE": "test"},
		},
		Upgrade: UpgradePolicy{
			KeepBackups: 3,
			HealthCheck: HealthCheck{
				Check:   func(context.Context) error { return nil },
				Timeout: time.Second,
			},
		},
	}
}

func TestAppValidate(t *testing.T) {
	app := validApp(t)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
	app.Service.Environment["BAD=KEY"] = "x"
	if err := app.Validate(); err == nil {
		t.Fatal("expected invalid environment key")
	}
}

func TestDecideInstall(t *testing.T) {
	app := validApp(t)
	fresh, err := DecideInstall(app, nil)
	if err != nil || fresh.Operation != OperationInstall {
		t.Fatalf("fresh install: decision=%+v err=%v", fresh, err)
	}

	installed := Manifest{AppID: app.ID, Version: "1.2.2"}
	upgrade, err := DecideInstall(app, &installed)
	if err != nil || upgrade.Operation != OperationUpgrade || upgrade.PreviousVersion != "1.2.2" {
		t.Fatalf("upgrade: decision=%+v err=%v", upgrade, err)
	}

	installed.Version = app.Version
	if _, err := DecideInstall(app, &installed); !errors.Is(err, ErrSameVersion) {
		t.Fatalf("same version error = %v", err)
	}

	installed.Version = "2.0.0"
	if _, err := DecideInstall(app, &installed); !errors.Is(err, ErrDowngradeBlocked) {
		t.Fatalf("downgrade error = %v", err)
	}

	app.Upgrade.AllowDowngrade = true
	if _, err := DecideInstall(app, &installed); err != nil {
		t.Fatalf("allowed downgrade: %v", err)
	}
}
