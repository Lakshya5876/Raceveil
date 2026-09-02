# DECISIONS.md — RaceVeil

Architecture Decision Records. Each entry: **Decision · Context · Options ·
Chosen · Reason · Tradeoffs**. These records are load-bearing — the design docs
assume them and must not contradict them.

---

## ADR-001 — Black/grey-box, not white-box
**Context.** White-box race detection is a solved research area (ReqRacer =
runtime instrumentation, RaceDB = concolic execution over source, ACIDRain = DB
access traces). Those need source, instrumentation, or DB visibility.
**Options.** (a) Build another white-box detector; (b) black/grey-box against a
running target.
**Chosen.** Black/grey-box.
**Reason.** The empty niche is *deployable verification of a running system you
don't have source for*. It's also the AI-era story: an independent verifier that
doesn't share the implementation's assumptions. Every existing discovery tool
needs your source; RaceVeil needs your URL.
**Tradeoffs.** No ground-truth from internals ⇒ the oracle is genuinely hard and
cannot claim completeness. Accepted — that difficulty *is* the contribution.

## ADR-002 — Verification, not weaponization
**Context.** Dual-use tool; trivial to slide into an exploitation framework.
**Options.** (a) Maximize impact/exploitability; (b) stop at proof.
**Chosen.** Stop at proof; hard-exclude weaponization features (SECURITY.md).
**Reason.** Keeps the project defensible, publishable, and OSS-viable; aligns
with the actual value (does the invariant hold?), which needs only minimal proof.
**Tradeoffs.** Less "powerful" for offensive users. Intended.

## ADR-003 — Focus on limit-overrun / duplicate-effect / uniqueness first
**Context.** Race classes vary wildly in oracle difficulty.
**Options.** (a) General race detection; (b) start with observable-invariant
classes.
**Chosen.** Limit-overrun / duplicate-effect / uniqueness / temporal-limit
violations (rate-limit bypass being the canonical example of the temporal case,
expressed as windowed `max_successes` — not a separate invariant type).
**Reason.** These have **strong, externally observable invariants** (`S ≤ L`),
which is the only way a black-box oracle can be trustworthy without source. It's
also the highest-impact, most-recognized class.
**Tradeoffs.** Doesn't cover subtle multi-endpoint state-machine sub-states in
v1. Deferred deliberately (ROADMAP Phase 6); admitting them now would force
assumptions the black-box oracle can't support.

## ADR-004 — The oracle is the central architecture
**Context.** Request synchronization is prior art; the missing piece is judging
success without source.
**Options.** (a) Treat oracle as a thin status-code check; (b) make it a
first-class, leveled subsystem.
**Chosen.** Leveled oracle (1–5) with baseline calibration + corroboration + FP
gate (ARCHITECTURE §4).
**Reason.** It's the only genuinely novel, genuinely hard part, and the thing an
AI coding agent can't safely abstract away. If it isn't excellent, nothing else
matters.
**Tradeoffs.** Most design/tuning effort concentrates here; higher risk. Accepted
and mitigated by the safe-twin test layer.

## ADR-005 — Evidence & reproducibility are first-class
**Context.** A race that fired once proves little.
**Options.** (a) Report first trigger; (b) require reproduction + retained
evidence + replayable artifact.
**Chosen.** (b). Confidence from Wilson interval over independent trials; full
evidence retained; `.rv` reproduction bundle.
**Reason.** Trust and shareability; lets a skeptic re-derive the conclusion.
**Tradeoffs.** More requests, more state management (fresh state per trial).
Worth it — reproducibility is the product's credibility.

## ADR-006 — Not another Turbo Intruder; not a Burp wrapper
**Context.** Turbo Intruder already sends synchronized bursts inside Burp.
**Options.** (a) Plugin on Burp/Turbo Intruder; (b) standalone tool that treats
synchronization as commodity infrastructure and adds discovery + oracle +
verification.
**Chosen.** (b) standalone.
**Reason.** The contribution is the verification layer *around* the burst, not the
burst. A Burp plugin would inherit GUI/manual-driving assumptions and a
Java/Burp-API dependency, and couldn't be a clean CI-friendly binary. We
acknowledge and reuse the *technique*, not the tool.
**Tradeoffs.** Reimplement synchronization ourselves (ADR-009). Accepted.

