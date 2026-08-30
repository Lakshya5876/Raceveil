# TEST_PLAN.md — RaceVeil

Testing is not an afterthought here; it is the evidence that the central claim
(a trustworthy black/grey-box oracle) is real. Unit tests are necessary but
nowhere near sufficient. The plan is layered, and the layers that matter most are
the **oracle**, **statistical**, and **corpus** layers, because they measure the
one metric that defines success: *can RaceVeil separate a concurrency-induced
integrity violation from ordinary nondeterminism?*

## Layer 1 — Unit tests (pure components)
Scope guard rules, IP pinning/rebinding logic, request/binding resolution,
signature matching, extractors, Wilson interval math, confidence-band mapping,
minimization search, `.rv` (de)serialization, secret redaction. Deterministic,
fast, run on every commit.

## Layer 2 — Protocol tests (HTTP correctness & synchronization)
Against controlled local servers:
- HTTP/1.1 and HTTP/2 request construction, session/cookie/CSRF handling,
  redirect scope enforcement.
- **Synchronization quality**: measure inter-arrival dispersion of the
  `H2SinglePacket` and `H1LastByte` strategies against an echo server that
  timestamps receipt. Assert h2 dispersion is sub-millisecond on loopback and
  record h1 best-effort numbers. This validates the borrowed technique is
  correctly reimplemented (not that we invented it).
- Tightness regression: fail if a change widens the burst window beyond a
  threshold.

## Layer 3 — Oracle tests (classification correctness) — CRITICAL
Feed the oracle **recorded** baseline + concurrent observations (fixtures, no
network) and assert the classification:
- True violation (`S > L`, absent in baseline) → violation.
- Correct behavior (`S ≤ L` under burst) → no violation.
- Non-idempotent-with-no-limit (`S = N` but no invariant established) → **no
  finding** (FP gate).
- Weak classifier separation → capped at SUSPECTED, demands corroboration.
- Post-state probe agreement/disagreement → correct upgrade/hold of confidence.
- **Corroboration matrix** (Level 3 is the primary violation; Level 2/4 are the
  only corroborating observables):
  - Level 3 alone (no Level 2 or 4) → **cannot reach CONFIRMED** (caps at LIKELY).
  - Level 3 + Level 2 (body differential) → **can reach CONFIRMED**.
  - Level 3 + Level 4 (post-state) → **can reach CONFIRMED**.
  - Level 3 + Level 2 + Level 4 → CONFIRMED (stronger, but not required).
  - Missing corroboration entirely → capped below CONFIRMED.
  Assert Level 3 is never counted as its own corroborating observable.
Fixtures are captured from real corpus runs and frozen, so oracle logic is
tested independently of network flakiness. This layer is where oracle
regressions are caught.

## Layer 4 — Statistical tests (does confidence mean anything?)
Simulated and replayed trial streams with known ground truth:
- **False positives**: streams from correct systems must never reach CONFIRMED;
  measure the SUSPECTED/LIKELY leak rate and drive CONFIRMED-FP to 0.
- **False negatives**: low-reproduction real violations (e.g. true rate 15%)
  must still reach at least LIKELY within `K_max`.
- **Nondeterminism / noise**: inject noisy responses, changing response order,
  latency jitter, occasional 5xx; assert the oracle's baseline-variability
  calibration absorbs it without firing.
- **Independence handling (mixed streams)**: only `independent` trials may enter
  the Wilson `K_independent`/`r_violations` (baseline `K_b` is never folded in).
  - 5 independent + 2 dependent → `K_independent=5`, `r_violations` counts
    violations among the 5 only; the 2 dependent trials are retained as Evidence
    but excluded; confidence may be CONFIRMED only if the independent subset
    supports it.
  - 5 independent + 2 unknown → same: `unknown` excluded from
    `K_independent`/`r_violations`.
  - 0 independent (all dependent/unknown) → reproduction undefined; **cannot
    exceed LIKELY** regardless of violation count.
  Assert the implementation never computes `r = all violations, K = all trials`.
- **Threshold tuning**: this layer produces the data to replace ARCHITECTURE.md
  §5's **provisional** (`provisional-v0`) band thresholds with calibrated ones.
  The Wilson methodology is fixed; only the numeric cutoffs are tuned here.
  Report a precision/recall curve over the threshold and select the final defaults
  from measured precision/recall and false-positive behavior — documented, not
  guessed. Bump the recorded band-set version when defaults change.

## Layer 5 — Seeded vulnerable corpus (multi-stack) — CRITICAL
Purpose-built vulnerable apps RaceVeil must catch. At minimum two stacks so the
tool isn't overfit to one framework's response idioms:
- **Spring Boot + PostgreSQL** (leverages the maintainer's strength; also the
  most realistic enterprise target).
- **Node.js (Express) + PostgreSQL** (different ORM/response idioms).

