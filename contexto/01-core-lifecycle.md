# Date
2026-09-14

# Objective
Establish the portable SvcForge contract before any platform mutation code exists.

# Decisions made
- `App.Version` is optional.
- Declared versions follow Semantic Versioning 2.0.0; a Go-style leading `v` is accepted.
- Equal precedence rejects `--install` with `ErrSameVersion` and directs callers toward repair.
- Downgrades are blocked unless `UpgradePolicy.AllowDowngrade` is true.
- App/service identifiers are restricted to predictable ASCII service-safe characters.
- Install manifests are schema-versioned JSON, written atomically with mode `0600`.
- The first manifest schema records app identity, version, binary path, service manager/name, timestamps and managed files.
- Backup/health structs are defined now because upgrade safety needs them, but execution remains for later tasks.

# Current architecture
The root package currently contains portable types, validation, Semantic Version comparison, manifest persistence and install decision logic. It has no service-manager, elevation or filesystem replacement side effects yet.

# Libraries used
Go standard library only.

# Important files modified
- `app.go`
- `version.go`
- `manifest.go`
- `lifecycle.go`
- `app_test.go`
- `version_test.go`
- `manifest_test.go`

# Problems found
A simple string comparison for versions would order values such as `1.10.0` incorrectly and would mishandle prereleases. A non-atomic manifest write could leave future repairs/upgrades without trustworthy installed-state metadata.

# Solutions implemented
A small SemVer parser/comparator was implemented in standard Go, including numeric prerelease precedence and build-metadata neutrality. Manifest writes use same-directory temporary files, `Sync`, restrictive permissions and `Rename`.

# Pending
Linux service managers, privilege escalation, transactional install/upgrade/rollback, integration testing and Windows service support.

# Next steps
Implement systemd/OpenRC service-manager detection, rendering and lifecycle operations plus Linux privilege re-exec.
