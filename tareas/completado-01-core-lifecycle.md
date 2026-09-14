# Task 01 — Core lifecycle contract

## Goal
Create the minimal public API required by every later lifecycle operation.

## Implemented
- `App`, executable, service, upgrade, backup and health-check specifications.
- Strict validation of identifiers, environment keys and absolute paths.
- Optional Semantic Version 2.0.0 parsing/comparison, including prerelease ordering and build metadata.
- Same-version and downgrade policy decisions.
- Schema-versioned install manifest with atomic `0600` persistence.
- Operation/result types.
- Unit tests for validation, SemVer, install decisions and manifest round trips.

## Validation
- `gofmt`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `git diff --check`: passed.
- Standard library only.

## Status
Completed.