## ADR-007 — Not an AI code-review / code-fix tool
**Context.** Tempting to "use AI to find/fix races."
**Options.** (a) LLM code review; (b) behavioral verification of the running
system.
**Chosen.** (b).
**Reason.** The whole thesis is *independent* verification that doesn't rely on
the same reasoning that wrote the code. An LLM reviewer shares the implementation's
blind spots and can't produce reproducible runtime evidence.
**Tradeoffs.** Doesn't localize the bug in source. Fine — the minimized `.rv`
gives an engineer everything needed to find it.

## ADR-008 — Language: Go
**Context.** Need a single local binary, precise HTTP/2 frame control, strong
concurrency, good CLI ergonomics.
**Options.** Python (h2spacex/Scapy), Go, Rust, JVM/Kotlin.
**Chosen.** Go.
**Reason.** Single static binary (matches local-first principle); goroutines fit
the scheduler; `golang.org/x/net/http2` exposes the `Framer` for single-packet
delivery; excellent cross-compilation and CLI ecosystem; permissive-license
ecosystem.
**Tradeoffs.** Python would prototype the burst faster (h2spacex exists) but is
GPL-3 (ADR-009), GIL-bound, and reads as a research script. **JVM/Kotlin** is the
maintainer's home turf (Spring Boot) and was seriously considered, but stdlib
HTTP/2 doesn't expose frame-level single-packet control cleanly, pushing you to
hand-roll HTTP/2 over sockets anyway — Go gives that control with less friction
and a better single-binary story. The maintainer's Java strength is still used
where it's genuinely right: the **seeded Spring Boot corpus** (TEST_PLAN Layer 5).
Rust was rejected as slower to build solo for equal benefit here.

## ADR-009 — Reimplement single-packet delivery; avoid GPL-3 dependency
**Context.** `h2spacex` is a ready HTTP/2 single-packet library — but GPL-3.
**Options.** (a) Depend on h2spacex (GPL-3) and inherit its license; (b)
reimplement last-frame synchronization from the published technique under
Apache-2.0.
**Chosen.** (b) reimplement.
**Reason.** Keep RaceVeil Apache-2.0 for OSS/commercial friendliness; the
technique is documented and the reimplementation is modest. Avoids a runtime
copyleft obligation on the whole tool.
**Tradeoffs.** More engineering for infrastructure that is explicitly *not* the
novel part. Accepted; it's also good protocol-depth signal.

## ADR-010 — Local-first, single binary; no infrastructure
**Context.** Scope pressure toward servers/queues/DBs/orchestration.
**Options.** (a) Service with control plane; (b) single local CLI + filesystem.
**Chosen.** (b).
**Reason.** The problem needs none of it; infra would be résumé theater and hurt
adoption. Simple local architecture over infrastructure theater.
**Tradeoffs.** No built-in multi-target fleet scanning. Out of scope by design.

## ADR-011 — Filesystem persistence, no database
**Context.** Need to persist runs, experiments, findings.
**Options.** (a) SQLite/embedded DB; (b) JSON/JSONL run directory.
**Chosen.** (b).
**Reason.** Runs are write-once, human-inspectable, git-attachable; a DB adds
dependency and opacity for no query need at this scale.
**Tradeoffs.** No rich querying across many runs. If that need appears later, add
an index — not now.

## ADR-012 — Discovery: crawler + traffic import, no headless browser (v1)
**Context.** SPAs hide endpoints behind JS.
**Options.** (a) Full headless-browser crawl; (b) lightweight crawler + OpenAPI
import + HAR/proxy import.
**Chosen.** (b).
**Reason.** A headless browser is heavy, flaky, and a large dependency; letting a
human drive the app once and import traffic is more reliable and keeps the binary
lean. Import also fits pentest/bug-bounty workflows.
**Tradeoffs.** Less "fully automatic" on SPAs without import. Acceptable; revisit
if demand is real.

