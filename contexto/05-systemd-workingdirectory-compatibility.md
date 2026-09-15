# Date
2026-09-15

# Objective
Fix generated systemd units that become `bad-setting` on Debian 13 when `Service.WorkingDirectory` is configured.

# Decisions made
- `WorkingDirectory=` is a path directive, not an `ExecStart=` command-line field.
- Do not reuse `systemdCommandArg` for path directives.
- Render the validated absolute working directory directly and only escape literal `%` as `%%` so systemd does not interpret application paths as specifiers.
- Keep the existing command/environment quoting unchanged because those directives use different parsers.
- Keep OpenRC rendering unchanged; its `directory=` assignment is shell syntax and still requires `shellQuote`.
- Add an optional `systemd-analyze verify` regression test when that tool is available on the test host.

# Current architecture
`renderSystemdUnit` now renders `WorkingDirectory=` with `systemdText`, while `ExecStart=` and `Environment=` continue using `systemdCommandArg`. This intentionally reflects the different parsing rules in systemd.

# Libraries used
Go standard library only. No dependency changes.

# Important files modified
- `service_linux.go`
- `service_linux_test.go`
- `README.md`
- `contexto/00-current-state.md`
- `contexto/02-linux-service-managers.md`

# Problems found
SvcForge emitted `WorkingDirectory="/path"`. systemd's WorkingDirectory parser receives the raw directive value, expands specifiers and then validates the result as a path. The leading quote therefore makes the value fail the absolute-path check. Debian 13 reports the generated unit as `bad-setting`, and applications such as Dex cannot start through `--install` even though the binary itself runs correctly.

# Solutions implemented
- Render `WorkingDirectory=/path` without command-argument quoting.
- Preserve literal `%` with systemd's `%%` escape.
- Add regression assertions that quoted WorkingDirectory output is forbidden.
- Add a percent-specifier regression test.
- Add an optional real parser smoke using `systemd-analyze verify`.

# Pending
Runtime validation on a Debian 13/systemd host after a consumer updates to this SvcForge revision.

# Next steps
Publish the SvcForge commit, update Dex to the new pseudo-version, rebuild Dex, and retry `dex --install` on Debian 13.
