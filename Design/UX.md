# UX.md — RaceVeil

The UX exists to make one thing effortless: **understanding whether a
concurrency integrity violation is real, and how sure RaceVeil is.** Everything
optimizes for clarity and for *never overstating certainty*.

## Principles
1. First run should need almost nothing beyond a URL and a scope.
2. Always show what will and won't be touched *before* touching it.
3. Progress should reveal the experiment, not just a spinner.
4. A finding must state Expected vs Observed, the concurrency count, the
   reproduction stats, and the evidence — in that order.
5. Certainty is graded and earned: **SUSPECTED → LIKELY → CONFIRMED**. The tool
   never prints "vulnerable" for a SUSPECTED signal.
6. Every finding ends with the one command that reproduces it.

## First run

```
$ raceveil scope init --target https://localhost:8443 --out scope.yaml
Resolved localhost:8443 → 127.0.0.1 (pinned). Wrote default-deny scope.yaml.
Review it, then run a scan. RaceVeil will only touch what scope.yaml allows.

$ raceveil scan https://localhost:8443 --scope scope.yaml --auth session.json
Scope: 127.0.0.1:8443  paths:/  caps: N≤30 rate≤200/s  destructive:blocked
Authorized-by: jane@corp — SEC-1421   (declared provenance; not verified by RaceVeil)
Proceed? [y/N]
```

The confirmation prints the *effective Scope* and the operator-declared
authorization provenance every time in safe-mode — RaceVeil enforces the Scope
but does not verify the authorization claim. Public targets are refused here
unless the Scope lists them and `--i-am-authorized` is set, not mid-scan.

## Progress display

On the v1 `scan <url>` (discovery) path, discovery and ranking appear first; on
the MVP `scan --candidate` path these two lines are absent (the Candidate is
supplied) and output starts at *Experiments*.

```
Discovery  ██████████████████░░  82%   endpoints:41  workflows:19
Ranking    candidates:14  (limit observed in baseline probe: 6)

Experiments  [2/6]
  ✔ POST /coupon/redeem      baseline: max_successes=1 (clean)   → testing N=2,5,10,18
  ▶ POST /inventory/reserve  baseline: max_successes=1 (clean)   → N=5
    POST /vote               baseline: no limit found → skipped (no invariant)
    GET  /account            read-only → skipped (non-mutating)
```

Notice the UX *shows the false-positive gate working*: `/vote` with no observed
limit is skipped, not flagged. Non-mutating endpoints are skipped. This is a
feature the user should see, because it's why they can trust the output.

## A confirmed finding

```
CRITICAL  CONCURRENCY INTEGRITY VIOLATION                         [CONFIRMED]
POST /coupon/redeem   (workflow: POST /cart/coupon)

  Invariant     max_successes = 1   (inferred from baseline)
  Expected      1 successful effect
  Observed      2 successful effects   under N=2 concurrent   (proof cap = L+1 = 2 reached)

  Reproduction  5/6 fresh-state (independent) trials   Wilson95 lower bound 0.436
  Minimized     N=2  (smallest burst that still proves S > L)
  Evidence      primary  Level-3 cross-request: 2 distinct redemption receipts
                corrob.  Level-4 post-state: 2 redemptions recorded (expected ≤ 1)
  Confidence    CONFIRMED  (Level-3 violation + ≥1 independent corroborating
                observable [here Level 4], on independent trials;
                confidence bands provisional — pending corpus calibration)
  Severity      HIGH (advisory: duplicate-effect integrity violation)

  Reproduce     raceveil replay findings/coupon-redeem.rv
  Re-verify     raceveil verify  findings/coupon-redeem.rv
```

## A weaker signal (honest downgrade)

```
MEDIUM  POSSIBLE CONCURRENCY INTEGRITY VIOLATION                  [LIKELY]
POST /inventory/reserve

  Invariant     monotonic_limit(stock ≥ 0)   (inferred)
  Observed      2 reservations of the last 1 unit under N=10
  Reproduction  2/8 fresh-state trials       Wilson95 lower bound 0.071
  Evidence      Level-3 cross-request only (no Level-2/4 corroboration available)
  Confidence    LIKELY  (single observable; reproduction low but nonzero)
  Note          Provide a post-state probe (GET /inventory/{id}) to upgrade,
                or declare an invariant in invariants.yaml.
```

The UX tells the user *exactly why* it isn't CONFIRMED and what would upgrade it.
No inflation.

## A clean result (equally important)

```
No concurrency integrity violations found (LIKELY+).
  6 experiments run · 0 violations · 3 endpoints skipped (no invariant)
  1 SUSPECTED signal recorded in run/notes (classifier separation weak) — not a vulnerability.
Exit code: 0
```

RaceVeil reporting "clean" on a correctly-synchronized app is a headline result,
not an anticlimax — it's what makes a positive finding credible. The
synchronized-safe corpus twins exist to guarantee this path stays clean
(TEST_PLAN.md).

## Evidence hierarchy shown to the user
- **SUSPECTED** — a signal, but classifier separation weak or invariant only
  heuristic. Recorded, never called a vulnerability.
- **LIKELY** — reproduced at least once with a real invariant, but no
  corroborating observable, or dependent/unknown trials.
- **CONFIRMED** — a Level-3 invariant violation **plus at least one independent
  corroborating observable (Level 2 or Level 4)**, reproduced across
  **independent** trials (fresh state per trial) with statistical confidence.
  One corroborating observable is sufficient (Level 3 + Level 2, or Level 3 +
  Level 4); both is stronger but not required. Trials that are dependent or of
  unknown state independence are **capped at LIKELY** and can never reach
  CONFIRMED.

The numeric cutoffs behind these bands are **provisional defaults**, not
validated constants: they are calibrated empirically against the seeded corpus
and synchronized-safe twins (TEST_PLAN.md), and output labels the band version so
a reader knows the thresholds are still being tuned. The methodology (Wilson
interval over independent trials + corroboration) is fixed; the exact thresholds
are not yet final.

## Report artifacts
`raceveil report` renders the same content as Markdown (for tickets/PRs), JSON
(tooling), or SARIF (code-scanning dashboards). Reports carry the Scope
provenance line so a reader always knows the test was authorized.

## Tone rules for all output
- Never print "exploited," "hacked," or dollar amounts of "impact." Print
  observable facts: expected vs observed counts, reproduction rate, evidence.
- Never claim completeness ("no races exist"). Claim scope ("no LIKELY+ violations
  found in the tested workflows").
- Every violation line is falsifiable by the reader via `verify`.
