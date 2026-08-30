# USER_FLOWS.md — RaceVeil (Governance Spec)

The 2–4 core journeys (Round 0 item 4), drawn from
[`Design/PRD.md`](../Design/PRD.md) §12,
[`Design/API.md`](../Design/API.md), and
[`Design/UX.md`](../Design/UX.md). These are the flows CLAUDE.md's
`GOAL`/`VERIFY` declarations should trace back to when a task touches the CLI
surface.

## Flow 1 — First scan against an authorized target (MVP happy path)

1. Operator runs `raceveil scope init --target <url> --out scope.yaml`.
   RaceVeil resolves the target host, pins its IP, and writes a default-deny
   `scope.yaml` template.
2. Operator edits `scope.yaml`: confirms hosts/ports/path-prefixes, caps
   (`max_concurrency`, `max_requests_total`, `max_rate_per_sec`,
   `max_wallclock_sec`), destructive-verb rules, and `proof.max_successful_effects`.
3. Operator runs `raceveil auth login --recipe login.yaml --out session.json`
   to capture a Session (cookies/tokens/CSRF + refresh logic).
4. Operator authors `candidate.yaml` (a Workflow + Invariant — see
   [`Design/DATA_MODEL.md`](../Design/DATA_MODEL.md) §MVP candidate input) for
   the endpoint under suspicion (e.g. `POST /coupon/redeem`).
5. Operator runs
   `raceveil scan --candidate candidate.yaml --scope scope.yaml --auth session.json`.
6. RaceVeil prints the effective Scope and the declared `authorized_by`
   provenance, then prompts `Proceed? [y/N]` (safe-mode confirmation).
7. RaceVeil runs the sequential Baseline (calibrates the success/reject
   classifier), then the synchronized concurrent burst(s) via the `sync`
   engine (`H2SinglePacket` or `H1LastByte`).
8. The Oracle evaluates Observables against the Invariant (Levels 1+3 in
   Phase 1; +2/+4 once Phase 2 lands), computes Confidence via the Wilson
   interval over independent trials.
9. On a violation, RaceVeil minimizes the reproducer (concurrency `N`, then
   request set) and writes `findings/<id>.rv`.
10. Operator reviews the human-readable finding (Expected vs Observed,
    reproduction stats, evidence, Confidence, Severity) and the one-line
    reproduce command.

## Flow 2 — Independently re-confirming a finding

1. A skeptic (reviewer, CI gate, or the original operator days later) has a
   `.rv` file and the target's current `scope.yaml`/`session.json`.
2. Runs `raceveil verify findings/<id>.rv --scope scope.yaml --auth session.json --trials 10`.
3. RaceVeil refuses if the current Scope doesn't cover the finding's target
   (no accidental cross-target replay).
4. RaceVeil re-runs the minimized experiment on **fresh** trials only —
   never reuses the original run's statistics — and recomputes Confidence
   from scratch.
5. Exit code reflects the re-confirmed band (`0` no findings, `2` LIKELY+,
   `3` SUSPECTED only), suitable for a CI gate.

## Flow 3 — Regression-guarding a known race in CI

1. Team commits a previously-confirmed `.rv` file to the repository as a
   regression fixture, alongside the fix for the underlying race.
2. CI pipeline runs `raceveil verify known-race.rv --scope scope.yaml --auth session.json`
   against staging after every deploy candidate.
3. Exit `0` (no longer reproduces) passes the build; a re-confirmed finding
   fails it — the fix regressed.
4. (Broader `scan <url>` + SARIF output for general CI gating is a v1/Phase 5
   capability, out of scope for this build per `docs/PRD.md` §3 — Flow 3 uses
   only `verify`, which is in scope.)

## Flow 4 — Reading a clean result (no violation)

1. Operator runs a scan against a target believed to be correctly
   synchronized (e.g. the safe twin in the seeded corpus, TEST_PLAN Layer 6).
2. RaceVeil reports `No concurrency integrity violations found (LIKELY+)`,
   the count of experiments run, and any SUSPECTED-only signals recorded
   separately (never presented as a vulnerability).
3. Exit code `0`. This path is explicitly a first-class, tested outcome
   (Design/UX.md: "reporting clean... is a headline result, not an
   anticlimax") — it is what makes a positive finding on Flow 1 credible.
