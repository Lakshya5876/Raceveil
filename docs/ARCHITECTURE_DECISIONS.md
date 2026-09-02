# ARCHITECTURE_DECISIONS.md — Governance Init Decisions

ADR-style log of the load-bearing choices made during `/init-governance`
rounds 1–5, in the Guide §4.1 CHECKPOINT "decisions locked: why this over the
alternative" style. Product-level architecture decisions (language, oracle
design, persistence, licensing) already have their own ADR log at
[`Context/DECISIONS.md`](../Context/DECISIONS.md) — this file only records
choices specific to *how the repo is governed*, not what the product is.

## GOV-001 — Stack tooling: golangci-lint + gofmt -l + go test (auto-detected)
**Round 1.** Chosen over hand-rolled `go vet` + custom scripts.
**Why**: `golangci-lint` bundles `go vet`, `staticcheck`, and dozens of other
linters behind one config file, which is the de facto Go community standard
and needs no bespoke maintenance. `go test ./... -count=1` is already
auto-detected by `covenantwin.py` from `go.mod`, so no manual command wiring
was needed there.

## GOV-002 — No deploy pipeline seeded; release/hotfix hard stops dormant
**Round 2.** Chosen over pre-building a GoReleaser/cross-compile pipeline.
**Why**: none exists yet and none was requested. Building one speculatively
would violate the Guide §2.5.6a Minimum Footprint rule (no configurability not
requested by the task). The release/hotfix branch-strategy hard stops are
still written into CLAUDE.md's hard-stop table, marked dormant, so activating
them later is a one-line PR instead of a re-derivation.

## GOV-003 — No baseline.json; true zero-tolerance
**Round 4.** Chosen over ratchet leniency.
**Why**: the repo has zero lines of application code at init time — there is
nothing to grandfather, so a ratchet mechanism would add complexity purely as
future-proofing, which the Guide explicitly discourages for greenfield. If
debt philosophy ever needs to loosen, that's a human PR that writes
`.claude/baseline.json` in the exact schema `install.sh`'s brownfield path
uses — not a default enabled today.

## GOV-004 — CORE_FILES seeded with 5 globs, weighted toward the Scope guard
**Round 5.** Chosen over a minimal 2-glob seed (config + domain only) or a
broader seed including the Oracle package.
**Why**: `internal/security/scope/**` was added beyond the framework's plain
default (config + domain + DI + fixtures) because
[`Design/SECURITY.md`](../Design/SECURITY.md) explicitly calls the Scope guard
"safety-critical...covered by its own test suite" — a regression there is an
SSRF/DNS-rebinding/scope-escape class bug, not an ordinary feature bug. The
Oracle package (`internal/application/oracle/`) was deliberately **not** added
to CORE_FILES at seed time despite being architecturally central (ADR-004 in
Context/DECISIONS.md) — it's covered by its own dedicated fixture-based test
layer (TEST_PLAN Layer 3) and by `internal/testutil/`+`testdata/oracle/` being
in CORE_FILES already, which is judged sufficient without also forcing
tier-3 full-suite runs on every Oracle change. This can be revisited via PR
once the dependency graph is real (Guide §2.2.3: any module imported by >5
others joins CORE_FILES automatically as a candidate for the next human pass).

## GOV-005 — Compliance posture: none required, product security invariants unaffected
**Round 3.** Chosen to keep SECURITY.md's existing invariants (Scope
enforcement, secret redaction, destructive-action controls) load-bearing text
in CLAUDE.md regardless of the "none yet" compliance answer.
**Why**: "no SOC2/HIPAA/PCI requirement" answers an org/regulatory question;
it says nothing about whether RaceVeil's own safety-critical behavior (the
thing that makes it safe to run against a real authorized target) should be
enforced. Those are load-bearing on product-safety grounds alone, independent
of any compliance framework.
