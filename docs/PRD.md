# PRD.md — RaceVeil (Governance Spec)

> This is the governance-framework PRD (init round 0). It scopes the constitution
> and CORE_FILES against a fixed "what's in v1" line. The full product reasoning —
> prior art, oracle design rationale, threat model — lives in
> [`Design/PRD.md`](../Design/PRD.md) and is not repeated here; this document is
> the scoped, requirements-shaped derivative used to drive CLAUDE.md.

## 1. Problem

Concurrent requests let callers do things a web application was supposed to
forbid — double-redeem a coupon, overdraw a balance, bypass a rate limit — even
when the code passes every sequential test. No mainstream tool proves this
class of bug against a **running system with no source access**. RaceVeil is
that external, black/grey-box verification layer.

## 2. Primary users

1. Backend/platform engineers who want a CI check that concurrency invariants
   hold in the deployed service.
2. AppSec engineers/pentesters who currently do this by hand.
3. Bug-bounty hunters testing authorized targets.
4. OSS maintainers guarding against reintroducing a known race.

## 3. Scope for this build (v1 = MVP = ROADMAP Phases 0–2)

Confirmed round 0: "first version" is the MVP, defined in
[`Context/ROADMAP.md`](../Context/ROADMAP.md) as Phases 0–2.

**In scope:**
- Phase 0 — project skeleton, Session handling, Scope/Authorization guard
  (allowlist, IP pinning, caps) with its own test suite, sequential experiment
  runner, run-directory persistence, audit log, secret redaction.
- Phase 1 — `sync` package (`H2SinglePacket`, `H1LastByte`), `candidate.yaml`
  manual-candidate input, `scan --candidate`, Baseline calibration, Oracle
  Levels 1 + 3, `max_successes` invariant only, Wilson confidence over
  independent trials, `.rv` finding format, `replay`, `verify`. Confidence
  capped at LIKELY (no corroboration yet).
- Phase 2 — seeded vulnerable corpus (Spring Boot + Node, both with
  synchronized-safe twins), Oracle Level 2 (body differential) + Level 4
  (post-state probe) + corroboration rule → first CONFIRMED-capable build.
  Remaining invariant types (`uniqueness`, `monotonic_limit`,
  `single_transition`). Minimization (concurrency + workflow).

**Out of scope for this build** (v1+/Phase 3 onward, per ROADMAP/PRD non-goals):
automatic crawler-driven candidate discovery and ranking, HAR/proxy import,
SARIF/CI polish, PortSwigger/OSS-CVE validation runs, multi-endpoint /
state-machine sub-state oracles, a user-defined invariant DSL, WebSocket/HTTP-3
delivery, any weaponization feature (stealth, persistence, exfiltration,
automated destructive exploitation — permanently excluded, see
[`Design/SECURITY.md`](../Design/SECURITY.md) §10).

## 4. Core entities

`Target`, `Scope`, `Endpoint`, `Request`, `Workflow` (Setup phase + Act
Request), `Session`, `Invariant`, `Candidate`, `Experiment`, `Baseline`,
`ConcurrentTrial`, `State Independence`, `Observable`, `Oracle`, `Evidence`,
`Confidence`, `Severity`, `Finding`, `Reproduction` — full definitions in
[`Design/DOMAIN.md`](../Design/DOMAIN.md), which is the canonical vocabulary
for the codebase.

## 5. Core user journeys

See [`docs/USER_FLOWS.md`](USER_FLOWS.md).

## 6. Non-functional requirements (round 0 item 5)

- **Scale**: single local operator, one scan at a time by default (one
  Experiment at a time; configurable). Not designed for fleet/multi-target
  concurrent scanning (ADR-010, out of scope by design).
- **Performance**: HTTP/2 single-packet burst dispersion sub-millisecond on
  loopback (TEST_PLAN Layer 2 assertion); no other latency SLA stated.
- **Availability**: not applicable — local CLI, no service to keep available.
- **Third-party integrations**: none required at runtime (no cloud services,
  no external API dependency). `golang.org/x/net/http2` is a build-time
  library dependency, not a runtime integration.
- Anything not stated above ("don't know yet") stays undefined rather than
  invented — e.g. no numeric SLA exists for scan wall-clock time beyond the
  Scope's `max_wallclock_sec` cap, which is operator-configured per run, not a
  product-wide target.

## 7. Success criteria

Per [`Design/PRD.md`](../Design/PRD.md) §9, measured on the seeded corpus +
PortSwigger labs + known-vuln OSS ([`Design/TEST_PLAN.md`](../Design/TEST_PLAN.md)):

- Detection rate ≥ 90% of seeded limit-overrun races at LIKELY+ confidence.
- False-positive rate on synchronized-safe twins: 0 CONFIRMED, ≤ 5% SUSPECTED.
- Reproduction stability: a CONFIRMED finding re-confirms via `verify` ≥ 90%
  of the time in a fresh environment.
- (PortSwigger labs and OSS-CVE rediscovery are Phase 5 / v1+ success criteria,
  out of scope for this build per §3 above.)

The metric that governs the whole project: **can RaceVeil distinguish a
concurrency-induced integrity violation from ordinary nondeterminism** —
detection rate and false-positive rate together, never one without the other.
