# /feature — Full Implementation Pipeline

Runs Phases 0–5 of the Guide's agentic pipeline (`v1_claude_code_development_guide_new.md`
§3.2) against this repo's actual stack. Read `CLAUDE.md` first if it has not
been read this session, or if `covenant.sh`/`covenantwin.py` reports a hash
drift warning.

## PHASE 0 — Pre-flight
- `git status` clean or changes understood; branch is not `main`/`master`/
  `develop` (Guide §2.5.1 — hard block otherwise, zero file reads).
- Branch prefix matches `feature/*`, `bugfix/*`, `hotfix/*`, `release/*`; a
  mismatch gets a one-line warning + explicit acknowledgement before continuing.
- Suite currently green: `go build ./...` and `go test ./... -count=1` both
  exit 0 before starting.

## PHASE 1 — Reconnaissance (zero writes)
- `grep`/`find` for symbols; `Read` with explicit offset/limit (Guide §2.5.5 —
  never `cat` a whole file speculatively).
- Produce an explicit change manifest: which files, which layer each belongs
  to (CLAUDE.md §1).
- >> CHECKPOINT EVALUATION (CLAUDE.md §10) <<

## PHASE 2 — Design Declaration (zero writes)
- Layer assignment per file, typed Go signatures, error states, named test
  list (file + test name) — before any implementation exists.
- State:
  ```
  GOAL:  <concrete, observable outcome>
  VERIFY: <exact go test / golangci-lint invocation that proves it>
  ```
- If a reasonable ambiguity exists, declare it — do not silently pick:
  ```
  ASSUMING:    <interpretation being proceeded on>
  ALTERNATIVE: <what changes if wrong>
  ```
- Cannot answer something with reasonable confidence? STOP — use
  `AskUserQuestion` (CLAUDE.md §8, Guide §2.5.6b). Never assume, never bury
  the question in prose.

## PHASE 2.5 — Stubs-First (mandatory at 3+ files)
- Write ALL stub signatures across ALL files first: real imports, typed
  signatures, zero-value/empty returns.
- Compile-check everything at once: `go build ./...` — exit 0 required
  before any real logic lands.

## PHASE 3 — Implementation (strict layer order)
`internal/domain` -> `internal/infrastructure` -> `internal/application` ->
`internal/presentation` -> tests (co-located `_test.go` files).
After each file: `go build ./...` + that package's `go test ./<pkg>/... -count=1`.
>> CHECKPOINT EVALUATION <<

## PHASE 4 — Verification Loop (max 3 attempts, Guide §3.3)
```bash
git update-index -q --refresh; git diff --no-ext-diff
go build ./...
go vet ./...
gofmt -l .                    # must be empty
golangci-lint run ./...
gocyclo -over 10 .             # must be empty (complexity threshold 10)
go test ./... -count=1
```
- Feature-scoped tests first, then the full suite (cheap while <~60s per
  CLAUDE.md §6).
- **Scope Discipline check (Guide §2.5.6a) runs here, before the checkpoint
  write, not after the diff is staged**: minimum footprint, surgical
  boundary, no drive-by edits.
- On any error: root-cause in one sentence before fixing (Q1: does the
  symbol exist? Q2: correct form? Q3: where's the bad reference?). Three
  strikes then STOP and report (never a 4th attempt).
- >> CHECKPOINT EVALUATION <<

## PHASE 5 — Output
- File manifest table, full verbatim test output, Conventional Commit
  message (`feat|fix|refactor|test|perf|security|docs|chore(scope): ...`).
- >> CHECKPOINT WRITE (always) <<
- Then the commit/push covenant, CLAUDE.md §9 — `/audit` and `/review` before
  `git commit`; explicit chat confirmation via `AskUserQuestion` before any
  `git push`, stated as its own exchange, never inferred from earlier text.

## Cost warning (Guide §7.1.1)
If a single phase consumes >40,000 context tokens, or history retransmission
exceeds 50%, emit a high-visibility warning recommending `/compact` or
restart before continuing.
