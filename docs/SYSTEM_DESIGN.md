# SYSTEM_DESIGN.md — RaceVeil (Governance Spec)

High-level component breakdown, and the explicit mapping from
[`Design/ARCHITECTURE.md`](../Design/ARCHITECTURE.md)'s components onto the
four-layer directory scaffold Phase B of `/init-governance` creates. This
document and that scaffold must agree; if they ever diverge, this file is
wrong and gets fixed via PR, not silently reinterpreted.

## 1. Shape of the system

One local CLI binary (`raceveil`, Go/Cobra). No server, no control plane, no
database. A scan is a pipeline of in-process stages that reads a Scope +
Session and writes a self-contained run directory to disk (ADR-010, ADR-011).

## 2. Component → layer mapping

| ARCHITECTURE.md component | Layer | Directory |
|---|---|---|
| Scope / Authorization guard (§2.1) | Security-critical, cross-cutting — invoked from Infrastructure at the one chokepoint every outbound request passes through | `internal/security/scope/` (CORE_FILES) |
| Crawler / Traffic Ingestion (§2.2, v1/Phase 3 — not built in this MVP) | Infrastructure (I/O: HTTP fetch, HTML/JS parsing) | `internal/infrastructure/crawl/` (stub only until Phase 3) |
| Workflow Discovery (§2.3, v1/Phase 3) | Application (orchestration/heuristics over captured data) | `internal/application/discovery/` (stub only until Phase 3) |
| Candidate Ranking (§2.4, v1/Phase 3) | Application | `internal/application/ranking/` (stub only until Phase 3) |
| Experiment Scheduler (§2.5) | Application — owns the Baseline→Setup→Act→Oracle→Verify→Minimize lifecycle | `internal/application/scheduler/` |
| Concurrency Engine / `sync` package (§2.6) | Infrastructure — raw socket/TLS I/O, HTTP/2 framer access | `internal/infrastructure/sync/` |
| Observable-State Oracle (§2.7) | Application — pure evaluation logic over domain Observables, no I/O of its own (fed by the Scheduler) | `internal/application/oracle/` |
| Statistical Verification (§2.8) | Application — Wilson interval computation | `internal/application/oracle/` (co-located with Oracle; same evidentiary concern) |
| Minimization (§2.9) | Application | `internal/application/minimize/` |
| Finding + Reproduction writer (§2.10) | Infrastructure — filesystem serialization | `internal/infrastructure/persist/` |
| CLI commands (`scope`, `auth`, `scan`, `verify`, `replay`, `report`, `list`) | Presentation | `internal/presentation/cli/`, thin entrypoint at `cmd/raceveil/main.go` |
| Domain entities (Target, Scope, Workflow, Candidate, Experiment, Invariant, Confidence, Finding, ...) | Domain — zero deps, pure types | `internal/domain/` |
| Config/flag resolution | Cross-cutting, single module (security invariant) | `internal/config/` (CORE_FILES) |
| CLI/scheduler/oracle/guard assembly | Wiring | `internal/wiring/` (CORE_FILES) |

Dependency direction matches Guide §2.2: Presentation → Application →
Domain ← Infrastructure. The Scope guard is the one deliberate exception to
"infrastructure never contains business logic" — it **is** infrastructure
(dials real sockets) but encodes a business-critical safety rule, which is
exactly why SECURITY.md and this doc both call it out as CORE_FILES rather
than ordinary infra code.

## 3. Third-party integrations (Round 0 item 5)

None at runtime. Build-time library dependencies only:
`golang.org/x/net/http2` (HTTP/2 Framer access for single-packet delivery,
ADR-009), the Cobra CLI framework, and a YAML parser for
`scope.yaml`/`candidate.yaml`/`invariants.yaml`. No cloud service, external
API, or SaaS dependency exists or is planned for the MVP.

## 4. Run directory (persistence boundary)

```
run-<timestamp>/
  config.yaml            # resolved scope + params, secrets redacted
  audit.jsonl            # every outbound request, for accountability
  requests.jsonl         # captured/imported requests (v1/Phase 3 path)
  candidates.jsonl        # candidates (declared, MVP; or inferred, v1)
  experiments/<candidate-id>/{baseline.json,trials.jsonl,oracle.json,minimization.json}
  findings/<finding-id>.rv
  report.{json,sarif,md}
```

Full field-level shapes: [`Design/DATA_MODEL.md`](../Design/DATA_MODEL.md).

## 5. What this build does NOT stand up

Per `docs/PRD.md` §3, the Crawler, Workflow Discovery, and Candidate Ranking
components exist in the directory scaffold as named packages (so the layering
is right from day one) but carry no real implementation until Phase 3 — Phase
0–2 only needs the `candidate.yaml` manual-candidate path. Building them out
early would be scope creep the Guide §2.5.6a Minimum Footprint rule forbids.
