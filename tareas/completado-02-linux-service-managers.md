# Task 02 — Linux service managers

## Goal
Implement the Linux service-manager and privilege layer used by later lifecycle transactions.

## Implemented
- Active manager detection for systemd/OpenRC.
- systemd unit rendering with safe argument/environment escaping.
- OpenRC `openrc-run` rendering with safe shell quoting.
- Atomic definition writes and definition removal.
- enable/disable/start/stop/restart actions without shell command construction.
- installed/running/enabled inspection.
- Linux administrator detection and interactive re-exec through sudo/doas/pkexec.
- Validation hardening for service arguments, accounts, paths and control characters.
- OpenRC syntax smoke tests and host detection smoke.

## Known deliberate limit
OpenRC non-never restart policies are rejected for now instead of pretending they are equivalent to systemd. Manual service restart is fully supported.

## Validation
- `gofmt`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `git diff --check`: passed.
- Host detection: `openrc`.
- Standard library only.

## Status
Completed.
