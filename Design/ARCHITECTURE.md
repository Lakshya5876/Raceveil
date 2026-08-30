# ARCHITECTURE.md — RaceVeil

Terminology is defined in DOMAIN.md and used verbatim here.

## 0. Shape of the system

RaceVeil is **one local CLI executable**. No server, no control plane, no
microservices, no orchestration, no database service. A scan is a pipeline of
in-process stages that reads a Scope + Session and writes a self-contained **run
directory** to disk. This is a deliberate architectural principle
(DECISIONS.md): simple local architecture over infrastructure theater.

```
Target Web App
   │
   ▼
Scope / Authorization ───────────────► (gate on every request, always on)
   │
   ▼
Crawler / Traffic Ingestion
   │
   ▼
Workflow Discovery
   │
   ▼
Candidate Ranking ──────────────────► (Candidate = Workflow + Invariant + score)
   │
   ▼
Experiment Scheduler
   ├─► Baseline Experiment  (sequential control)
   ├─► Concurrent Experiment (synchronized burst)
   ▼
Observable-State Oracle  (levels 1–5)
   │
   ▼
Statistical Verification (Wilson interval, confidence bands)
   │
   ▼
Minimization
   │
   ▼
Finding ──► Reproduction Artifact (.rv)
```

## 1. Locked technology decisions

(Rationale and rejected alternatives in DECISIONS.md.)

| Concern | Decision |
|---|---|
| Language / runtime | **Go** — single static binary, goroutine concurrency for the scheduler, direct HTTP/2 framer access via `golang.org/x/net/http2` |
| Distribution | Single local executable; cross-compiled binaries |
| Interface | **CLI only** for MVP/v1 (Cobra). No GUI, no daemon |
| License | **Apache-2.0**. Do **not** take a runtime dependency on GPL-3 `h2spacex`; reimplement single-packet delivery from the documented technique |
| HTTP client (crawl/baseline) | Go stdlib `net/http` (HTTP/1.1 + h2) |
| Synchronized concurrency engine | **Custom**, over `x/net/http2` `Framer` on a raw TLS conn for HTTP/2 single-packet; raw `crypto/tls` conn for HTTP/1.1 last-byte |
| HTTP/1.1 | Supported (last-byte sync, best-effort) |
| HTTP/2 | Supported (single-packet / last-frame sync) |
| HTTP/3 | **Deferred** |
| Persistence | Filesystem run directory: `config.yaml`, `requests.jsonl`, `experiments/`, `findings/*.rv`. **No database** |
| Config | YAML for scope/auth/invariants; flags override |
| Concurrency model | Bounded worker pool of goroutines; one Experiment at a time by default (avoids cross-experiment interference), configurable |
| Logging | Structured (JSON) audit log of every request; human log to stderr |
| Cancellation | `context.Context` throughout; Ctrl-C drains safely and still writes partial run dir |

## 2. Components

### 2.1 Scope / Authorization guard
A single chokepoint every outbound request passes through (`scope.Guard`).
Enforces: host/port/path allowlist (default-deny), pinned resolved IP per host
(anti-DNS-rebinding), private/link-local/metadata-IP blocking unless the target
itself is explicitly a localhost/private authorized target, redirect-target
re-validation, per-scope concurrency/rate/total-request caps, and
destructive-verb rules. See SECURITY.md — this component is safety-critical and
is covered by its own test suite.

### 2.2 Crawler / Traffic Ingestion (Phase 3 / v1)
Two ways to get Requests (both **v1**, not used on the MVP path, where the
Candidate is supplied via `candidate.yaml`):
1. **Crawler** (Phase 3 / v1): authenticated crawl within Scope, collecting
   forms, links, and (best-effort) XHR/fetch endpoints from inline JS and any
   OpenAPI/Swagger doc if exposed. No headless browser in v1 (DECISIONS.md); a
   lightweight HTML+JS-heuristic crawler plus optional OpenAPI import.
