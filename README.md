# RaceVeil

RaceVeil is a black/grey-box **concurrency-integrity verification** tool: it
tests running web applications for observable integrity violations caused by
concurrent requests — duplicate coupon redemption, gift-card double-spend,
inventory overrun, rate-limit bypass, duplicate signup — and produces
reproducible, statistically-verified, minimized evidence for each one.

It is not a DAST scanner, a request-spammer, a Burp wrapper, an AI
code-review tool, or an exploit framework. It stops at proof: a reproducible
`.rv` finding with a Wilson-interval confidence band, never an automated
exploit chain. See [`Design/SECURITY.md`](Design/SECURITY.md) for the full
safety model.

> Every existing race-discovery tool needs your source. RaceVeil needs your
> URL.

## What it does

```
$ raceveil scan http://target --scope scope.yaml --auth session.json --i-am-authorized
Discovery  endpoints:23  workflows:20  ranked:20
Ranking    probed:20  candidates with an established invariant:9
  9 experiment(s) run · 5 finding(s) · 11 endpoint(s) skipped (no invariant established)

HIGH    CONCURRENCY INTEGRITY VIOLATION                    [CONFIRMED]
POST /coupon/redeem

  Invariant     max_successes = 1   (declared)
  Observed      2 successful effects   under N=2 concurrent
  Reproduction  3/3 fresh-state (independent) trials   Wilson95 lower bound 0.438
  Evidence      primary  Level-3 cross-request: 2 distinct successful effects observed
                corrob.  Level-2 body-differential: distinct effect signatures exceed the invariant
                corrob.  Level-4 post-state: persisted count exceeds the invariant
  Confidence    CONFIRMED
  Severity      HIGH (advisory: duplicate-effect integrity violation)

  Reproduce     raceveil replay run/findings/find_redeem.rv
  Re-verify     raceveil verify  run/findings/find_redeem.rv
```

Point it at a URL (crawl/import-driven discovery) or at a hand-written
`candidate.yaml` (a declared Workflow + Invariant), and it runs the full
pipeline: baseline calibration → synchronized concurrent trials → an
Observable-State Oracle (Levels 1-4: response classification,
body-differential, cross-request consistency, post-state probe) → Wilson-
interval statistical verification → minimization → a self-contained `.rv`
finding you can hand to a teammate to `replay` or `verify` independently,
with no access to the original run.

## Evidence

Detection on a seeded, two-stack corpus (Node+Postgres, Spring
Boot+Postgres) covering six race archetypes, each with a
synchronized-safe twin — [`corpus/`](corpus/), results in
[`corpus/results/`](corpus/results/) and
[`corpus/results-spring/`](corpus/results-spring/):

| Archetype | Node | Spring Boot |
|---|---|---|
| coupon double-redemption | CONFIRMED / clean twin | CONFIRMED / clean twin |
| gift-card double-spend | CONFIRMED / clean twin | — |
| inventory overrun | CONFIRMED / clean twin | CONFIRMED / clean twin |
| duplicate order confirmation | CONFIRMED / clean twin | CONFIRMED / clean twin |
| duplicate signup | CONFIRMED / clean twin | — |
| login-attempt limit bypass | CONFIRMED / clean twin | — |

**9/9 vulnerable archetypes CONFIRMED, 9/9 synchronized-safe twins clean —
zero false positives across two independent tech stacks.**

Automatic discovery (`scan <url>`, no hand-written candidate) against a mixed
index of vulnerable and safe endpoints: 9 experiments run, 5 findings, 0
false positives on the safe twins, 11 non-idempotent endpoints correctly
skipped for lacking an established invariant.

