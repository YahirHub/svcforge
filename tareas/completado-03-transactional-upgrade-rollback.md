# Task 03 — Transactional upgrade, backup and rollback

## Goal
Implement safe install/upgrade/repair/remove transactions around the Linux service-manager primitives.

## Implemented
- lifecycle lock;
- atomic binary replacement;
- SHA-256 verification;
- optional/required declarative backup targets;
- ownership/mode/timestamp/symlink-preserving snapshots;
- HTTP/callback health checks;
- rollback of binary, service definition, application data and service state;
- backup retention;
- unmanaged-path conflict protection;
- status integrity checks;
- remove that preserves persistent application data.

## Validation
- `go test ./... -count=1`: passed.
- `go vet ./...`: passed.
- `git diff --check`: passed.

## Status
Completed.