2. **Import** (Phase 3 / v1): load Requests from a HAR file or a captured proxy
   log, so a human can drive the app once and hand RaceVeil the traffic. This
   sidesteps crawler blind spots for SPAs and is the pragmatic path for hard
   targets.

Output: a set of Endpoints and captured Requests with detected variable bindings
(session tokens, CSRF, IDs).

### 2.3 Workflow Discovery (Phase 3 / v1)
Groups captured Requests into Workflows. **On the MVP path this stage is not
run — the Workflow is already supplied inside `candidate.yaml`.** In Phase 3 / v1:
mostly identity (one Request = one Workflow) plus a **setup-dependency**
heuristic — if executing the candidate Request alone returns a precondition error
(empty cart, no code applied), search captured traffic for the predecessor
Requests that satisfy it and attach them as the Workflow prefix. Data
dependencies (an ID returned by request A used by B) are tracked as variable
bindings.

### 2.4 Candidate Ranking (Phase 3 / v1 only)
Turns Workflows into ranked Candidates. **Not part of the MVP** (a supplied
Candidate has no score; see DATA_MODEL.md `source: declared`). **Two-phase**, so
in discovery we prioritize experiments instead of hammering everything:

- **Phase A — static signals (cheap, no traffic):** mutation verb
  (POST/PUT/PATCH/DELETE); path/param/field name semantics
  (`redeem`, `coupon`, `voucher`, `apply`, `claim`, `reserve`, `checkout`,
  `vote`, `transfer`, `withdraw`, `balance`, `quantity`, `limit`, `once`);
  absence of an idempotency key; presence of a resource identifier reused across
  requests. Produces a prior score.
- **Phase B — invariant probe (cheap traffic):** for top-ranked Workflows, run a
  tiny **sequential** probe (2–3 executions) to detect a limit: does execution
  #2 get rejected ("already applied", 409, quantity refused)? If a rejection
  signal appears, an Invariant (`max_successes`/`uniqueness`) is **inferred** and
  the Candidate is promoted. **If no limit is observed and none is user-declared,
  the Candidate is dropped** — this is the FP gate from DOMAIN.md.

User-declared Invariants (level-5, via `invariants.yaml`) enter here directly and
always outrank inferred ones.

### 2.5 Experiment Scheduler
Owns the lifecycle of each Experiment: acquire fresh Session/state → Baseline →
**Setup phase (once) → Act phase (N synchronized Act instances)** → Oracle →
Verification → Minimization → emit. Per trial it runs the Workflow's Setup phase
once, binds its outputs into the Act Request, then hands `N` prepared Act
instances to the concurrency engine; it does **not** replicate setup as part of
the burst (DOMAIN.md Act Request). Between reproduction trials it invokes the
executable Reset Recipe to obtain fresh state (marking the trial `independent`) or
records `dependent`/`unknown`. Enforces caps, handles warm-up, sequences
experiments to avoid interference, supports cancellation, and records everything
to the run dir.

### 2.6 Concurrency Engine (`sync` package)
Prior-art infrastructure, pluggable behind one interface. The `reqs` passed to
`Burst` are the `N` prepared **Act Request** instances for one trial (setup has
already run):

```go
type SyncStrategy interface {
    // Release N prepared Act-request instances' final bytes/frames
    // as simultaneously as possible; return per-instance responses.
    Burst(ctx context.Context, reqs []PreparedRequest) ([]Response, error)
    Name() string
}
```

Implementations:
- `H2SinglePacket` — open one h2 connection, send all instances' HEADERS (and
  DATA up to the final frame) withholding the last frame, then flush all final
  frames back-to-back so they coalesce into one TCP segment (`TCP_NODELAY`).
  Reimplemented from the published last-frame-synchronization technique.
- `H1LastByte` — one TCP+TLS connection per instance, write each request up to
  its final byte, then flush all final bytes together. Best-effort; jitter is
  higher than h2, documented as such.

The engine measures and records inter-arrival dispersion so the Oracle knows how
tight the burst actually was.

### 2.7 Observable-State Oracle
The core of the project — its own section below (§4).

### 2.8 Statistical Verification
Turns per-trial violation observations into a Confidence band (§5).

