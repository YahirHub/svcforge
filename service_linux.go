//go:build linux

package svcforge

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type linuxServiceLayout struct {
	systemdUnitDir string
	openRCInitDir  string
}

func defaultLinuxServiceLayout() linuxServiceLayout {
	return linuxServiceLayout{
		systemdUnitDir: "/etc/systemd/system",
		openRCInitDir:  "/etc/init.d",
	}
}

// DetectServiceManager returns the active supported Linux service manager.
func DetectServiceManager() (ServiceManager, error) {
	pid1, _ := os.ReadFile("/proc/1/comm")
	if strings.TrimSpace(string(pid1)) == "systemd" {
		if _, err := exec.LookPath("systemctl"); err == nil {
			return ServiceManagerSystemd, nil
		}
	}
	if _, rcServiceErr := exec.LookPath("rc-service"); rcServiceErr == nil {
		if _, rcUpdateErr := exec.LookPath("rc-update"); rcUpdateErr == nil {
			return ServiceManagerOpenRC, nil
		}
	}
	if info, err := os.Stat("/run/systemd/system"); err == nil && info.IsDir() {
		if _, pathErr := exec.LookPath("systemctl"); pathErr == nil {
			return ServiceManagerSystemd, nil
		}
	}
	return "", ErrServiceManagerUnavailable
}

func renderSystemdUnit(app App) ([]byte, error) {
	if err := app.Validate(); err != nil {
		return nil, err
	}
	if !app.Service.Enabled {
		return nil, errors.New("service is not enabled in app specification")
	}
	if app.Executable.InstallPath == "" || !filepath.IsAbs(app.Executable.InstallPath) {
		return nil, errors.New("executable install path is required to render a service")
	}
	description := app.Service.Description
	if description == "" {
		description = app.Description
	}
	if description == "" {
		description = app.DisplayName
	}
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=")
	b.WriteString(systemdText(description))
	b.WriteByte('\n')
	if app.Service.DependOnNetwork {
		b.WriteString("Wants=network-online.target\nAfter=network-online.target\n")
	}
	b.WriteString("\n[Service]\nType=simple\nExecStart=")
	b.WriteString(systemdCommandArg(app.Executable.InstallPath))
	for _, arg := range app.Service.Arguments {
		b.WriteByte(' ')
		b.WriteString(systemdCommandArg(arg))
	}
	b.WriteByte('\n')
	if app.Service.WorkingDirectory != "" {
		b.WriteString("WorkingDirectory=")
		b.WriteString(systemdCommandArg(app.Service.WorkingDirectory))
		b.WriteByte('\n')
	}
	if app.Service.User != "" {
		b.WriteString("User=")
		b.WriteString(systemdText(app.Service.User))
		b.WriteByte('\n')
	}
	if app.Service.Group != "" {
		b.WriteString("Group=")
		b.WriteString(systemdText(app.Service.Group))
		b.WriteByte('\n')
	}
	keys := sortedEnvironmentKeys(app.Service.Environment)
	for _, key := range keys {
		b.WriteString("Environment=")
		b.WriteString(systemdCommandArg(key + "=" + app.Service.Environment[key]))
		b.WriteByte('\n')
	}
	switch app.Service.Restart {
	case RestartAlways:
		b.WriteString("Restart=always\nRestartSec=2s\n")
	case RestartOnFailure:
		b.WriteString("Restart=on-failure\nRestartSec=2s\n")
	default:
		b.WriteString("Restart=no\n")
	}
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return []byte(b.String()), nil
}

