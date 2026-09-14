# Date
2026-09-14

# Objective
Create SvcForge as a reusable Go lifecycle library and validate it against an isolated Dex copy named Dex-srvforge.

# Decisions made
- Repository: `/root/SvcForge`, branch `main`, no remote yet.
- Module path: `github.com/YahirHub/svcforge`.
- Public API, runtime messages and documentation are English-only for now.
- Versioning is optional; same-version `--install` must direct the operator to `--repair`.
- The library will use a persistent install manifest to distinguish installed version, owned files, service manager and rollback state.
- Linux system service managers: systemd and OpenRC.
- Windows service support is a first-class target, but runtime validation must happen on Windows later; cross-compilation alone is not treated as runtime proof.
- Desktop shortcuts/icons are a future layer and must not complicate the first service lifecycle core.
- Dex-srvforge is a fresh Git repository copied from Dex without Dex's `.git` history or remote.
- No remote is added and nothing is pushed until the user creates the repository.

# Current architecture
Initial repository bootstrap only. The intended first stable API will expose an `App` specification and a CLI lifecycle handler. Platform-specific service and elevation code will stay behind build-tagged files while transactional file/manifest logic remains portable Go.

# Libraries used
Go standard library only at bootstrap.

# Important files modified
- `go.mod`
- `README.md`
- `AGENTS.md`
- `.gitignore`
- `contexto/00-current-state.md`
- `tareas/*`

# Problems found
Lifecycle installation combines several failure-prone domains: privilege escalation, service-manager mutation, replacing a running executable, preserving user data and rolling back after failed health checks. A single monolithic installer function would be hard to reason about and unsafe to reuse.

# Solutions implemented
The work is split into explicit Ponytail tasks with one active task at a time. The public API will remain small while state transitions receive dedicated tests.

# Pending
Core API, manifest/version logic, Linux service managers, transactional upgrade/rollback, Dex-srvforge integration, Windows support and later desktop integration.

# Next steps
Implement task 01: core types, validation, version comparison and install manifest format using only the Go standard library.

## Progress — 2026-09-14
Task 01 completed: the portable core now has validated app specifications, optional SemVer ordering, install decisions and atomic manifests. Task 02 is active for Linux systemd/OpenRC lifecycle and privilege escalation.
