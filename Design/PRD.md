# PRD.md — RaceVeil

## 1. Problem

Web applications routinely contain **concurrency-induced integrity violations**:
places where the code looks correct and passes tests, but where issuing several
requests at nearly the same instant lets a caller do something the application
was supposed to forbid — redeem a one-time coupon twice when only one redemption
is allowed, buy the last item
twice, overdraw a balance, bypass a rate limit, create a "unique" resource
twice.

These bugs are structurally invisible to the usual defenses:

- **Unit/integration tests** exercise sequential paths and pass.
- **Static analysis / linters** see locks and transactions in the source and
  approve, without knowing whether they actually hold at the deployed isolation
  level, behind the real connection pool, through the real ORM.
- **Conventional DAST scanners** fire one request at a time and cannot express
  "do this 20 times in the same millisecond and check an invariant."

So the entire class is handled today by **expert humans doing manual work** with
request-synchronization tools. That's slow, unrepeatable, and doesn't run in CI.

## 2. The AI-era motivation (a sharpening, not the sole reason)

The traditional pipeline:

```
human writes code → tests → static analysis → ship
```

The increasingly common pipeline:

```
AI generates code → tests may pass → static analysis may pass → ship
```

An AI coding agent will happily add a lock, wrap a transaction, and write a test
that passes — and produce code that *looks* race-safe while the deployed system
is not (wrong isolation level, lock on the wrong object, check-then-act across
two statements, ORM hiding the transaction boundary). The agent is not a
trustworthy verifier of its own concurrency correctness, because the reasoning
that wrote the bug is the reasoning that would have to catch it.

RaceVeil is the **independent behavioral verification layer**:

```
generation → implementation → tests → static analysis → RaceVeil → adversarial behavioral verification
```

It attacks the **running system** and asks one question reality can answer: *does
the deployed behavior actually preserve its invariants under concurrency?*

This is a **verification problem, not a code-generation problem.** RaceVeil never
generates or fixes application code. It also remains fully useful for
traditionally hand-written software — the AI framing explains *why the need is
growing*, not the only reason the tool exists.

## 3. Honest position relative to prior art

RaceVeil does **not** claim to invent race testing or race discovery.

- **Burp / Turbo Intruder / single-packet attack** already deliver highly
  synchronized concurrent requests, including sub-millisecond HTTP/2 bursts.
  RaceVeil treats request synchronization as **prior-art infrastructure** and
  reimplements it (license reasons, DECISIONS.md), it does not claim novelty
  there.
- **ReqRacer, RaceDB, ACIDRain** already automate server-side race *detection* —
  but each requires white-box visibility: runtime instrumentation, concolic
  execution over source, or database-access traces.
- Offensive tools (`race-the-web`, `h2spacex`, `Raceocat`) already fire
  concurrent requests, but **leave the "did it work?" judgment entirely to the
  human.**

RaceVeil's contribution is the **external, black/grey-box verification layer**
that none of those provide together: automatic Candidate discovery, **observable
integrity Oracles**, baseline-vs-concurrent experimentation, statistical
confidence, reproduction, minimization, and evidence generation — against a
running target with **no source, no instrumentation, no database access.**

The single defensible sentence: **every existing race *discovery* tool needs
your source; RaceVeil needs your URL.**

## 4. Target users

1. **Backend / platform engineers** (incl. teams shipping AI-assisted code) who
   want a CI check that concurrency invariants actually hold in the deployed
   service.
2. **Application security engineers / pentesters** who currently do this by hand
   and want repeatable, evidence-backed findings.
3. **Bug-bounty hunters** testing authorized targets who want automated
   candidate triage plus a trustworthy oracle instead of eyeballing responses.
4. **OSS maintainers** who want a regression guard against reintroducing a
   known race.

## 5. Core use case

> Give RaceVeil an authorized running web application and either a **supplied
> Candidate** (`candidate.yaml`, the MVP path) or, in v1, a **discoverable
> workflow set** (`scan <url>`). It establishes each candidate's sequential
> baseline, runs synchronized concurrent trials, detects any externally
> observable invariant violation, reproduces and minimizes it, and emits a
> finding an engineer can independently re-run.

