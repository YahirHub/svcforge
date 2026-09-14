# SvcForge

SvcForge is a Go library for installing, upgrading, repairing, removing and inspecting self-contained applications and long-running services.

The project is designed for applications that ship a single executable and want predictable lifecycle commands such as:

```text
--install
--repair
--remove
--status
```

## Goals

- optional application versioning;
- idempotent installation and explicit repair semantics;
- safe upgrades with staging, backups, health checks and rollback;
- service-manager detection instead of distro-name branching;
- Linux support for systemd and OpenRC;
- Windows service support without requiring a separate installer executable;
- privilege escalation when the requested lifecycle action requires it;
- declarative preservation of databases, configuration and persistent data;
- no deletion of user data unless the application explicitly opts in;
- a small public API and minimal dependencies;
- future desktop integration for Windows Start Menu and Linux application menus.

## Version behavior

Versioning is optional. When both the installed manifest and the current application declare versions, SvcForge compares them before installation.

- newer version: upgrade;
- same version: installation stops and suggests `--repair`;
- older version: downgrade is rejected unless explicitly allowed;
- no version: lifecycle operations still work, but version ordering is skipped.

## Development status

SvcForge is under active development. `/root/Dex-srvforge` is the integration laboratory used to validate lifecycle behavior against a real service application without modifying the main Dex repository.

## Linux service managers

SvcForge detects the active manager instead of branching on distribution names. The current Linux backend supports systemd and OpenRC using their native management commands. Generated definitions are written atomically, and service commands are executed as direct argv calls rather than through a shell.