Real-world validation:
[`benchmark/nopcommerce-cve-2024-58248/`](benchmark/nopcommerce-cve-2024-58248/)
reproduces [CVE-2024-58248](https://nvd.nist.gov/vuln/detail/CVE-2024-58248)
(nopCommerce's unlocked order-placement race) against real, unmodified
nopCommerce 4.70.5 — a `./reproduce.sh` script anyone can run locally, free,
that stands up the target and shows three concurrent order confirmations
producing three duplicate `Order` rows from one checkout.

## Quickstart

```bash
git clone https://github.com/Lakshya5876/Raceveil.git
cd Raceveil
git config core.hooksPath .githooks   # activate the governance hooks in this clone
go build ./...
go test ./... -count=1
```

Run it against the seeded corpus (see [`corpus/README.md`](corpus/README.md)
for bringing the fixtures up):

```bash
raceveil scope init --target http://127.0.0.1:4000 --out scope.yaml
raceveil scan --candidate corpus/node/candidates/coupon-vuln.yaml \
  --scope corpus/node/candidates/scope.yaml \
  --auth corpus/node/candidates/session.json \
  --out run --no-safe-mode --i-am-authorized
```

Exit codes are stable and CI-friendly (`0` no findings, `2` LIKELY+ finding,
`3` SUSPECTED-only, `4` scan error, `5` scope/authorization refusal — see
[`Design/API.md`](Design/API.md)); `raceveil report run --format sarif`
renders code-scanning-compatible SARIF for any completed run.

This repository is governed by [`CLAUDE.md`](CLAUDE.md) — read it before
making any change. `git commit` and `git push` run the mechanical covenant
(`.githooks/pre-commit`, `.githooks/pre-push`) automatically; it cannot be
skipped except via the audited `SKIP_COVENANT=1` bypass documented in the
Guide, and that path requires an interactive terminal and a typed reason.

## Commands

`scope init`/`check`, `auth login`/`check`, `scan [url] --candidate`,
`verify <finding.rv>`, `replay <finding.rv>`, `report`, `list`, `version`.
Run `raceveil <command> --help` for details, or see
[`Design/API.md`](Design/API.md) for the full command reference and CI
usage pattern.

## How it decides something is a real violation

Four things have to line up before RaceVeil calls anything CONFIRMED:

1. **An invariant must already be established** — either declared
   (`candidate.yaml`) or inferred from a baseline probe. No invariant, no
   candidate, no finding — this is the primary false-positive defense
   (`internal/application/discovery`, ADR-015 in
   [`Context/DECISIONS.md`](Context/DECISIONS.md)).
2. **Reproduction across independent, fresh-state trials**, scored with a
   Wilson score interval over `K_independent`/`r_violations` — dependent or
   unknown-independence trials are excluded from the statistics (retained
   only as evidence), never folded in to inflate confidence.
3. **A stopping rule that's exact, not heuristic**: a trial loop stops the
   moment no remaining trial could still move the Confidence band, so
   RaceVeil never runs more concurrent bursts against a live target than
   the evidence actually requires (`internal/application/oracle/stopping.go`).
4. **CONFIRMED requires corroboration** — the primary Level-3 (cross-request
   consistency) signal plus at least one independent observable (Level-2
   body-differential or Level-4 post-state probe). Level 3 alone caps at
   LIKELY, always.

Every outbound request passes through a single Scope/Authorization guard
chokepoint (`internal/security/scope`): default-deny host/port/path
allowlist, IP pinning against DNS rebinding, private/metadata-IP blocking,
per-run caps, and a destructive-verb policy. RaceVeil never exceeds the
declared `proof.max_successful_effects` to force a proof — an experiment
that would need more is refused as unprovable-under-cap, not overridden.

## Layout

```
cmd/raceveil/            CLI entrypoint (thin)
internal/domain/         entities (Design/DOMAIN.md) — zero external deps
internal/application/    orchestration: scheduler, oracle, minimize, discovery, ranking
internal/infrastructure/ I/O: sync engine (H2SinglePacket/H1LastByte), crawler, filesystem persistence
internal/presentation/   Cobra CLI commands, human/JSON/SARIF/Markdown output
internal/security/scope/ the Scope/Authorization guard chokepoint
internal/config/         single config-access module
internal/wiring/         DI wiring (presentation -> application, never infrastructure directly)
corpus/                  seeded two-stack vulnerable corpus + synchronized-safe twins, Docker Compose
benchmark/                reproducible real-world validation (CVE-2024-58248 rediscovery)
Design/                  full product specification
Context/                 architecture decisions (Context/DECISIONS.md) + roadmap (Context/ROADMAP.md)
docs/                    governance-scoped spec derivatives (PRD/TRD/SYSTEM_DESIGN/ARCHITECTURE_DECISIONS)
```

## Status

Phases 0-5 of [`Context/ROADMAP.md`](Context/ROADMAP.md) are implemented:
Oracle Levels 1-4 with the corroboration rule, all four invariant types
(`max_successes`, `uniqueness`, `monotonic_limit`, `single_transition`),
reproduction + minimization, automatic discovery (crawler + HAR/OpenAPI
import + two-phase ranking + invariant-inference gate), Wilson-interval
statistical verification with an exact stopping rule, SARIF/JSON/Markdown
reporting, the two-stack seeded corpus with safe twins, and a real-world
CVE rediscovery. PortSwigger Academy lab validation is deliberately deferred
(ADR-017, `Context/DECISIONS.md`) — those labs are live, account-gated,
remote infrastructure incompatible with this project's zero-cost/local-first
constraint; picking it up needs the user's own Academy access.

Phase 6 (multi-endpoint/partial-construction workflows, a user-defined
invariant DSL, WebSocket/HTTP-3 delivery, deeper traffic import) is
explicitly not started — per `Context/ROADMAP.md`'s own rule, it's taken up
only when a concrete need demands it, not spun up for its own sake.