func renderOpenRCScript(app App) ([]byte, error) {
	if err := app.Validate(); err != nil {
		return nil, err
	}
	if !app.Service.Enabled {
		return nil, errors.New("service is not enabled in app specification")
	}
	if app.Service.Restart != "" && app.Service.Restart != RestartNever {
		return nil, fmt.Errorf("OpenRC restart policy %q is not implemented yet; use RestartNever or manage supervision explicitly", app.Service.Restart)
	}
	if app.Executable.InstallPath == "" || !filepath.IsAbs(app.Executable.InstallPath) {
		return nil, errors.New("executable install path is required to render a service")
	}
	serviceName := app.serviceName()
	description := app.Service.Description
	if description == "" {
		description = app.Description
	}
	if description == "" {
		description = app.DisplayName
	}
	var b strings.Builder
	b.WriteString("#!/sbin/openrc-run\n\n")
	b.WriteString("name=")
	b.WriteString(shellQuote(app.DisplayName))
	b.WriteByte('\n')
	b.WriteString("description=")
	b.WriteString(shellQuote(description))
	b.WriteByte('\n')
	b.WriteString("command=")
	b.WriteString(shellQuote(app.Executable.InstallPath))
	b.WriteByte('\n')
	if len(app.Service.Arguments) > 0 {
		quoted := make([]string, 0, len(app.Service.Arguments))
		for _, arg := range app.Service.Arguments {
			quoted = append(quoted, shellQuote(arg))
		}
		b.WriteString("command_args=")
		b.WriteString(shellQuote(strings.Join(quoted, " ")))
		b.WriteByte('\n')
	}
	b.WriteString("command_background=yes\n")
	b.WriteString("pidfile=")
	b.WriteString(shellQuote(filepath.Join("/run", serviceName+".pid")))
	b.WriteByte('\n')
	if app.Service.WorkingDirectory != "" {
		b.WriteString("directory=")
		b.WriteString(shellQuote(app.Service.WorkingDirectory))
		b.WriteByte('\n')
	}
	if app.Service.User != "" {
		user := app.Service.User
		if app.Service.Group != "" {
			user += ":" + app.Service.Group
		}
		b.WriteString("command_user=")
		b.WriteString(shellQuote(user))
		b.WriteByte('\n')
	}
	if app.Service.LogPath != "" {
		b.WriteString("output_log=")
		b.WriteString(shellQuote(app.Service.LogPath))
		b.WriteByte('\n')
		b.WriteString("error_log=")
		b.WriteString(shellQuote(app.Service.LogPath))
		b.WriteByte('\n')
	}
	for _, key := range sortedEnvironmentKeys(app.Service.Environment) {
		b.WriteString("export ")
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(shellQuote(app.Service.Environment[key]))
		b.WriteByte('\n')
	}
	if app.Service.DependOnNetwork {
		b.WriteString("\ndepend() {\n    need net\n}\n")
	}
	return []byte(b.String()), nil
}

func serviceDefinitionPathLinux(app App, manager ServiceManager, layout linuxServiceLayout) (string, error) {
	switch manager {
	case ServiceManagerSystemd:
		return filepath.Join(layout.systemdUnitDir, systemdUnitName(app.serviceName())), nil
	case ServiceManagerOpenRC:
		return filepath.Join(layout.openRCInitDir, app.serviceName()), nil
	default:
		return "", fmt.Errorf("unsupported Linux service manager %q", manager)
	}
}

func reloadServiceManagerLinux(manager ServiceManager) error {
	if manager == ServiceManagerSystemd {
		return runServiceCommand("systemctl", "daemon-reload")
	}
	if manager == ServiceManagerOpenRC {
		return nil
	}
	return fmt.Errorf("unsupported Linux service manager %q", manager)
}

func installServiceDefinitionLinux(app App, manager ServiceManager, layout linuxServiceLayout) (string, error) {
	var (
		path string
		data []byte
		mode os.FileMode
		err  error
	)
	path, err = serviceDefinitionPathLinux(app, manager, layout)
	if err != nil {
		return "", err
	}
	switch manager {
	case ServiceManagerSystemd:
		data, err = renderSystemdUnit(app)
		mode = 0o644
	case ServiceManagerOpenRC:
		data, err = renderOpenRCScript(app)
		mode = 0o755
	}
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data, mode); err != nil {
		return "", err
	}
	if err := reloadServiceManagerLinux(manager); err != nil {
		return "", err
	}
	return path, nil
}