### 2.9 Minimization
Reduces a confirmed violation to its smallest reproducible form (§6).

### 2.10 Finding + Reproduction writer
Serializes Evidence, Confidence, Severity, and a replayable `.rv` bundle
(DATA_MODEL.md).

## 3. What is in the MVP

In: Scope guard, **`candidate.yaml` input** (a supplied Candidate = Workflow +
Invariant; DATA_MODEL.md §Candidate input), Session/auth handling, scheduler,
both SyncStrategies, Baseline calibration, Oracle **Levels 1 + 3** (Phase 1;
Phase 2 adds Levels 2 + 4) plus **Level 5** user-declared config, Wilson-based
verification, minimization, findings + `.rv`,
`scan --candidate/replay/verify/report`.

**Not in the MVP:** automatic candidate discovery and ranking. The crawler /
import (§2.2) exists only as request-capture infrastructure for later phases and
is **not** required to run the MVP; on the MVP path the operator supplies the
Candidate directly. Automatic crawler-driven discovery, Workflow extraction, and
two-phase ranking with invariant inference are **v1 / Phase 3**.

Deferred to v1+: automatic discovery + ranking, full crawler/OpenAPI, HAR import
polish, richer extractors, SARIF, multi-request workflow minimization, broader
invariant types.

## 4. Oracle design (the central contribution)

The Oracle answers: *did concurrency cause an observable Invariant violation that
does not occur sequentially?* **Levels 1–4 form the evidence-strength hierarchy**;
a Candidate is evaluated at the highest level its Observables support, and higher
levels supply the corroboration that upgrades Confidence. **Level 5 is
configuration, not an evidence tier** — a user-declared invariant/classifier that
*feeds* Levels 1–4, never a source of corroboration on its own. **Never** just
compares status codes.

**Baseline calibration (prerequisite for all levels).** From the sequential
Baseline the Oracle learns a classifier `c(response) → {success, reject, error}`:
- positive examples: the successful execution(s) (≤ L of them);
- negative examples: the limit-enforced rejection(s).
It also records the natural variability of every extracted Observable. If success
and reject are not cleanly separable on the Baseline, the Oracle caps achievable
Confidence at SUSPECTED and requires either a user-declared classifier (Level 5)
or a Level-4 post-state probe (available Phase 2) to proceed.

- **Level 1 — direct response invariant.** Count `S = #{concurrent responses
  classified success}`. Violation iff `S > L`. Cheapest, weakest alone.
- **Level 2 — response-body differential.** Compare extracted body fields
  (balance, quantity, remaining-uses, ids) between the sequential Baseline and
  the concurrent burst. E.g. balance decremented once sequentially but the burst
  yields multiple "success + new balance" bodies inconsistent with a single
  decrement. Detects violations even when status codes are all 200.
- **Level 3 — cross-request consistency.** The logical check: `N` competing
  workflow instances should collectively produce ≤ `L` successful *effects*;
  observing `> L` distinct successful effects (distinct order ids, distinct
  redemption receipts) is the violation. This is the primary limit-overrun
  oracle.
- **Level 4 — observable post-state (authorized read-only; Phase 2).** Issue a
  read-only **state probe** after the burst (`GET /account/redemptions`,
  `GET /cart`, `GET /orders`) and check the persisted state against the Invariant
  (persisted redemption count is 2, exceeding the permitted 1). Strongest single
  Observable; requires a probe Request (user-supplied on the MVP path) and
  read-only safety classification.
- **Level 5 — user-declared configuration (not an evidence tier).** The tester
  declares the invariant and, optionally, the classifiers/probe explicitly (in
  `candidate.yaml` for MVP, or `invariants.yaml` during v1 discovery). It
  configures what Levels 1–4 evaluate and is always authoritative over inference,
  but it never counts as corroboration by itself.

**Confidence corroboration rule:** CONFIRMED requires the level-3 violation
**plus at least one independent corroborating Observable** (level-2 differential
or level-4 post-state). A single-signal violation caps at LIKELY. This is a
deliberate defense against classifier error.

