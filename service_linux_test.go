//go:build linux

package svcforge

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func linuxServiceApp(t *testing.T) App {
	t.Helper()
	app := validApp(t)
	app.DisplayName = "Demo Service"
	app.Description = "Demo background service"
	app.Executable.InstallPath = "/opt/demo/bin/demo"
	app.Service = Service{
		Enabled:          true,
		Name:             "demo",
		Description:      "Demo background service",
		Arguments:        []string{"serve", "--label", "hello world", "--literal=$VALUE%"},
		Environment:      map[string]string{"DEMO_MODE": "production", "QUOTE": `a"b`},
		WorkingDirectory: "/var/lib/demo",
		AutoStart:        true,
		Restart:          RestartOnFailure,
		DependOnNetwork:  true,
		LogPath:          "/var/log/demo.log",
	}
	return app
}

func TestRenderSystemdUnit(t *testing.T) {
	app := linuxServiceApp(t)
	app.Service.Restart = RestartOnFailure
	unit, err := renderSystemdUnit(app)
	if err != nil {
		t.Fatal(err)
	}
	text := string(unit)
	for _, marker := range []string{
		"Description=Demo background service",
		"Wants=network-online.target",
		`ExecStart="/opt/demo/bin/demo" "serve" "--label" "hello world" "--literal=$$VALUE%%"`,
		`Environment="DEMO_MODE=production"`,
		`Environment="QUOTE=a\"b"`,
		"WorkingDirectory=/var/lib/demo",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("systemd unit missing %q:\n%s", marker, text)
		}
	}
	if strings.Contains(text, `WorkingDirectory="/var/lib/demo"`) {
		t.Fatalf("WorkingDirectory must use path syntax, not command argument quoting:\n%s", text)
	}
}

func TestRenderSystemdWorkingDirectoryEscapesSpecifiers(t *testing.T) {
	app := linuxServiceApp(t)
	app.Service.WorkingDirectory = "/var/lib/demo%cache"
	unit, err := renderSystemdUnit(app)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "WorkingDirectory=/var/lib/demo%%cache\n") {
		t.Fatalf("WorkingDirectory percent must be escaped as a literal systemd specifier:\n%s", unit)
	}
}

func TestRenderedSystemdUnitPassesSystemdAnalyzeWhenAvailable(t *testing.T) {
	tool, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze is not installed on this test host")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "demo")
	writeExecutable(t, executable, "#!/bin/sh\nexit 0\n")
	app := linuxServiceApp(t)
	app.Executable.InstallPath = executable
	app.Service.WorkingDirectory = root
	unit, err := renderSystemdUnit(app)
	if err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(root, "demo.service")
	if err := os.WriteFile(unitPath, unit, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(tool, "verify", unitPath).CombinedOutput()
	if err != nil {
		t.Fatalf("systemd-analyze verify failed: %v\n%s\nunit:\n%s", err, output, unit)
	}
}

func TestRenderOpenRCScriptQuotesValues(t *testing.T) {
	app := linuxServiceApp(t)
	app.Service.Restart = RestartNever
	app.Service.Arguments = []string{"serve", "a b", "x'$(touch /tmp/nope)'"}
	script, err := renderOpenRCScript(app)
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, marker := range []string{
		"#!/sbin/openrc-run",
		"command='/opt/demo/bin/demo'",
		"command_background=yes",
		"pidfile='/run/demo.pid'",
		"need net",
		"output_log='/var/log/demo.log'",
		"export DEMO_MODE='production'",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("OpenRC script missing %q:\n%s", marker, text)
		}
	}
	tmp := filepath.Join(t.TempDir(), "demo")
	if err := os.WriteFile(tmp, script, 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/sh", "-n", tmp).CombinedOutput(); err != nil {
		t.Fatalf("OpenRC syntax: %v: %s", err, output)
	}
	if strings.Contains(text, "command_args=serve a b") {
		t.Fatal("OpenRC arguments were emitted without quoting")
	}
}

func TestInstallServiceDefinitionLinuxUsesExpectedMode(t *testing.T) {
	app := linuxServiceApp(t)
	app.Service.Restart = RestartNever
	root := t.TempDir()
	layout := linuxServiceLayout{
		systemdUnitDir: filepath.Join(root, "systemd"),
		openRCInitDir:  filepath.Join(root, "init.d"),
	}
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(fakeBin, "systemctl"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", fakeBin)

	systemdPath, err := installServiceDefinitionLinux(app, ServiceManagerSystemd, layout)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(systemdPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("systemd mode = %o", info.Mode().Perm())
	}

	openRCPath, err := installServiceDefinitionLinux(app, ServiceManagerOpenRC, layout)
	if err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(openRCPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("OpenRC mode = %o", info.Mode().Perm())
	}
}

func TestOpenRCDefaultContainsExactService(t *testing.T) {
	output := []byte("       demo-helper | default\n              demo | default\n")
	if !openRCDefaultContains(output, "demo") {
		t.Fatal("demo should be enabled")
	}
	if openRCDefaultContains(output, "dem") {
		t.Fatal("partial service name must not match")
	}
}

func TestDetectServiceManagerOnSupportedHost(t *testing.T) {
	manager, err := DetectServiceManager()
	if err != nil {
		t.Skipf("host has no supported active service manager: %v", err)
	}
	if manager != ServiceManagerOpenRC && manager != ServiceManagerSystemd {
		t.Fatalf("unexpected manager %q", manager)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
