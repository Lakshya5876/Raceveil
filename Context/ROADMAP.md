# ROADMAP.md — RaceVeil

**Status: design frozen for Phase 0–2 implementation.** Further doc edits are
justified only by an actual contradiction, an unimplementable requirement, a
demonstrated false assumption, or a missing requirement for existing scope —
surfaced by implementation, testing, or experimental results. Otherwise: build.


The roadmap is organized around **progressively stronger evidence**, not feature
accumulation. Each phase ends with a concrete, externally checkable result. The
project is already worthwhile if it stops after Phase 3–5; later phases are
upside, taken only if justified.

Guiding order of value: a working oracle on a seeded race (Phase 2) > automatic
discovery (Phase 3) > standardized/real-world evidence (Phase 5). Build the thing
that proves the thesis first.

Mapping to PRD scope tiers and phase roles:
- **Phase 0 — foundation.**
- **Phase 1 — MVP vertical slice** (supplied `candidate.yaml`; full pipeline
  end-to-end, but Oracle levels 1 + 3 only, so confidence is **capped at LIKELY**).
- **Phase 2 — MVP validation / thesis proof** (adds Level 2 + Level 4
  corroboration → first **CONFIRMED**-capable build; corpus + safe twins).
- **Phase 3+ — v1 expansion** (automatic discovery and productization).

So the **MVP is Phases 0–2** on a **manually supplied or imported** candidate;
Phase 1 alone is an intentionally incomplete proof pipeline (LIKELY-capped) and
Phase 2 is the first genuinely complete MVP. **Automatic crawler-driven discovery
is Phase 3**, and Phases 3–5 constitute **v1**. Automatic discovery is therefore
never an MVP requirement.

**Frozen implementation order.** Build strictly in this sequence; do not pull
later items forward:
1. **First working path (Phase 1):** `candidate.yaml` → baseline → synchronized
   experiment → oracle (Levels 1 + 3) → repeated trials + state independence →
   Wilson → `.rv` → `replay`/`verify`. Implement **`max_successes` only** first;
   add `uniqueness`/`monotonic_limit`/`single_transition` only after this path
   works end-to-end.
2. **Then minimization** (Phase 2) — only once reproduction is trustworthy.
3. **Then corroboration** Levels 2 + 4 (Phase 2) → CONFIRMED + corpus/safe-twin
   validation.
4. **Then automatic discovery** (Phase 3).
5. **Then SARIF/CI/reporting polish** (Phase 5) — the initial usable artifact is
   correct CLI behavior + human-readable findings + `.rv` + `replay` + `verify`,
   nothing more.

---

## Phase 0 — Foundation
**Build.** Project skeleton (Go, Cobra CLI), request/response model, Session
handling, the **Scope guard** (allowlist, IP pinning, caps) with its test suite,
stdlib HTTP client, deterministic sequential experiment runner, run-directory
persistence, audit log, secret redaction.
**Exit criteria.** Can authenticate, import/capture a handful of requests within
scope (as request-capture infrastructure, **not** automatic candidate discovery),
and run a sequential workflow, writing a clean run directory. Scope guard provably
blocks out-of-scope/rebinding/metadata targets (tests green).
**Evidence.** Scope-guard test suite passes; a sequential run dir is produced.

## Phase 1 — Controlled race experiments
**Build.** The `sync` package: `H2SinglePacket` and `H1LastByte` strategies.
The **`candidate.yaml` manual-candidate input** (Candidate = Workflow +
Invariant) and `scan --candidate`. Baseline collection with classifier
calibration. Oracle Levels 1 + 3, **`max_successes` invariant only** (other types
deferred). Wilson confidence over independent trials
(`K_independent`/`r_violations`) + SUSPECTED/LIKELY bands. Reproduction across
fresh trials. `.rv` finding format. `replay`, `verify`. **No minimization in
Phase 1** (deferred to Phase 2, after reproduction is trustworthy).
**Exit criteria.** Given a hand-written `candidate.yaml` for a single vulnerable
endpoint, RaceVeil establishes the baseline, fires a synchronized burst, detects
`S > L`, reproduces it, and emits a `.rv` that `verify` independently re-confirms
— **with confidence capped at LIKELY until Phase 2 corroboration (Level 2/4) is
available.** Sync tightness measured (sub-ms h2 on loopback). No crawling/
discovery is used.
**Evidence.** A reproduced, `verify`-re-confirmed **LIKELY** finding on a
known-vulnerable local endpoint from a supplied `candidate.yaml`, plus the
synchronization dispersion numbers. (CONFIRMED becomes reachable in Phase 2.)