**The false-positive gate, restated at the architecture level:** the Oracle may
only assert a violation where an Invariant was established (inferred limit or
user-declared). A non-idempotent endpoint with no limit produces no Invariant,
so `S = N` successes is *expected*, not a finding. Correctly-synchronized code
enforces the limit even under the burst, so `S ≤ L`, so no finding. Both safe
cases fall out of the same rule.

## 5. Experiment & statistical methodology

Every Experiment is a controlled experiment:

```
Hypothesis:   Invariant I holds for Workflow W  (H0: no concurrency-induced violation)
Control:      Baseline — sequential runs; confirm I holds, calibrate classifier, measure noise
Intervention: ConcurrentTrial — synchronized burst of N instances
Observation:  S (success count) + Observables per trial
Comparison:   violation iff S breaks I *and* Baseline never broke I
Replication:  repeat K trials on fresh, independent state
Conclusion:   Confidence band from reproduction statistics + corroboration
```

**Why not a t-test / arbitrary threshold.** A single trial's violation is a
*logical* fact, not a statistical estimate: under correct sequential semantics
`S ≤ L` is impossible to exceed, so any trial with `S > L` is a counterexample.
What is uncertain is (a) whether the classifier mislabeled, handled by
corroboration, and (b) whether the violation *reproduces* or was a one-off of a
specific server state. (b) is a **binomial** question: `r` violations in `K`
independent trials.

**State Independence (a first-class, recorded property).** Every Experiment
records a `state_independence` value — `independent`, `dependent`, or `unknown`
(defined in DOMAIN.md) — and, when independent, the **Reset Recipe** used to
obtain fresh state per trial (new single-use code, disposable account, fixture
reset). Only `independent` trials are Bernoulli-independent and may support
CONFIRMED. `dependent` (state cannot be reset, e.g. one real coupon on an
external target) and `unknown` are usable as Evidence but **cap Confidence at
LIKELY**, never CONFIRMED, because the reproduction statistics below are not
valid otherwise. The scheduler establishes independence via the Reset Recipe
before counting trials; absent a recipe, it records `unknown`. This is a minimal
descriptor + reset hook, not a state-management subsystem. The value and recipe
are carried into the `.rv` so an independent re-run can reproduce the same state
assumptions.

**Confidence from the Wilson score interval.** Compute the 95% Wilson interval
for `p = r_violations / K_independent` (chosen over the normal approximation
because the sample is small and `p` is often near 0 or 1, where the normal
approximation misbehaves), where **`K_independent` is the count of `independent`
reproduction trials only and `r_violations` the violations among them.**
`dependent`/`unknown` trials are retained as Evidence but **never enter
`K_independent` or `r_violations`**, and the baseline count `K_b` is never folded
in — an implementation must not compute `r = all violations, K = all trials`. If
there are no independent trials, `p` is undefined and Confidence cannot exceed
LIKELY. Wilson bounds are always computed by the actual Wilson formula in code —
never copied from the illustrative values in these docs. Let `lo` be the lower
bound.

Two things must be kept separate here: **(A) the methodology**, which is fixed —
per-trial violation is a logical counterexample, reproduction is a binomial
question, Confidence comes from the Wilson lower bound over **independent** trials
plus corroboration, mapped to SUSPECTED/LIKELY/CONFIRMED — and **(B) the numeric
thresholds**, which are **provisional defaults, not validated constants.** The
values below are an initial `provisional-v0` band set; the final cutoffs are
**selected empirically** from the measured precision/recall and false-positive
behavior on the seeded vulnerable corpus and synchronized-safe twins
(TEST_PLAN.md Layer 4). Runs record the band-set version so no output implies the
current numbers are settled.

Provisional (`provisional-v0`) bands, configurable:
- **CONFIRMED** — `r_violations ≥ 2`, Wilson `lo ≥ 0.10`, and ≥ 1 corroborating
  Observable, on **independent** trials. (Provisional rationale: even a
  ~10%-reliable duplicate-effect bug is a real violation; we want statistical
  confidence that the true reproduction rate is *nonzero and repeatable*, not that
  it is high. The `0.10` bound is a starting point to be calibrated, not a claim.)
