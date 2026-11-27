# DevDoctor

A local-first developer diagnostics CLI. DevDoctor scans your machine and a
project directory, detects common problems (missing tools, missing
dependencies, broken configuration), and explains likely causes with safe,
suggested fixes.

Local-first means the analysis runs entirely on your computer. Nothing is
uploaded. Secrets are never printed.

## Status

| Phase | Feature                                  | State    |
| ----- | ---------------------------------------- | -------- |
| 0     | Project scaffold, runnable CLI           | done     |
| 1     | System info (OS, architecture)           | done     |
| 2     | Developer tool detection                 | done     |
| 3     | Project detection                        | done (tests included) |
| 4     | Diagnostic rule engine                   | done (tests included) |
| 5     | Environment variable diagnostics         | done (tests included) |
| 6     | Subcommands, JSON output, exit codes     | done (tests included) |

## Run

```bash
# scan the current directory (default, human-readable text)
go run .

# same scan as one JSON document
go run . --json

# scan a specific project directory
go run . scan ./some-project

# scan a specific directory and emit JSON
go run . scan --json ./some-project
```

`--json` is accepted before or after `scan`. Unknown subcommands, unknown
flags and extra positional arguments are rejected with a short usage message
instead of being guessed at.

## Build

```bash
go build -o devdoctor .
```

## Test

```bash
go test ./...
```

Project and environment detection are tested against in-memory filesystems
(`fstest.MapFS`), so tests never touch the real disk. The CLI layer is tested
with temporary directories and a stubbed tool check, so it never runs version
commands either.

## Exit codes

DevDoctor is usable from scripts and CI because the exit code reflects what it
found, not just whether it ran:

| Code | Meaning                                                                 |
| ---- | ----------------------------------------------------------------------- |
| 0    | Scan completed with no warnings and no errors                           |
| 1    | Scan completed, but at least one warning or error was reported          |
| 2    | Invalid usage, invalid path, or the scan/runtime itself failed          |

Informational diagnostics (for example `GIT-001`, a note that the directory is
not a Git repository) are shown but do not count as problems, so a run that
reports only information still exits `0`.

```bash
go run . scan ./some-project
echo $?   # 0 when clean, 1 when something needs attention
```

Errors that prevent a scan from happening at all — bad flags, a directory that
does not exist, a path that is a file — are written to stderr and exit `2`.

## JSON output

`--json` prints a single JSON object on stdout and nothing else. It contains:

- `version` — the DevDoctor version
- `root` — the absolute path that was scanned
- `system` — `os` and `architecture`
- `tools` — `name`, `status` (`ok`, `not_found`, `error`) and `version` when known
- `facts` — project facts as `label` and `detail`
- `diagnostics` — `id`, `severity`, `title`, `description`, `evidence`, `suggestedFix`
- `summary` — counts for `info`, `warnings` and `errors`

The document is deterministic: the same inputs produce byte-identical output,
fields always appear in the same order, and empty collections are `[]` rather
than `null`.

**JSON safety guarantee:** `.env` values are never stored, so they cannot be
emitted. The scanner parses environment files into variable *names*, empty
declarations and malformed line *numbers* only; the same rule applies to text
and JSON output alike. Tool paths and underlying tool errors are also omitted —
they describe the local machine, not the project.

```bash
go run . --json | grep -c 'SECRET'   # 0 unless the word is in a variable name
```

## Principles

- Go standard library first; no dependencies without a compelling reason.
- Deterministic diagnostics before any AI features.
- Never print secret values (e.g. from `.env`).
- Every suggested fix is displayed before it could ever be executed.
- Package-manager fixes are suggested only when one lockfile identifies a
  single manager; DevDoctor never guesses between npm, pnpm, Yarn, or Bun.
- One source of truth: the CLI renders what the rules produced and never
  re-derives a problem of its own.
