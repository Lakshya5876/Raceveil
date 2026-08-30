# DOMAIN.md — RaceVeil Domain Model

This document fixes the vocabulary. Every other design document and the codebase
use these terms with exactly these meanings. If a concept is not here, it does
not exist in RaceVeil.

RaceVeil is a **black/grey-box concurrency verification tool**. It observes a
running web application from the outside and tries to *prove* that a specific
expected invariant is broken when requests are issued concurrently. It does not
read source, instrument the runtime, or read the database. That boundary is the
whole point of the project, so the domain model is built entirely from
externally observable concepts.

---

## 1. The three behaviors (the spine of the whole tool)

These three definitions are the reason RaceVeil can make a trustworthy claim.
Everything else is machinery in service of separating them.

- **Expected behavior** — what *should* happen under valid, purely sequential
  execution of a workflow, given an invariant. Example: a one-time coupon can be
  redeemed **at most once**; a second sequential redemption is rejected.

- **Observed behavior** — what *actually* happened during an experiment, measured
  only through Observables (responses and, optionally, post-state probes).

- **Concurrency-induced violation** — an Observed behavior that (a) breaks the
  Invariant and (b) does **not** occur under the sequential Baseline, appearing
  **only** when the workflow is executed concurrently. The "only under
  concurrency" clause is mandatory. A violation that also shows up sequentially
  is an ordinary logic bug, not RaceVeil's concern, and RaceVeil must not report
  it as a concurrency finding.

A RaceVeil Finding is, precisely, a reproduced concurrency-induced violation.

---

## 2. Core entities

### Target
The authorized application under test. Identified by a base URL and the set of
hosts/IPs it resolves to. A Target is only testable if it is covered by a Scope.

### Scope
The explicit authorization boundary. An allowlist of hosts/ports/path-prefixes
RaceVeil is permitted to touch, plus caps (max concurrency, max requests,
request rate) and destructive-action rules. Scope is **default-deny**: anything
not listed is out of bounds. Scope is a first-class safety object, not a
convenience filter — see SECURITY.md.

### Endpoint
A single addressable operation: method + path template (+ significant query/body
shape). E.g. `POST /coupon/redeem`. Endpoints are the atoms discovery works on.

### Request
A concrete HTTP message: method, URL, headers, body, and the Session it belongs
to. Requests are captured (from crawl or imported traffic) and are the unit the
concurrency engine sends. A Request may contain **variable bindings** (tokens,
CSRF values, IDs) resolved at send time.

### Workflow
An **ordered sequence of Requests**, with variable bindings flowing between them,
that together perform one logical state-changing operation. Most limit-overrun
Workflows are a single Request (`POST /coupon/redeem`), but some require setup
(`POST /cart` → `POST /cart/coupon` → `POST /checkout`). The Workflow is what
gets executed sequentially in the Baseline; in a ConcurrentTrial it splits into a
**Setup phase** and an **Act phase** (below).

### Act Request (and the Setup / Act phases)
Every Workflow designates exactly one **Act Request** (`act_request`): the request
whose execution is **replicated concurrently** to create the race. The requests
before it form the **Setup phase**.

- **Setup phase** — executed to establish the precondition the Act Request needs.
  By default setup runs **once** per trial and its result (cart id, token) is
  bound into the Act Request; it is **not** replicated as part of the burst. (A
  Workflow may declare a setup request as *per-instance* when the race genuinely
  requires N distinct preconditions; this must be explicit, and MVP does not do
  it automatically.)
- **Act phase** — `N` instances of the Act Request are prepared and released as a
  synchronized burst (single-packet / last-byte). Only the Act Request is
  duplicated.

This makes the unit of duplication unambiguous: *replicate the Act Request N
times over one shared setup*, not `N × (setup + act)` (which would create N carts
and change the experiment). When setup and act have a data dependency, the shared
setup output is bound into every Act instance. Modelling a setup request as
per-instance is a Phase-6 concern (multi-request minimization), not MVP.

### Session
The authentication/identity context a Workflow runs under: cookies, bearer
tokens, CSRF tokens, and the logic to refresh them. RaceVeil supports multiple
Sessions (e.g. attacker account vs. victim account) but keeps them isolated;
credentials never cross Session boundaries (SECURITY.md).

---

## 3. Experiment entities

### Invariant
A predicate expected to hold over the Observed behavior of a Workflow. RaceVeil
only makes a Finding where an Invariant is either **inferred** from the Baseline
or **declared** by the user. Supported Invariant types in v1:

- `max_successes(L)` — at most `L` executions of the workflow may succeed
  (default `L = 1`: one-time actions, single-use codes). Optionally carries a
  **temporal window** `window(duration)`: with no window, the bound is
  lifetime/workflow-scoped; with a window, it means *at most `L` successful
  effects within one window of that duration*. Rate-limit bypass is this
  windowed form (e.g. `L = 5`, `window = 60s`) — not a separate invariant type.
  **MVP/v1 define the window as a fixed experiment window beginning at the first
  Act Request of the burst; sliding-window semantics are deferred** (a rate
  limiter may implement either; RaceVeil does not silently assume sliding). For a
  windowed invariant, the concurrency Experiment must establish that the competing
  Act instances land within that one fixed window, and the Finding records the
  window parameters and the observed inter-arrival timing.
