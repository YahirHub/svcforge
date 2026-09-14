# SvcForge project rules

- Public API, exported comments, runtime messages, README, docs and examples are written in English.
- Git commits are written in Spanish.
- Use the Ponytail methodology: standard library first, no speculative abstractions, no dependencies without a concrete need.
- Security, validation, backups, rollback, error handling and tests are not optional simplifications.
- Keep `contexto/` and `tareas/` current. Exactly one task may be `en-proceso-*` at a time.
- Go code must pass `gofmt`, `go test ./...` and `go vet ./...`.
- The library must remain usable without CGO whenever the target platform permits it.
- Never add a remote or push unless the user explicitly asks after creating the repository.
- Git identity for this repository is the user's local identity already configured in `.git/config`.
