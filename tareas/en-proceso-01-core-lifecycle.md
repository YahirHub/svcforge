# Task 01 — Core lifecycle contract

## Goal
Create the minimal public API required by every later lifecycle operation.

## Scope
- `App`, executable, service, paths, upgrade, backup and health-check specifications.
- strict validation of identifiers, service names and paths;
- optional version parsing/comparison;
- persistent install manifest schema with atomic read/write;
- operation/result types for install, upgrade, repair, remove and status;
- English errors/messages;
- unit tests for validation, versions and manifest persistence.

## Constraints
- Standard library only.
- No service-manager implementation yet.
- No privilege escalation yet.
- No speculative desktop API beyond reserving it for a future task.

## Status
In progress.
