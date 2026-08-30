# API.md — RaceVeil CLI

RaceVeil is a single binary, `raceveil`, built with Cobra. It should feel like a
serious security/dev tool: explicit authorization, safe defaults, machine-readable
output, deterministic re-runs. There is no network API and no daemon in v1.

Global conventions:
- Every command that touches a target requires a resolvable **Scope**
  (`--scope`) or is refused. There is no "just point it at prod" shortcut.
  RaceVeil enforces the Scope **technically**; it does not verify that the
  operator is actually authorized — `authorized_by` in the Scope is
  provenance/audit metadata, and the operator is responsible for real
  authorization.
- `--safe-mode` (default **on**) enforces the strictest caps and blocks
  destructive verbs, and refuses targets resolving to public IPs unless the Scope
  lists them and `--i-am-authorized` is passed; `--no-safe-mode` also requires
  `--i-am-authorized` and prints the Scope provenance it's relying on. These are
  technical gates, not a determination that testing is permitted.
- `-o/--out` sets the run directory (default `./raceveil-run-<timestamp>`).
- `--format {human,json,sarif}` for machine consumption; `--json` is shorthand.
- Exit codes (stable, for CI): `0` no findings · `2` findings at
  LIKELY+ · `3` findings SUSPECTED only · `4` scan error · `5` scope/authorization
  refusal.

## Commands

```
raceveil scope   init|check      Create/validate an authorization scope
raceveil auth    login|check     Capture/validate a Session
raceveil scan    --candidate F   MVP path: run a supplied Candidate (Workflow + Invariant)
raceveil scan    <url>           v1 path: discover → experiment → verify → findings
raceveil verify  <finding.rv>    Independently re-confirm a finding (fresh stats)
raceveil replay  <finding.rv>    Re-run the exact minimized experiment once
raceveil report  <run-dir>       Render findings (human/json/sarif)
raceveil list    <run-dir>       List candidates/experiments/findings
```

`scan` has two mutually exclusive input modes: **`--candidate candidate.yaml`**
(the MVP path — the operator supplies one Candidate directly; no crawling or
discovery) and **`<url>`** (the v1 path — automatic discovery/ranking, Phase 3).
Everything after the input stage (baseline → concurrent trials → oracle →
verify → minimize → `.rv`) is identical between the two.

### `scan --candidate` (MVP path)
```
raceveil scan --candidate candidate.yaml --scope scope.yaml --auth session.json
```
Runs the single Candidate defined in `candidate.yaml` (Workflow + Invariant,
optional classifiers / post-state probe / reset recipe; shape in DATA_MODEL.md).
No discovery is performed. This is the path the MVP must make excellent.

### `scope init` / `scope check`
```
raceveil scope init --target https://staging.shop.internal --out scope.yaml
raceveil scope check --scope scope.yaml
```
`init` resolves hosts, pins IPs, writes a default-deny template with caps.
`check` validates reachability, that IPs are within authorized ranges, and prints
exactly what RaceVeil would and would not touch. Run this before anything.

### `auth login` / `auth check`
```
raceveil auth login --recipe login.yaml --out session.json
raceveil auth check --scope scope.yaml --auth session.json
```
A login recipe describes how to obtain a Session (form post, token exchange) and
how to detect expiry + refresh. `check` confirms the Session is live and
authenticated. Multiple sessions (e.g. attacker/victim) get isolation groups.

### `scan <url>` (v1 discovery path)
```
raceveil scan https://staging.shop.internal \
    --scope scope.yaml \
    --auth session.json \
    [--invariants invariants.yaml] \
    [--import traffic.har] \
    [--include "POST:/coupon/**,POST:/inventory/**"] \
    [--max-concurrency 30] \
    [--max-N 20] \
    [--trials 7] \
    [--min-trials 5] \
    [--sync auto|h2|h1] \
    [--discovery crawl|import|both] \
    [--safe-mode] \
    [--out run-dir] \
    [--format human|json|sarif] \
    [--seed 4815162342]
```

