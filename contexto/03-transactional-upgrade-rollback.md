# Date
2026-09-14

# Objective
Implement the Linux transactional lifecycle used by install, upgrade, repair, status and remove.

# Decisions made
- Lifecycle mutations are serialized with a non-blocking file lock.
- Fresh installs refuse unmanaged binary/service-definition conflicts.
- Installed binaries are replaced atomically and verified by SHA-256.
- Upgrades and repairs snapshot the managed binary/service definition plus declared persistent backup targets before mutation.
- Backup snapshots preserve mode, UID/GID, timestamps and symlinks on Linux.
- Required backup targets abort the upgrade if missing; optional targets may be absent.
- Backup targets may not overlap the SvcForge state directory or managed executable/service definition.
- Persistent application data is never deleted by `--remove`; only managed binary/service files and the manifest are removed.
- A failed health check triggers rollback of binary, service definition, declared data and previous service enabled/running state.
- Same-version install returns `ErrSameVersion`; repair requires the installed version.
- Status verifies the installed binary against the manifest SHA-256.

# Current architecture
`Runner` exposes portable lifecycle execution. Linux path resolution, locking and service transitions live behind Linux build tags. The transaction performs stop -> snapshot -> atomic mutation -> service start -> health check -> manifest commit. Failure after mutation invokes rollback.

# Libraries used
Go standard library only.

# Important files modified
- `runner.go`
- `runner_linux.go`
- `runner_linux_test.go`
- `backup.go`
- `copy.go`
- `hash.go`
- `health.go`
- `lock_linux.go`
- `paths_linux.go`
- `ownership_linux.go`
- `ownership_other.go`
- `manifest.go`
- `fileutil.go`
- `service_linux.go`

# Problems found
- Replacing a running service without a snapshot can leave the host unbootable if health validation fails.
- Copying backups as root without restoring ownership can corrupt application data ownership.
- A fresh install must not silently adopt or overwrite an unrelated binary/service definition.
- Application data and SvcForge-managed files require different remove semantics.

# Solutions implemented
- Transactional snapshots and rollback.
- SHA-256 manifest integrity.
- File lock and unmanaged-path conflict protection.
- Ownership/mode/timestamp-preserving backups.
- Health checks via callback or HTTP.
- Backup retention and explicit persistent-data preservation.

# Validation
- `go test ./... -count=1`: passed.
- `go vet ./...`: passed.
- `git diff --check`: passed.
- Tests cover fresh install, same version, repair, upgrade, downgrade rejection, tamper detection, rollback after health failure, persistent-data restoration, remove preservation, missing required backup, path conflicts, lock contention and metadata restoration.

# Pending
Real service-manager integration in Dex-srvforge and Windows support.

# Next steps
Integrate Dex-srvforge with SvcForge, then perform real OpenRC install/repair/upgrade/remove tests on DexOS.