## Phase 2 — MVP validation / thesis proof (corpus + safe twins)
**Build.** Seeded vulnerable apps in **Spring Boot + PostgreSQL** and
**Node + PostgreSQL** across the race archetypes (check-then-act, uniqueness,
duplicate redemption, inventory overrun, rate-limit bypass, duplicate
transition), each with a **synchronized-safe twin**. Oracle level 2
(body-differential) and level 4 (post-state probe) + corroboration rule — this is
the **first CONFIRMED-capable** build (Level 3 + Level 2/4). Remaining invariant
types (`uniqueness`/`monotonic_limit`/`single_transition`) added here, once the
`max_successes` pipeline works end-to-end. Minimization (concurrency + workflow)
— **built here, after reproduction is trustworthy**, not before.
**Exit criteria.** ≥ 90% detection at LIKELY+ on vulnerable corpus **and 0
CONFIRMED on safe twins**; CONFIRMED reached on corpus cases that have a
corroborating observable. Minimization reports a small stable `N`.
**Evidence.** The corpus benchmark table (detection vs. FP), reproducible via
Docker compose. **This is the result that makes the project real** — it shows the
oracle distinguishes broken from correctly-synchronized code.

## Phase 3 — Automatic discovery
**Build.** Authenticated crawler + OpenAPI/HAR import, workflow extraction
(identity + setup-dependency), two-phase candidate ranking with the
**invariant-inference gate**, automatic experiment generation.
**Exit criteria.** From a URL alone (or URL + imported traffic), RaceVeil finds
the vulnerable workflows in the corpus without being handed the endpoint, and
correctly skips no-invariant endpoints.
**Evidence.** End-to-end `scan <url>` run on the corpus that auto-discovers the
seeded races and skips the safe/non-idempotent ones.

## Phase 4 — Strong verification
**Build.** Statistical hardening from TEST_PLAN Layer 4 (noise/order/jitter
robustness), empirically-tuned confidence bands (precision/recall curve),
dependent-trial capping, richer extractors, stopping criteria, warm-up handling.
**Exit criteria.** Confidence bands are calibrated from data (not guessed);
noise-injection tests don't produce CONFIRMED-FPs; low-reproduction real
violations still reach LIKELY.
**Evidence.** Published precision/recall-vs-threshold analysis; chosen defaults
justified.

## Phase 5 — External validation (the shareable milestone)
**Build.** PortSwigger race-lab runners (regression smoke suite); local
known-vuln OSS targets (anchor: **nopCommerce < 4.80.0, CVE-2024-58248**);
performance measurements; SARIF output + CI mode; docs and a reproducible public
benchmark (scripts + compose + expected outputs).
**Exit criteria.** Solves the limit-overrun/rate-limit PortSwigger labs
autonomously black-box; rediscovers ≥ 1 real CVE-class race in a container with a
minimized `.rv`; benchmark reproducible by a third party.
**Evidence.** "Solves PortSwigger race labs black-box" + "rediscovered
CVE-2024-58248 with no source" + a public, re-runnable benchmark repo. This is
the evidence that earns stars, write-ups, and contributor interest — on merit,
not hype.

## Phase 6 — Expansion (only if earlier phases are excellent)
Candidates, each gated on real demand and on not breaking the black-box
trust model:
- Multi-endpoint / partial-construction / state-machine sub-state oracles.
- A user-defined invariant DSL (generalizing `invariants.yaml`).
- WebSocket delivery; HTTP/3.
- Deeper traffic import (proxy integration), richer post-state probing.
- CI ecosystem integrations beyond SARIF.
**Rule.** Do not start Phase 6 to add surface area. Start an item only when a
concrete user/finding demands it and it can be done without weakening the
oracle's trustworthiness or the safety model.

---

## Evidence ladder (how social proof accrues, honestly)
1. Detects seeded races (Phase 2). 2. Doesn't flag safe twins (Phase 2).
3. Solves PortSwigger labs (Phase 5). 4. Rediscovers OSS CVE locally (Phase 5).
5. Reproducible minimized findings (Phase 2+). 6. Independent users reproduce the
benchmark (Phase 5). 7. Real authorized bug-bounty findings (post-v1). 8. External
contributors/issues. 9. Security researchers discuss the *methodology* (the
oracle), which is the durable, interesting part.

No projected star counts. The architecture is not optimized for GitHub metrics;
it's optimized for evidence that happens to be worth sharing.

## Definition of done for v1
Phases 0–5 complete: automatic black/grey-box discovery + trustworthy oracle +
reproduction/minimization + safe-twin-clean + PortSwigger-green + one real OSS CVE
rediscovered + reproducible benchmark + CLI/CI/SARIF + the SECURITY model
enforced and tested. That artifact stands on its own, with or without the
AI-era framing, and is defensible in a senior SWE/security interview.