- `uniqueness(key)` — at most one resource with a given natural key may be
  created.
- `monotonic_limit(resource, cap)` — a consumable (inventory, balance, quota)
  must not be drawn below/above a bound.
- `single_transition(state)` — a state transition may fire at most once.

These four are the **canonical invariant taxonomy**; no other document introduces
additional top-level invariant types. Related race classes are expressed within
this set: **rate-limit bypass** is a temporal/windowed application of
`max_successes` (at most `L` successes per time window); **inventory overrun** is
`monotonic_limit`; **duplicate state transition** is `single_transition`;
**duplicate resource creation** is `uniqueness`.

An Invariant is **not assumed**. If the Baseline cannot establish that a limit
exists (i.e. sequential repeats are all accepted with no rejection signal) and
the user has not declared one, there is **no Invariant**, therefore **no
Candidate and no possible Finding**. This gate is the primary false-positive
defense (a non-idempotent `POST /comment` legitimately produces N results and
must never be flagged).

**Proof requirement (per Invariant).** The *minimum observable evidence needed to
establish a violation* depends on the Invariant type, and is separate from the
operator's safety ceiling `proof.max_successful_effects` (SECURITY.md):
- `max_successes(L)` → `required_proof_effects = L + 1` (one more successful
  effect than permitted proves `S > L`; for the default `L = 1` this is 2). For a
  windowed `max_successes`, the `L + 1` effects must fall within one window.
- `uniqueness(key)` → two persisted resources sharing the same natural key.
- `monotonic_limit(resource, cap)` → one observed effect that crosses the bound
  (e.g. the `cap`-th+1 reservation succeeding, or a consumable driven past its
  limit).
- `single_transition(state)` → the guarded transition observed to succeed twice.

RaceVeil performs the **minimum** effects necessary to establish the violation
and then stops; it never continues to maximize impact. If
`proof.max_successful_effects` is below an Invariant's `required_proof_effects`
(e.g. cap 2 for `max_successes(L=5)`, which needs 6), RaceVeil **cannot prove**
that Invariant within the safety ceiling and reports it as unprovable-under-cap
rather than exceeding the ceiling.

### Candidate
A `(Workflow, Invariant)` pair that RaceVeil hypothesizes may exhibit a
concurrency-induced violation. Every Candidate has a **`source`**:
- **declared** — supplied directly by the operator via `candidate.yaml` (the MVP
  path). `score` is `null` (no ranking applies).
- **inferred** — produced by v1 automatic discovery, which attaches a ranking
  `score`.

Both origins normalize to the **same** Candidate representation and are recorded
in `candidates.jsonl` (DATA_MODEL.md), so the rest of the pipeline never sees two
kinds of Candidate. The scheduler turns Candidates into Experiments (discovery
additionally uses `score` to prioritize them).

### Experiment
The full test of one Candidate: `Baseline → Concurrent phase → Oracle evaluation
→ Verification → (Minimization)`. Each Experiment is self-contained and produces
either "no violation" or a Finding. An Experiment is literally structured as a
controlled experiment: hypothesis (the Invariant), control (Baseline),
intervention (concurrency), observation, comparison, replication, conclusion.

