# TRD.md — RaceVeil (Governance Spec)

Technical requirements as established in init rounds 1–4. Full architectural
rationale lives in [`Design/ARCHITECTURE.md`](../Design/ARCHITECTURE.md) and
[`Context/DECISIONS.md`](../Context/DECISIONS.md); this document is the
scoped derivative that drives CLAUDE.md and the covenant configuration.

## 1. Stack (Round 1 — confirmed)

| Concern | Decision | Source |
|---|---|---|
| Language / runtime | Go, single static binary | ADR-008 |
| Distribution | Cross-compiled binaries, no daemon | ARCHITECTURE.md §1 |
| Interface | CLI only (Cobra) | ARCHITECTURE.md §1 |
| Persistence | Filesystem run directory (JSON/JSONL), **no database** | ADR-011 |
| HTTP client (crawl/baseline) | Go stdlib `net/http` | ARCHITECTURE.md §1 |
| Synchronized concurrency engine | Custom, over `golang.org/x/net/http2` Framer (h2) / raw `crypto/tls` (h1) | ADR-009 |
| Config access | Single config module only — no raw `os.Getenv`/flag reads in feature code | Universal security invariant, Guide §2.2.1 |
| Test runner | `go test ./... -count=1` (auto-detected by covenantwin.py from `go.mod`) | Confirmed |
| Linter | `golangci-lint` (includes `go vet` + `staticcheck`) | Confirmed |
| Format check | `gofmt -l` | Confirmed |
| Complexity scanner | `gocyclo` on changed files, threshold 10 | Guide §4.4 default |
| License | Apache-2.0; no GPL-3 runtime dependency | ADR-009, ADR-016 |

## 2. Operational reality (Round 2 — confirmed)

- **Deploy pipeline**: none yet. This is local-only / CI-test-only for now.
  Consequence: the release/hotfix branch-strategy hard stops in CLAUDE.md are
  seeded as **dormant placeholders** — present in the hard-stops table,
  explicitly marked inactive until a release pipeline exists. Activating them
  is a human edit (CLAUDE.md PR), never something the agent infers on its own.
- **Schema/migrations**: not applicable. Persistence is filesystem-only
  (ADR-011) — `config.yaml`, `*.jsonl`, `experiments/**`, `findings/*.rv`,
  `report.*`. No SQL layer exists or is planned for v1. See
  [`docs/DB_SCHEMA.md`](DB_SCHEMA.md).

## 3. Risk posture (Round 3 — confirmed)

- **Compliance**: none today (no SOC2/HIPAA/PCI requirement). This does not
  relax RaceVeil's own product-level security invariants — the Scope guard,
  IP pinning, secret redaction, and destructive-action controls in
  [`Design/SECURITY.md`](../Design/SECURITY.md) are load-bearing regardless,
  because they are what makes RaceVeil itself safe to run against authorized
  targets. Compliance frameworks and product safety invariants are separate
  axes; only the former is "none yet" here.
- **Team**: solo/small, already comfortable with Claude Code governance
  tooling. Default framework friction (Tier-3 brainstorming per Guide §2.5.9,
  standard Execution Mode Menu thresholds) applies as-is — no extra tightening
  requested.

## 4. Debt philosophy (Round 4 — confirmed)

**True zero-tolerance from commit one.** Any lint/test/type failure blocks;
no ratchet. This is the framework default for greenfield and matches the
repo's actual state (no code exists yet — there is nothing to grandfather).
Consequence: **no `.claude/baseline.json` is written.** A missing baseline is
mechanically treated by `covenantwin.py` as zero-tolerance, which is the
intended state, not a gap.

## 5. Architecture layering (Go idiom, per Guide §2.2)

Adapted from the four-layer clean-architecture model to Go convention and to
RaceVeil's actual component shape (a CLI pipeline, not a web service — see
[`docs/SYSTEM_DESIGN.md`](SYSTEM_DESIGN.md) for the full mapping):

```
cmd/raceveil/            <- thin main.go: wiring + Execute()
internal/domain/         <- entities, invariants, zero deps (DOMAIN.md vocabulary)
internal/application/    <- Experiment Scheduler, Oracle evaluation, Statistical
                             Verification, Minimization — orchestration/business logic
internal/infrastructure/ <- HTTP client, sync engine (H2SinglePacket/H1LastByte),
                             filesystem run-directory persistence, crawler/import
internal/presentation/   <- Cobra commands, human/JSON/SARIF output rendering,
                             progress UI, input validation, safe-mode confirmation
internal/security/scope/ <- the Scope/Authorization guard chokepoint (CORE_FILES)
internal/config/         <- single config-access module (CORE_FILES)
internal/wiring/         <- DI wiring for CLI commands (CORE_FILES)
internal/testutil/       <- shared test fixtures/helpers (CORE_FILES)
testdata/oracle/         <- frozen oracle fixtures per TEST_PLAN Layer 3 (CORE_FILES)
```

Test mirror: `<pkg>/x.go` → `<pkg>/x_test.go` (Go idiom — same package
directory, not a separate `tests/` tree), preserving the naming-contract
mirror rule in spirit: deterministic module→test mapping.

## 6. Naming contracts (Go idiom of Guide §2.2)

| Guide concept | Go form |
|---|---|
| Repositories: `fetch_*`/`find_*`/`persist_*`/`remove_*` | Same verb prefixes on `Store`/`Repository` interface methods, e.g. `FetchFinding`, `PersistTrial` |
| Use cases: `Execute*UseCase`/`Query*UseCase` | `Execute*` / `Query*` functions or `*Scheduler`/`*Evaluator` types in `internal/application` |
| Entities: PascalCase nouns | `Workflow`, `Candidate`, `Finding` (Go idiom is PascalCase by default) |
| Events: past-tense | `ViolationDetected`, `TrialCompleted` (where domain events are modeled) |
| Tests: mirror source | `<file>_test.go` beside `<file>.go`, package-internal |
