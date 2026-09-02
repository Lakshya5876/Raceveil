# RaceVeil — Phase 2 (MVP Validation / Thesis Proof) — Build Report

**Branch:** `chore/init-governance-scaffold`
**Commits this phase:** `89956c8` → `c3f3d7f` (5 commits, on top of Phase 1's `c1f690d`)
**Pushed anywhere:** **No.** No `git push` was run; `main` untouched.

```
c3f3d7f feat(corpus): wire a real Level 4 post-state probe into the coupon archetype
b65ddea feat(corpus): add Spring Boot cross-stack subset — 18/18 corpus detection
f5efa5a feat(corpus): add Node+Postgres seeded corpus; fix baseline calibration bug
89956c8 feat(phase2): implement Oracle Level 2/4 corroboration, all invariant types, minimization
```

## Cost / infrastructure — zero, entirely local

Everything in this phase runs on this machine only, with no billing surface
anywhere:
- **Postgres**: one local Docker container (`postgres:16-alpine`), bound to
  `127.0.0.1:55432` only — never exposed beyond localhost. Docker Desktop
  itself is free for individual use.
- **Node/Express corpus app**: local process, `127.0.0.1:4000`. Dependencies
  (`express`, `pg`) came from the free public npm registry.
- **Spring Boot corpus app**: local process, `127.0.0.1:4100`. Dependencies
  came from the free public Maven Central repository.
- No cloud provider, no managed database, no API keys, nothing deployed.

## What Phase 2 asked for (ROADMAP.md) vs. what's built

| Requirement | Status |
|---|---|
| Seeded vulnerable apps, Spring Boot + PostgreSQL **and** Node + PostgreSQL | **Done** — Node: all 6 archetypes; Spring Boot: 3 of 6 (see "Not done" below) |
| Synchronized-safe twin for every archetype | **Done** — every vulnerable endpoint has a twin using either an atomic `UPDATE...WHERE` or an explicit `SELECT...FOR UPDATE` transaction |
| Oracle Level 2 (body-differential) corroboration | **Done and proven** — every corpus archetype's response carries a `receipt_id`; distinct values across concurrent successes corroborate the Level-3 count |
| Oracle Level 4 (post-state probe) corroboration | **Done and proven** — a time-windowed `GET /coupon/redemptions` probe on the Node coupon archetype; `coupon-vuln` reaches CONFIRMED via Level 2 **and** 4 together |
| Corroboration rule (CONFIRMED needs ≥1 of Level 2/4) | **Done** — was already structurally guarded in Phase 1; now genuinely reachable and reached |
| Remaining invariant types (`uniqueness`, `monotonic_limit`, `single_transition`) | **Done** — all three exercised for real against the corpus (signup, inventory, order archetypes) |
| Minimization (concurrency + workflow) | **Done** — decreasing-sweep concurrency search + greedy Setup-request delta-debugging, both real (not simulated) against every corpus finding |
| ≥90% detection at LIKELY+, 0 CONFIRMED on safe twins | **Exceeded** — 100% detection **at CONFIRMED** (not just LIKELY+), and safe twins produced **zero findings at any confidence**, not just "capped below CONFIRMED" |

## The corpus result

18 scans, 18 correct — every vulnerable endpoint reached **CONFIRMED** with a
real corroborating observable, every safe twin came back **clean**:

| Archetype | Invariant type | Node | Spring Boot |
|---|---|---|---|
| Coupon redemption | `max_successes` | vuln→**CONFIRMED** (Level 2+4) · safe→clean | vuln→**CONFIRMED** (Level 2) · safe→clean |
| Signup uniqueness | `uniqueness` | vuln→**CONFIRMED** (Level 2) · safe→clean | — |
| Gift-card redemption | `max_successes` | vuln→**CONFIRMED** (Level 2) · safe→clean | — |
| Inventory overrun | `monotonic_limit` | vuln→**CONFIRMED** (Level 2) · safe→clean | vuln→**CONFIRMED** (Level 2) · safe→clean |
| Login rate-limit bypass | `max_successes` (windowed pattern) | vuln→**CONFIRMED** (Level 2) · safe→clean | — |
| Order confirmation | `single_transition` | vuln→**CONFIRMED** (Level 2) · safe→clean | vuln→**CONFIRMED** (Level 2) · safe→clean |

Minimization settled on the smallest possible burst in every case: **N=2**
for every archetype except the rate-limited login archetype (`N=4`, the
mathematical minimum since its invariant permits 3 successes — `S>3`
literally cannot happen at `N<4`). Raw scan output for every cell above is
in `corpus/results/` (Node) and `corpus/results-spring/` (Spring Boot).

### A real bug the corpus caught

`runBaseline` was using `required_proof_effects` to decide how many
sequential calls to make during calibration. That's correct for
`max_successes` (where it equals `L+1`), but `monotonic_limit`'s
`required_proof_effects` is a constant `1` regardless of the bound — so
calibration only ever called the endpoint once, never saw a rejection, and
reported "weak" classifier separation, which capped **every**
`monotonic_limit` finding at SUSPECTED even with 3/3 perfect reproduction.
Fixed to use `Invariant.Value + 1` (the number that actually demonstrates
the limit sequentially), which is a different quantity from
`required_proof_effects` in general. Covered by a new regression test
(`TestRunBaseline_MonotonicLimit_ClassifierSeparationIsClean`).

## What's explicitly not done (honest gaps, not hidden)

- **Spring Boot only covers 3 of 6 archetypes** (coupon, inventory, order —
  one per major evaluation path: plain `UPDATE...WHERE`, `SELECT...FOR
  UPDATE`, and a second plain-update case). Signup/uniqueness, gift-card,
  and login weren't reimplemented in Spring Boot. The cross-stack claim
  ("not overfit to one framework's idioms") is real but narrower than a
  full 6×2 matrix.
- **No second isolation-level/lock-config variant per archetype.** TEST_PLAN
  suggests "two isolation levels... where relevant" per archetype (e.g. a
  naive `READ COMMITTED` fix alongside a `SELECT...FOR UPDATE` fix for the
  *same* archetype). This build has one vulnerable + one safe version each,
  not multiple safe variants — though two genuinely different safe
  techniques (atomic `UPDATE...WHERE` and explicit `FOR UPDATE`
  transactions) are both exercised across the corpus.
- **Level 4 probe only wired for one archetype** (coupon, Node). The
  mechanism is generic and archetype-agnostic in the scheduler; adding
  probes to the rest is direct but not yet done.
- **No empirical threshold recalibration** (TEST_PLAN Layer 4's "produce a
  precision/recall curve, replace provisional-v0"). The provisional-v0 bands
  scored 18/18 clean here, so there's no evidence yet that recalibration is
  even needed, but a 6-archetype/2-stack corpus is a small sample for that
  claim — that's explicitly Phase 4 scope in ROADMAP.md, not attempted.
- **Layers 7-9 of TEST_PLAN** (PortSwigger Academy, real OSS CVE
  rediscovery, published benchmark) are Phase 5 and were not started — they
  need live external network access this build deliberately never took.

## Verification

```
$ go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./... && gocyclo -over 10 . && gosec -quiet ./...
(all clean, 0 issues)

$ go test ./... -count=1 -cover
(all packages pass; scheduler 80%+, oracle 92%+, minimize 100%)
```

Every commit above went through the real pre-commit hook (full lint +
build + test), same as Phase 1.

## Still running locally (free, your call whether to stop them)

- Postgres container: `docker compose -f corpus/docker-compose.yml down`
  (from the `corpus/` directory) to stop and remove it.
- Node corpus app and Spring Boot corpus app: both running as background
  local processes; find with `netstat -ano | Select-String "4000|4100"` and
  stop with `taskkill /PID <pid> /F` if you want them down.

Nothing else was left running or scheduled.