### Baseline
The sequential control phase (**calibration/control data — not part of the
reproduction sample**). Executes the Workflow one instance at a time, `K_b` times,
on fresh state where possible, to establish:
1. **Expected success count** under the Invariant (usually `L`).
2. **Success signature** — the response pattern of a successful execution.
3. **Rejection signature** — the response pattern when the limit is enforced.
4. **Natural variability** — the ordinary, concurrency-free variation in
   Observables (so we don't mistake noise for a violation later).

`K_b` (baseline run count) is distinct from `K_independent` (the reproduction
sample) below; baseline runs never enter the Wilson statistics. If Baseline
cannot separate success from rejection cleanly, the Candidate is downgraded (it
cannot support a CONFIRMED Finding) — see Oracle.

### ConcurrentTrial
One synchronized burst: `N` instances of the **Act Request** released as close to
simultaneously as the concurrency engine allows (single-packet for HTTP/2,
last-byte sync for HTTP/1.1), over one shared Setup phase (see Workflow / Act
Request), plus the collected Observables. A trial yields a **success count `S`**
(via the Baseline-derived success classifier). A trial is a **violation
observation** iff `S` breaks the Invariant (e.g. `S > L`). Each trial records its
own state-reset status, from which the Experiment's aggregate State Independence
is derived.

### State Independence
Whether the trials of an Experiment can be treated as **statistically
independent**, which the reproduction statistics (Wilson interval) require. Each
Experiment carries one of three values:

- **independent** — each trial runs on genuinely fresh/resettable state (a new
  single-use code, a disposable account, a **reset fixture**), so trials are
  Bernoulli-independent. Only this value permits CONFIRMED Confidence.
- **dependent** — state cannot be reset between trials (e.g. one real coupon on
  an external target), so trials are not independent. Usable as Evidence but
  Confidence is **capped at LIKELY**, never CONFIRMED.
- **unknown** — independence has not been verified. Treated as `dependent` for
  Confidence purposes (capped at LIKELY) until established.

**Trial independence is recorded per ConcurrentTrial**, and the Experiment's
aggregate `state_independence` is derived from them. **Only trials marked
`independent` enter the Wilson reproduction calculation** — they alone contribute
to **`K_independent`** (the reproduction sample count) and **`r_violations`**
(violations within it). `dependent` and `unknown` trials are retained as Evidence
but are **excluded from `K_independent`/`r_violations`**; an implementation must
not compute `r = all violations, K = all trials`, and must not fold the baseline
count `K_b` into `K_independent`. If an Experiment has no independent trials, its
reproduction statistics are undefined and it **cannot reach CONFIRMED** (capped at
LIKELY).

State Independence is established by an **executable Reset Recipe** — RaceVeil
must be able to *run* it to obtain fresh state, or independence cannot be claimed.
A Reset Recipe is exactly one of:
- **built-in reset strategy** — e.g. `fresh_code`: re-run an operator-supplied
  setup request that mints a new single-use code / resource per trial;
- **authenticated reset endpoint** — a declared in-scope request that resets the
  fixture (test apps only);
- **external fixture command** — a local command RaceVeil executes between trials
  (corpus/local use);
- **operator-supplied setup recipe** — a Workflow prefix re-run per trial.

**MVP supports only the built-in `fresh_code` strategy and operator-supplied
setup recipe** (the two that need no new machinery); the others are v1. If no
executable recipe is available, trials are `dependent`/`unknown` and Confidence is
capped at LIKELY — "fresh independent trials" is never assumed, only executed.

### Observable
Any externally measurable signal used by the Oracle:
- **Response Observables**: status code, latency, presence/value of body fields
  (balance, quantity, id, error/success markers) extracted by configured or
  learned extractors.
- **Post-state Observable** (optional, Level 4): the result of a read-only
  **state probe** Request issued after the burst (e.g. `GET /cart`,
  `GET /account/balance`) to see the persisted effect.

Observables are the *only* window RaceVeil has. No Observable ⇒ no evidence.

---

## 4. Judgment entities

### Oracle
The component that decides, from Baseline + ConcurrentTrials, whether a
concurrency-induced violation occurred. Structured as a **hierarchy of levels**
(see ARCHITECTURE.md §Oracle); higher levels give stronger evidence. The Oracle
never assumes an Invariant it did not establish, and always requires the
"absent in Baseline, present under concurrency" contrast.

### Evidence
The concrete artifacts that support (or refute) a violation: the Baseline
signatures, the per-trial success counts, the response differential
(sequential vs. concurrent), and any post-state differential. Evidence is
retained in full so a skeptic can re-derive the conclusion by hand.

### Confidence
A graded assessment — **SUSPECTED / LIKELY / CONFIRMED** — derived from
reproduction statistics (Wilson score interval over independent trials) and the
number of corroborating Observables. Confidence is never a hardcoded "5/5";
it is computed and every input is recorded. Definitions and thresholds live in
ARCHITECTURE.md §Verification and are configurable.

### Severity
An **advisory** impact label derived from the Invariant type and effect
(e.g. a duplicate-effect integrity violation ⇒ HIGH). Severity describes the
*observable integrity impact*, explicitly **not** business/financial impact,
which RaceVeil cannot know from external observation. Severity never gates a
Finding; Confidence does.

### Finding
A reproduced concurrency-induced violation: `Candidate + confirmed Invariant
violation + Confidence + Evidence + Reproduction`. The terminal, reportable unit.

### Reproduction
A self-contained recipe (the `.rv` artifact) containing everything needed to
re-run the Experiment and re-confirm the violation independently: the minimized
Workflow, Session bootstrap, Invariant, concurrency parameters, expected
Observables, and the **State Independence** assumptions (the Reset Recipe, or a
record that trials were `dependent`/`unknown`). It stores no secrets.
`raceveil replay` and `raceveil verify` consume it.

---

## 5. Relationships (at a glance)

```
Target ──covered by──> Scope
Target ──has──> Endpoint*
Endpoint* ──compose──> Workflow ──runs under──> Session
(Workflow, Invariant) = Candidate ──ranked, selected──> Experiment
Experiment = Baseline + ConcurrentTrial* ──evaluated by──> Oracle
Oracle ──emits──> (Evidence, Confidence, Severity) ──> Finding
Finding ──packaged as──> Reproduction (.rv)
```

## 6. Deliberately excluded from the domain (v1)

Not modeled, on purpose (see PRD non-goals): source symbols, code paths,
database rows/transactions, thread schedules, WebSocket frames, browser DOM
state, multi-endpoint state-machine sub-states beyond simple setup workflows.
These belong to white-box tools or to a later RaceVeil phase, and admitting them
into the domain now would break the black/grey-box guarantee.