Key flags:
- `--candidate` — MVP path: a single `candidate.yaml` (a complete Candidate =
  Workflow + Invariant, supplied by the operator). Mutually exclusive with `<url>`
  discovery; when used, no crawling/ranking/inference runs.
- `--invariants` — **v1/discovery path only**: user declarations that
  constrain/override *inferred* invariants during automatic discovery (Oracle
  level 5). It does **not** supply a Workflow (that comes from discovery), which
  is the difference from `candidate.yaml`. The canonical types apply (DOMAIN.md);
  `max_successes` may carry an optional `window: {duration: 60s}` for
  rate-limit-style temporal limits (not a separate type).
- `--import` — use captured traffic (HAR/proxy) instead of/with crawling; the
  pragmatic path for SPAs.
- `--include`/`--exclude` — restrict experiments to endpoint patterns (safety +
  focus).
- `--max-N` — ceiling of the concurrency ladder; `--max-concurrency` — hard cap
  actually sent at once (Scope may lower it).
- `--trials`/`--min-trials` — verification and minimization trial counts (counted
  over **independent** trials only; DOMAIN.md State Independence).
- `--sync auto` — h2 single-packet when the target speaks HTTP/2, else h1
  last-byte.
- `--seed` — fixes harness RNG so runs are reproducible (the race outcome stays
  probabilistic; the plan doesn't).

Proof-cap behavior: RaceVeil performs only the minimum successful effects needed
to establish the violation (`required_proof_effects`, e.g. `L+1` for
`max_successes(L)`) and stops. If the Scope's `proof.max_successful_effects`
ceiling is below an invariant's `required_proof_effects`, that invariant is
reported as **unprovable-under-cap** rather than exceeding the ceiling.

Example run (human output): see UX.md.

### `verify`
```
raceveil verify findings/coupon-redeem.rv --scope scope.yaml --auth session.json --trials 10
```
Re-runs the minimized experiment on **fresh** trials and recomputes Confidence
from scratch. This is the command a skeptic (or CI gate) runs to trust a finding.
Exit code reflects the re-confirmed band. Refuses if the current Scope doesn't
cover the finding's target (no accidental cross-target replay).

### `replay`
```
raceveil replay findings/coupon-redeem.rv --scope scope.yaml --auth session.json
```
Runs the exact recorded burst **once** for demonstration/debugging and prints the
observed vs. expected. Does not recompute Confidence. Honors
`proof.max_successful_effects` so a replay can't be used to drain state.

### `report`
```
raceveil report ./raceveil-run-2026-... --format sarif > raceveil.sarif
raceveil report ./raceveil-run-2026-... --format md   > REPORT.md
```
Renders findings from an existing run dir. SARIF integrates with code-scanning
dashboards; only LIKELY+ findings are emitted as results (SUSPECTED go to a
separate notes section, never as asserted vulnerabilities).

## CI usage

```yaml
# Gate a deploy on concurrency-integrity regressions against staging.
- run: raceveil scope check --scope scope.yaml
- run: raceveil auth login --recipe login.yaml --out session.json
- run: |
    raceveil scan "$STAGING_URL" \
      --scope scope.yaml --auth session.json \
      --invariants invariants.yaml \
      --format sarif --out run --seed 1
- run: raceveil report run --format sarif > raceveil.sarif
# job fails on exit code 2 (LIKELY+ finding); upload SARIF for review
```

For a **regression guard** (known race must stay fixed), commit the `.rv` and run
`raceveil verify known-race.rv`; expect exit `0` (no longer reproduces). A
re-confirmed finding fails the build.

## Non-commands (explicitly not provided)
No `exploit`, no `drain`, no `bruteforce`, no `stealth`, no `persist`. RaceVeil
stops at proof (SECURITY.md). Requests to add such commands are out of scope for
the project.