## ADR-013 — Confidence via Wilson score interval over independent trials
**Context.** Need honest confidence, warned against hardcoded "5/5 = high."
**Options.** (a) Fixed ratios; (b) normal-approximation CI; (c) Wilson interval;
(d) full Bayesian model.
**Chosen.** (c) Wilson, with bands tuned empirically (TEST_PLAN Layer 4).
**Reason.** Wilson behaves well for small `K` and proportions near 0/1 (exactly
our regime) where the normal approximation fails; simpler and more defensible
than a Bayesian model for v1. The per-trial violation stays a *logical*
counterexample; only reproduction is treated statistically.
**Tradeoffs.** Requires fresh, independent state per trial for validity; dependent
trials are capped at LIKELY. Documented and enforced.

## ADR-014 — Minimization by decreasing-sweep + delta-debugging, not bisection
**Context.** Reduce `N` and workflow to the smallest reproducer.
**Options.** (a) Binary-search `N`; (b) decreasing sweep with statistical
re-verification + delta-debugging on requests.
**Chosen.** (b).
**Reason.** Reproduction is probabilistic; there's no crisp monotone threshold to
bisect, so bisection would report unstable minima. Re-verifying at each step is
correct even if slower.
**Tradeoffs.** More trials during minimization. Accepted for correctness.

## ADR-015 — Invariant-inference gate as the primary false-positive defense
**Context.** The dangerous FP is flagging a legitimately non-idempotent endpoint.
**Options.** (a) Flag any `S > 1`; (b) only assert violations where an invariant
was established (baseline-inferred limit or user-declared).
**Chosen.** (b).
**Reason.** Without an established invariant there is nothing to violate;
`S = N` on `POST /comment` is correct behavior. This single rule also yields the
clean result on synchronized-safe twins.
**Tradeoffs.** Misses violations where the limit isn't observable in a short probe
and the user didn't declare one. Mitigated by user-supplied invariants
(Oracle level 5) and richer probing later.

## ADR-016 — Licensing posture
**Decision.** Apache-2.0 for RaceVeil; no GPL runtime deps (see ADR-009); corpus
apps and their CVE references used locally for testing only, credited, never
redistributed as exploits.
**Reason.** OSS + commercial friendliness, clean provenance.
**Tradeoffs.** Occasionally reimplementing rather than importing. Accepted.

## ADR-017 — PortSwigger race-lab validation: reconciled against local-first/zero-cost, deferred
**Context.** ROADMAP.md Phase 5 names "solves the limit-overrun/rate-limit
PortSwigger labs autonomously black-box" as an exit criterion. PortSwigger's
Web Security Academy labs are live, remote, account-gated, per-session-
ephemeral instances on PortSwigger's own infrastructure — not something a
local Docker Compose stack can stand up. Attempting them requires a
PortSwigger Academy account (their labs are free to use but still require
account creation) and sending scan traffic to a target this build's own
zero-cost/local-first operating constraint (this session's explicit
instruction: no infrastructure that isn't local, no new external accounts
created without the user doing it themselves) does not authorize creating
or using.
**Options considered.** (a) Create a PortSwigger account and drive the labs
through the Browser tool interactively; (b) build local fixture endpoints
that approximate the named lab archetypes (limit-overrun, rate-limit
bypass) without claiming fidelity to PortSwigger's actual lab
implementation; (c) defer, documented, until the user supplies Academy
access or explicitly asks for (a) or (b).
**Chosen.** (c), with the corpus (Phase 2, `corpus/node` +
`corpus/spring-boot`) and the CVE-2024-58248 rediscovery
(`benchmark/nopcommerce-cve-2024-58248/`, Phase 5) standing in as the
evidence actually produced this session: both are free, local, and
independently reproducible, and both exercise the same limit-overrun /
duplicate-effect archetype the PortSwigger labs test, just against
purpose-built and real-world targets instead of PortSwigger's proprietary
ones.
**Reason.** Account creation is a user action per this session's
permission boundaries, not something to do unprompted even for a free
service; approximated local fixtures would be a weaker, self-graded
substitute for a genuinely independent benchmark and shouldn't be
presented as "solves PortSwigger labs" when it wasn't run against them.
Honest deferral beats either.
**Tradeoffs.** The literal Phase 5 exit-criterion line item is unmet.
Accepted — the two evidence sources actually delivered are the same
strength of proof for the same underlying claim (RaceVeil finds real
limit-overrun races and stays clean on synchronized-safe code), and
PortSwigger-lab validation can be picked up as a same-shaped follow-on the
moment the user provides Academy access or asks for it directly.
