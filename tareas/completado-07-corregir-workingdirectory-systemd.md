# Task 07 — Fix systemd WorkingDirectory rendering

## Objective
Make SvcForge render `WorkingDirectory=` using systemd path-directive syntax instead of command-argument quoting, so generated services load correctly on Debian 13/systemd.

## Root cause
`renderSystemdUnit` used `systemdCommandArg` for `WorkingDirectory`. That helper deliberately wraps values in double quotes for `ExecStart=`/`Environment=` parsing. systemd's `WorkingDirectory=` parser receives the raw directive value, expands specifiers and then validates the result as a path; the generated `WorkingDirectory="/"` therefore begins with a quote and is rejected as a bad unit setting.

## Implemented
- `WorkingDirectory=` now uses path-directive rendering through `systemdText`.
- Literal `%` remains escaped as `%%` so application paths are not interpreted as systemd specifiers.
- `ExecStart=` and `Environment=` keep their existing command/value quoting.
- OpenRC rendering is unchanged.
- Regression test requires `WorkingDirectory=/var/lib/demo` and explicitly rejects `WorkingDirectory="/var/lib/demo"`.
- Added a literal-percent regression test.
- Added an optional `systemd-analyze verify` smoke when the tool is installed.
- Updated README and persistent context.

## Verification
- `gofmt` applied.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- `systemd-analyze` is not installed on the current DexOS/OpenRC host, so that optional parser smoke skipped cleanly; it remains active automatically on hosts/CI where systemd tooling exists.

## Status
Completed.