Seeded race archetypes, each as an endpoint/workflow, mapped to the canonical
invariant taxonomy (DOMAIN.md):
- check-then-act coupon redemption, no lock — `max_successes`.
- missing uniqueness constraint, duplicate resource creation — `uniqueness`.
- duplicate redemption of a one-time code — `max_successes`.
- inventory overrun (reserve last unit twice) — `monotonic_limit`.
- rate-limit bypass via concurrency — `max_successes` applied over a time window
  (windowed, not a separate invariant type).
- duplicate state transition ("confirm" fired twice) — `single_transition`.

Each archetype ships in **two isolation levels / lock configs** where relevant
(e.g. `READ COMMITTED` naive vs. `SELECT … FOR UPDATE`) so the corpus covers the
"looks safe, isn't" cases an AI agent would produce.

**Reset fixtures (trial independence).** Each corpus app exposes a
fresh-resource/reset mechanism (issue a new single-use code, create a disposable
account, reset a fixture) so the harness can obtain genuinely fresh state per
trial and mark Experiments `state_independence: independent` (DOMAIN.md). This is
what makes CONFIRMED confidence valid on the corpus. The tests also exercise the
`dependent`/`unknown` paths (no reset available) to assert Confidence is capped at
LIKELY there.

**Proof-cap cases.** Seed limits at multiple `L` and assert
`required_proof_effects` and the safety-ceiling refusal (DOMAIN.md / SECURITY.md):
- `max_successes(L=1)` → proof requires 2 successful effects; RaceVeil stops at 2.
- `max_successes(L=5)` with `proof.max_successful_effects = 2` → **refused as
  unprovable-under-cap** (needs 6; must not exceed the ceiling to force a proof).
- `max_successes(L=5)` with `proof.max_successful_effects ≥ 6` → may prove at 6
  successful effects, then **stops** (does not continue to maximize effect).
- Non-count proof requirements: `uniqueness` proven by two same-key resources;
  `monotonic_limit` by one boundary-crossing effect; `single_transition` by a
  twice-fired transition.

**Windowed `max_successes` (rate-limit) case.** Seed an endpoint that permits
`L` successes per time window (e.g. 5 logins / 60s) and is not concurrency-safe.
Assert RaceVeil: expresses it as windowed `max_successes` (no new invariant type);
arranges the competing Act instances to fall **within one fixed experiment window
(beginning at the first Act Request; sliding-window deferred)**; detects
`> L` in-window successes; and records the window parameters plus inter-arrival
timing as Evidence. Include a correctly-synchronized twin that must stay clean.

**Metric**: detection rate ≥ 90% at LIKELY+, with correct minimization.

## Layer 6 — Synchronized-safe twins — CRITICAL
For **every** seeded-vulnerable app, a byte-for-byte-similar **correctly
synchronized** twin (proper transaction + unique constraint + `FOR UPDATE` /
atomic decrement). RaceVeil must return **clean** on all twins.

**Metric**: **0 CONFIRMED and 0 LIKELY** on twins. Any CONFIRMED here is a
release blocker — it means the oracle can't be trusted. This layer, paired with
Layer 5, is the real proof of the project.

## Layer 7 — PortSwigger benchmark (standardized regression smoke test)
Run against the PortSwigger Web Security Academy race-condition labs
(limit-overrun, rate-limit/multi-endpoint, single-endpoint) as **standardized,
reproducible race test cases**. Treated as a smoke/regression suite, **not** as
proof of general discovery. Passing means "solves these known cases black-box,"
which is a concrete, third-party-reproducible claim — and nothing more.

## Layer 8 — Real open-source vulnerable applications (controlled, local)
Where legal and technical: run against known-vulnerable OSS in local containers.
Anchor case: **nopCommerce < 4.80.0, CVE-2024-58248** (gift-card double-redeem
race). Success = rediscover the violation black-box, no source, with a CONFIRMED
finding and a minimized `.rv`. Add further CVE-class targets as they're vetted.
All testing is local, on disposable state, within scope.

## Layer 9 — End-to-end experiments & metrics
Full `scan` runs across corpus + labs + OSS, recording:
- detection rate, **false-positive rate (esp. CONFIRMED-FP = 0 target)**,
- reproduction rate and `verify` re-confirmation stability across fresh
  environments,
- requests sent, time-to-finding, **race-window sensitivity** (min `N` to
  trigger), minimization effectiveness (start `N` → minimal `N`),
- resource consumption (peak conns, memory), and sync tightness under load.

Results are published as a reproducible benchmark (scripts + Docker compose +
expected outputs) so a skeptic can re-run everything. Reproducibility of the
*benchmark* is a first-class deliverable even though individual races are
probabilistic.

## The metric that governs the project
Not requests sent, not races triggered. The pair **(detection rate on
vulnerable, CONFIRMED-FP rate on safe)**. A tool that catches races but also
flags safe code is worse than useless; the safe-twin layer exists to keep that
honest.

## CI for RaceVeil itself
Layers 1–4 on every PR; Layers 5–7 nightly (containers); Layers 8–9 on release.
The safe-twin (Layer 6) result gates releases.