func removeServiceDefinitionLinux(app App, manager ServiceManager, layout linuxServiceLayout) error {
	path, err := serviceDefinitionPathLinux(app, manager, layout)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove service definition %s: %w", path, err)
	}
	return reloadServiceManagerLinux(manager)
}

func enableServiceLinux(app App, manager ServiceManager) error {
	switch manager {
	case ServiceManagerSystemd:
		return runServiceCommand("systemctl", "enable", systemdUnitName(app.serviceName()))
	case ServiceManagerOpenRC:
		return runServiceCommand("rc-update", "add", app.serviceName(), "default")
	default:
		return fmt.Errorf("unsupported Linux service manager %q", manager)
	}
}

func disableServiceLinux(app App, manager ServiceManager) error {
	switch manager {
	case ServiceManagerSystemd:
		return runServiceCommand("systemctl", "disable", systemdUnitName(app.serviceName()))
	case ServiceManagerOpenRC:
		return runServiceCommand("rc-update", "del", app.serviceName(), "default")
	default:
		return fmt.Errorf("unsupported Linux service manager %q", manager)
	}
}

func startServiceLinux(app App, manager ServiceManager) error {
	return serviceActionLinux(app, manager, "start")
}

func stopServiceLinux(app App, manager ServiceManager) error {
	return serviceActionLinux(app, manager, "stop")
}

func restartServiceLinux(app App, manager ServiceManager) error {
	return serviceActionLinux(app, manager, "restart")
}

func serviceActionLinux(app App, manager ServiceManager, action string) error {
	switch manager {
	case ServiceManagerSystemd:
		return runServiceCommand("systemctl", action, systemdUnitName(app.serviceName()))
	case ServiceManagerOpenRC:
		return runServiceCommand("rc-service", app.serviceName(), action)
	default:
		return fmt.Errorf("unsupported Linux service manager %q", manager)
	}
}

func inspectServiceLinux(app App, manager ServiceManager, layout linuxServiceLayout) (ServiceState, error) {
	state := ServiceState{Manager: manager}
	switch manager {
	case ServiceManagerSystemd:
		state.DefinitionPath = filepath.Join(layout.systemdUnitDir, systemdUnitName(app.serviceName()))
	case ServiceManagerOpenRC:
		state.DefinitionPath = filepath.Join(layout.openRCInitDir, app.serviceName())
	default:
		return state, fmt.Errorf("unsupported Linux service manager %q", manager)
	}
	if _, err := os.Stat(state.DefinitionPath); err == nil {
		state.Installed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return state, fmt.Errorf("stat service definition: %w", err)
	}
	if !state.Installed {
		return state, nil
	}
	switch manager {
	case ServiceManagerSystemd:
		state.Running = commandSucceeds("systemctl", "is-active", "--quiet", systemdUnitName(app.serviceName()))
		state.Enabled = commandSucceeds("systemctl", "is-enabled", "--quiet", systemdUnitName(app.serviceName()))
	case ServiceManagerOpenRC:
		state.Running = commandSucceeds("rc-service", app.serviceName(), "status")
		output, err := exec.Command("rc-update", "show", "default").Output()
		if err == nil {
			state.Enabled = openRCDefaultContains(output, app.serviceName())
		}
	}
	return state, nil
}

func runServiceCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func commandSucceeds(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func openRCDefaultContains(output []byte, serviceName string) bool {
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) > 0 && string(fields[0]) == serviceName {
			return true
		}
	}
	return false
}

func sortedEnvironmentKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func systemdUnitName(serviceName string) string {
	if strings.HasSuffix(serviceName, ".service") {
		return serviceName
	}
	return serviceName + ".service"
}

func systemdText(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

func systemdCommandArg(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.ReplaceAll(value, "$", "$$")
	return "\"" + value + "\""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
