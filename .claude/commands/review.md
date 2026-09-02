# /review — Ledger-Aware Pre-PR Covenant

## Step 0 — Ledger check
Recompute the FULL fingerprint (Guide §4.2 — tree + staged diff + unstaged
diff + untracked files, pinned diff config) and compare against
`.claude/covenant_state.json`. Exact match → **SKIP loudly**, printing the
script-generated COVENANT REPORT verbatim (never a model-composed one — if
`covenantwin.py` did not actually run, there is no report to print). Any
mismatch → run the full review below.

## Diff inventory
```bash
git update-index -q --refresh; git diff --no-ext-diff
git diff main...HEAD --name-only
```
Every changed file must be explainable in one sentence against the current
task's GOAL (CLAUDE.md §7).

## Dependency-manifest assertion (this stack's lockfile equivalent)
```bash
git diff main...HEAD -- go.mod go.sum
```
Any diff here without a prior hard-stop approval for that specific dependency
(CLAUDE.md §2.2 table) = **HARD STOP**. This is RaceVeil's equivalent of the
Guide's lockfile assertion — `go.sum` is the integrity-pinned lockfile.

## Per-file layer compliance
For every changed file under `internal/`, confirm it matches CLAUDE.md §1:
- `internal/domain/**` imports nothing outside stdlib + itself.
- `internal/application/**` does not import `net/http` directly for outbound
  calls, does not import Cobra, does not perform raw socket/TLS I/O itself.
- `internal/presentation/**` does not import `internal/infrastructure`
  directly — only via `internal/wiring` → `internal/application`.
- `internal/infrastructure/**` does not import `internal/application`.
- Any outbound-request code outside `internal/security/scope` and
  `internal/infrastructure` is a layer violation — flag it.

## Secrets-in-diff grep
```bash
git diff main...HEAD | grep -iE "password|secret|api[_-]?key|token|bearer" 
```
Zero matches expected outside test fixtures/templated `${ENV}` placeholders
(Design/SECURITY.md — `.rv` artifacts template secrets, never embed them).

## Coverage check
```bash
go test ./... -count=1 -cover
```
Every new function in `internal/application` or `internal/domain` has a named
test (CLAUDE.md §6 N1). Threshold 80% (CLAUDE.md §6, Guide §4.4 default);
block if it drops below.

## Quarantine report
```bash
cat quarantine.txt
```
Print the quarantine count and covered modules on every run. A quarantined
test covering a CORE_FILES module (CLAUDE.md §4) is a **HARD STOP**.

## Conventional-commit check
Verify the proposed commit message matches
`type(scope): imperative description` with `type` in
`feat|fix|refactor|test|perf|security|docs|chore`.

## PR body generation
Summary (why, not just what), test plan checklist (`go build ./...`,
`go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `gocyclo -over 10 .`,
`go test ./... -count=1 -cover`), and a note on which CLAUDE.md layer(s) the
change touches.

## Finish
Have the covenant script (`covenantwin.py`, via the pre-commit hook — never
this command directly) write the new receipt atomically. This command
reports; it does not write `.claude/covenant_state.json` itself
(`Write`/`Edit` on that file is denied to the agent by design, CLAUDE.md §5).