- **LIKELY** — `r_violations ≥ 1` but corroboration missing, or trials
  dependent/unknown, or Wilson `lo` below the CONFIRMED bar.
- **SUSPECTED** — a signal consistent with a violation, but classifier
  separation weak or Invariant only heuristically inferred (not Baseline-proven).

**Stopping criteria.** Run up to `K_max` independent trials; stop early when the
band can no longer change (e.g. enough violations to CONFIRM, or enough clean
trials that `lo` can't reach the bar). Warm-up trials (discarded) prime
connection/JIT/cache so first-trial cold effects aren't mistaken for signal.

**Concurrency-level sweep.** If the initial `N` yields no violation, escalate `N`
across a small ladder (e.g. 2, 5, 10, 20, up to the Scope cap) before concluding
"no violation," because race windows have a minimum `N` to line up. Record the
`N` at which violations first appear.

**Proof requirement vs. safety ceiling.** The scheduler computes an Invariant's
`required_proof_effects` (DOMAIN.md: `L + 1` for `max_successes(L)`; the
type-specific minimum for the others) and treats
`proof.max_successful_effects` (Scope, SECURITY.md) as an upper bound on the
successful **state-changing effects RaceVeil may cause** (read-only post-state
probes do not consume this budget). It stops as soon as enough successful effects
exist to establish the violation, and never continues to maximize impact. If
`required_proof_effects > proof.max_successful_effects`
(e.g. `max_successes(L=5)` needs 6 but the ceiling is 2), the Experiment is
**refused as unprovable-under-cap** — RaceVeil never exceeds the ceiling to force
a proof.

**Windowed `max_successes` experiments.** When the Invariant carries a temporal
`window` (DOMAIN.md), the burst must be arranged so the competing Act instances
land within **one fixed experiment window beginning at the first Act Request**
(sliding-window semantics are deferred). The single-packet burst already collapses
arrival to sub-window timing; for larger windows the scheduler simply issues the
`required_proof_effects` competing Act requests inside that window. The Baseline
establishes the per-window limit `L`, and the Finding records the window
parameters and the observed inter-arrival timing as Evidence that the successes
were genuinely within-window.

## 6. Minimization

Once CONFIRMED/LIKELY at some `N`:

- **Concurrency minimization.** *Not* binary bisection — reproduction is
  probabilistic, so there is no crisp monotone threshold to bisect. Instead do a
  **decreasing sweep with statistical re-verification**: step `N` down, run
  `K_min` fresh trials at each step, keep the smallest `N` whose Wilson `lo`
  still meets the LIKELY bar. Report that `N` with its measured reproduction rate.
- **Workflow/request-set minimization.** For multi-request Workflows, greedily
  drop prefix/parallel requests not required to still trigger the violation
  (delta-debugging style), re-verifying after each removal.

Output: *the smallest reproducible concurrent interaction that violates the
invariant* — the most useful artifact for a human fixing the bug.

## 7. Run directory (persistence)

```
run-<timestamp>/
  config.yaml            # resolved scope + params (secrets redacted)
  audit.jsonl            # every request sent, for accountability
  requests.jsonl         # captured/imported requests
  candidates.jsonl       # ranked candidates + inferred invariants
  experiments/
    <candidate-id>/
      baseline.json
      trials.jsonl
      oracle.json
      minimization.json
  findings/
    <finding-id>.rv      # self-contained reproduction bundle (JSON)
  report.json | report.sarif | report.md
```

## 8. Cross-cutting

- **Resource limits:** global caps on total requests, concurrency, rate,
  wall-clock; enforced in the Scope guard and scheduler.
- **Determinism where possible:** fixed RNG seed (recorded) for ordering/jitter
  choices so a run is re-playable; the race outcome itself stays probabilistic
  but the harness is reproducible.
- **Observability:** progress UI (UX.md) reads scheduler state; the JSON audit
  log doubles as a debugging trace.
