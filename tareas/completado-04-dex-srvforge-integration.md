# Task 04 — Dex-srvforge integration

## Goal
Replace the copied Dex installer lifecycle with SvcForge and perform real install/repair/same-version/upgrade/rollback/remove tests on OpenRC.

## Result
Completed and validated on the real DexOS/OpenRC host.

## Covered
- fresh install;
- same-version no-op;
- repair service definition;
- detect/repair tampered binary;
- upgrade;
- rollback after forced health failure;
- self-remove from installed binary;
- persistent config preservation;
- reinstall;
- backup retention;
- final health/status/integrity.

## Final runtime
Dex-srvforge 0.2.0 is installed and running under OpenRC with SvcForge lifecycle metadata.

## Status
Completed.