## 6. Product promise (narrow and defensible)

> **RaceVeil automatically tests running web applications for externally
> observable integrity violations caused by concurrent requests, and produces
> reproducible, minimized, statistically-qualified evidence for each one.**

It explicitly does **not** promise to "find all race conditions." It promises to
correctly and reproducibly identify a well-defined, observable class — and,
equally important, to **not cry wolf** on correctly-synchronized code.

## 7. Non-goals (explicit)

- Not a general race detector; not a claim of completeness or "finds all races."
- Not white-box: no source analysis, concolic execution, or DB/runtime
  instrumentation.
- Not a Turbo Intruder replacement or a Burp wrapper; not "yet another concurrent
  request sender."
- Not an AI code-review or code-fix tool; it never edits application code.
- Not a weaponization / exploitation framework: no stealth, evasion, persistence,
  credential theft, privilege escalation, or automated destructive exploitation
  (SECURITY.md). It stops at **proof**.
- Not a full DAST platform (no XSS/SQLi/etc.).
- No distributed control plane, no Kubernetes, no microservices, no cloud SaaS.
- v1 does not attempt WebSocket races, HTTP/3, browser automation, or complex
  multi-endpoint state-machine sub-states.

## 8. Scope tiers

### MVP (the milestone that must be excellent)
Given an authorized app containing a **limit-overrun / duplicate-effect /
uniqueness / rate-limit-bypass** race with an externally observable consequence,
and a **manually supplied or imported** candidate workflow, RaceVeil can: **take
that candidate workflow → establish the sequential baseline → run synchronized
concurrent trials (HTTP/2 single-packet + HTTP/1.1 last-byte) → detect the
observable invariant violation via a baseline-calibrated oracle → reproduce and
statistically qualify it across independent trials → minimize the triggering
request set → emit a trustworthy `.rv` finding.** And it must **not** flag the
synchronized-safe twin of the same app.

The MVP spans two phases (ROADMAP): the **Phase 1 vertical slice** proves the full
pipeline end-to-end but, with only Oracle levels 1 + 3, its strongest confidence
is **LIKELY** (CONFIRMED requires a Level-2 or Level-4 corroborating observable,
which is not yet built). **Phase 2 adds Level 2 + Level 4 corroboration**, at
which point the MVP can reach **CONFIRMED** and is validated against the
vulnerable corpus and synchronized-safe twins. "Trustworthy `.rv`" in Phase 1
means a reproduced, statistically-qualified LIKELY finding that `verify`
re-confirms; CONFIRMED is a Phase-2 capability.

Automatic crawler-driven workflow discovery is **not** an MVP requirement — the
MVP exists to prove the oracle/thesis on a supplied candidate. Discovery is added
in v1 (see below and ROADMAP Phase 3).

The MVP candidate is supplied as a single **`candidate.yaml`** file (a Candidate =
Workflow + Invariant; shape in DATA_MODEL.md), run via
`raceveil scan --candidate candidate.yaml --scope scope.yaml --auth session.json`.
No crawling or ranking is required on this path. (A minimal crawler/import exists
only as request-capture infrastructure and is distinct from *automatic candidate
discovery*, which is Phase 3.)

- Interface: CLI, single local Go binary.
- Oracle: Phase 1 levels 1 + 3 (+ level 5 user invariants); Phase 2 adds levels
  2 and 4 (the corroboration CONFIRMED needs).
- Invariant types (canonical set, defined in DOMAIN.md): `max_successes`,
  `uniqueness`, `monotonic_limit`, `single_transition`. Rate-limit bypass is
  expressed as a temporal/windowed application of `max_successes` (at most `L`
  successes per fixed window), not a separate invariant type. Workflows:
  single-request and short setup-then-act sequences (setup once, Act Request
  replicated; DOMAIN.md).

### v1 (first release)
MVP + automatic crawler-driven discovery with ranking, richer body-field
extractors, minimization of multi-request workflows, machine-readable
(JSON + SARIF) output, CI mode with exit codes, PortSwigger-lab regression suite
green, and a documented seeded-corpus benchmark (Spring Boot + Node, vulnerable
and safe twins).

