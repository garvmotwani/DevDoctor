# DevDoctor

A local-first developer diagnostics CLI. DevDoctor scans your machine and the
project in your current directory, detects common problems (missing tools,
missing dependencies, broken configuration), and explains likely causes with
safe, suggested fixes.

Local-first means the analysis runs entirely on your computer. Nothing is
uploaded. Secrets are never printed.

## Status

| Phase | Feature                          | State    |
| ----- | -------------------------------- | -------- |
| 0     | Project scaffold, runnable CLI   | done     |
| 1     | System info (OS, architecture)   | done     |
| 2     | Developer tool detection         | done     |
| 3     | Project detection                | done (tests included) |
| 4     | Diagnostic rule engine           | done (tests included) |
| 5     | Environment variable diagnostics | done (tests included) |

## Run

```bash
go run .
```

## Build

```bash
go build -o devdoctor .
```

## Test

```bash
go test ./...
```

Project and environment detection are tested against in-memory filesystems
(`fstest.MapFS`), so tests never touch the real disk.

Environment diagnostics (`ENV-001`..`ENV-004`) report variable *names*,
emptiness and malformed line numbers. `.env` values are never stored, so they
cannot be printed by accident.

## Principles

- Go standard library first; no dependencies without a compelling reason.
- Deterministic diagnostics before any AI features.
- Never print secret values (e.g. from `.env`).
- Every suggested fix is displayed before it could ever be executed.
- Package-manager fixes are suggested only when one lockfile identifies a
  single manager; DevDoctor never guesses between npm, pnpm, Yarn, or Bun.
