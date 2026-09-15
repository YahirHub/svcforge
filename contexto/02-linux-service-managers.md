# Date
2026-09-14

# Objective
Add the Linux platform layer required for system-wide service installation without coupling lifecycle state to a specific distribution.

# Decisions made
- Detect the active service manager, not the distro name.
- Prefer systemd only when PID 1 is systemd (or `/run/systemd/system` is active) and `systemctl` exists.
- Detect OpenRC from `rc-service` + `rc-update` availability.
- Render systemd units and OpenRC scripts in Go; never concatenate user values into `sh -c`.
- Use direct argv execution for `systemctl`, `rc-service` and `rc-update`.
- OpenRC scripts use `#!/sbin/openrc-run`, default service functions and `command_background=yes` for foreground single-process applications.
- systemd supports `RestartNever`, `RestartOnFailure` and `RestartAlways`.
- OpenRC currently refuses non-never restart policies instead of silently claiming semantics that differ from systemd. Supervision will be added deliberately later if required.
- Linux elevation tries `sudo`, then `doas`, then `pkexec`, forwarding stdio to preserve interactive password prompts.

# Current architecture
`service_linux.go` owns detection, definition rendering, definition install/remove, enable/disable, actions and inspection. `privilege_linux.go` owns Linux privilege checks/re-exec. Portable types remain in `service.go` and `app.go`.

# Libraries used
Go standard library only. Platform behavior follows native systemd/OpenRC tools already present on the host.

# Important files modified
- `app.go`
- `app_test.go`
- `service.go`
- `service_linux.go`
- `service_linux_test.go`
- `privilege_linux.go`
- `fileutil.go`

# Problems found
- Service definitions are configuration languages with their own quoting rules; raw interpolation would permit malformed definitions or command injection.
- App descriptions/arguments/paths originally allowed control characters that could break generated service files.
- OpenRC restart supervision does not map perfectly to systemd's `Restart=on-failure`, so silently translating the enum would be misleading.

# Solutions implemented
- Strict control-character/path/environment validation.
- Dedicated systemd escaping and POSIX shell quoting.
- `WorkingDirectory=` is rendered as a systemd path directive rather than as a quoted command argument; this fixes Debian 13 `bad-setting` units while preserving literal `%` with `%%`.
- Deterministic environment ordering for reproducible definitions.
- OpenRC script syntax smoke via `/bin/sh -n`.
- Optional `systemd-analyze verify` smoke when available on the test host.
- Atomic service-definition writes with correct 0644/0755 modes.
- Host smoke confirms manager detection returns `openrc` on DexOS.

# Pending
Transactional binary replacement, install lock, backups, health checks, rollback, public CLI lifecycle and Dex-srvforge integration.

# Next steps
Implement task 03: transactional install/upgrade/repair/remove around the platform primitives.