### Future (only if earlier phases are excellent)
Multi-endpoint / partial-construction / state-machine sub-state oracles,
user-defined invariant DSL, WebSocket delivery, HTTP/3, traffic-import from HAR /
proxy, deeper post-state probing.

## 9. Success metrics

Measured on the seeded corpus + PortSwigger labs + known-vuln OSS (TEST_PLAN.md):

- **Detection rate** on seeded vulnerable targets: ≥ 90% of seeded limit-overrun
  races found at LIKELY+ confidence.
- **False-positive rate** on synchronized-safe twins: **0 CONFIRMED**; ≤ 5%
  SUSPECTED (and SUSPECTED must never be reported as a vulnerability).
- **Reproduction stability**: a CONFIRMED finding re-confirms on `verify` in a
  fresh environment ≥ 90% of the time.
- **PortSwigger labs**: limit-overrun + rate-limit labs solved autonomously,
  black-box.
- **Known-vuln OSS**: rediscovers ≥ 1 real CVE-class race (e.g. nopCommerce
  CVE-2024-58248 gift-card double-redeem) in a local container, no source.
- **Minimization**: median minimized concurrency `N` reported, with retained
  reproduction rate above the LIKELY threshold.

The metric that matters most is not requests sent or races triggered; it is:
**can RaceVeil distinguish a concurrency-induced integrity violation from
ordinary nondeterminism?** i.e. detection rate *and* FP rate together.

## 10. Failure criteria (when to admit the design is wrong)

- CONFIRMED findings on synchronized-safe twins that can't be driven to zero by
  the invariant-inference gate and corroboration requirement.
- Oracle cannot separate success from rejection on realistic apps without
  per-target manual configuration (i.e. no meaningful automation over Turbo
  Intruder).
- Reproduction proves unstable enough that confidence is meaningless (findings
  don't re-confirm).

If these hold after honest effort, the black-box oracle thesis is falsified for
this class and the project should say so publicly rather than ship a noisy tool.

## 11. Threat / abuse boundaries

RaceVeil is dual-use. It is built for **authorized** testing and is deliberately
constrained to **verification, not weaponization** (full model in SECURITY.md):
default-deny scope, concurrency/rate caps, capping proof to the minimum effect
needed, SSRF/DNS-rebinding protection, secret redaction, audit logging, and no
stealth/persistence/exfiltration/escalation features. RaceVeil enforces its
configured Scope **technically**; it does **not** independently verify that the
operator is organizationally or legally authorized. The Scope's `authorized_by`
field is provenance/audit metadata, not proof of authorization, and the operator
remains responsible for ensuring authorization exists. In safe-mode, public
targets are refused unless the required explicit operator flag
(`--i-am-authorized`) and Scope conditions are satisfied — a technical gate, not a
determination that testing is actually permitted.

## 12. Representative user workflow

```
# one-time: declare what you're allowed to touch
$ raceveil scope init --target https://staging.shop.internal --out scope.yaml
# (edit scope.yaml: hosts, path prefixes, caps, destructive rules)

$ raceveil auth login --recipe login.yaml        # capture a Session

$ raceveil scan https://staging.shop.internal \
      --scope scope.yaml --auth session.json --safe-mode

Discovery ██████████████████░░ 82%   candidates: 14   experiments queued: 6
Testing POST /coupon/redeem …    baseline: limit=1 detected
Testing POST /inventory/reserve …
...

CRITICAL  CONCURRENCY INTEGRITY VIOLATION   [CONFIRMED]
  POST /coupon/redeem
  Expected successful effects (max_successes=1): 1
  Observed successful effects (N=2, proof cap = L+1 = 2): 2
  Reproduction: 5/6 fresh-state trials   Wilson95 lower bound: 0.436 (provisional band)
  Evidence: response differential + post-state redemption-count probe
  Severity: HIGH — duplicate-effect integrity violation (advisory)
  Reproduce: raceveil replay findings/coupon-redeem.rv

$ raceveil verify findings/coupon-redeem.rv     # independent re-confirmation
$ raceveil report ./run-2026-... --format sarif  # for CI / dashboards
```
