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
| 4     | Diagnostic rule engine           | not yet  |

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

Project detection is tested against in-memory filesystems (`fstest.MapFS`),
so tests never touch the real disk.

## Principles

- Go standard library first; no dependencies without a compelling reason.
- Deterministic diagnostics before any AI features.
- Never print secret values (e.g. from `.env`).
- Every suggested fix is displayed before it could ever be executed.
