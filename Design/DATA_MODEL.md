# DATA_MODEL.md — RaceVeil

Persistence is **filesystem-only**. No database (DECISIONS.md): a scan is a
self-contained run directory of JSON / JSONL files. JSONL is used for
append-as-you-go streams (requests, trials, audit); JSON for finalized objects.
Entities and field meanings follow DOMAIN.md.

All schemas below are illustrative JSON shapes, not a frozen wire format; v1 will
pin them with JSON Schema. IDs are short random slugs unless noted.

## 1. On-disk layout

```
run-<timestamp>/
  config.yaml
  audit.jsonl
  requests.jsonl
  candidates.jsonl
  experiments/<candidate-id>/{baseline.json,trials.jsonl,oracle.json,minimization.json}
  findings/<finding-id>.rv
  report.{json,sarif,md}
```

## 2. Configuration & authorization

### Scope (`scope.yaml`, copied resolved into `config.yaml`)
```yaml
target: https://staging.shop.internal
authorized_by: "jane@corp — ticket SEC-1421"     # provenance/audit metadata only;
                                                  # NOT verified by RaceVeil (operator is responsible)
hosts:                                            # default-deny allowlist
  - host: staging.shop.internal
    ports: [443]
    pin_ip: 10.4.2.11                             # resolved once; anti-rebinding
    path_prefixes: ["/"]
allow_private_target: true                        # required for RFC-1918/localhost
caps:
  max_concurrency: 30
  max_requests_total: 20000
  max_rate_per_sec: 200
  max_wallclock_sec: 900
destructive:
  allow_verbs: []                                 # DELETE etc. blocked by default
  deny_paths: ["/admin/**", "/users/*/delete"]
proof:
  max_successful_effects: 2                       # SAFETY CEILING on effects RaceVeil may cause;
                                                  # must be >= an invariant's required_proof_effects
                                                  # (L+1 for max_successes) or that invariant is unprovable
```

### Session / auth state (`session.json`)
```json
{
  "id": "sess_attacker",
  "cookies": [{"name":"session","value":"<redacted>","domain":"...","path":"/"}],
  "headers": {"Authorization": "<redacted>"},
  "csrf": {"source":"response_body","selector":"input[name=csrf]","current":"<redacted>"},
  "refresh": {"recipe":"login.yaml"},
  "isolation_group": "attacker"
}
```
Secrets are redacted in every persisted artifact except the live in-memory
Session; see SECURITY.md.

### User invariants (`invariants.yaml`, optional, Oracle level 5)
```yaml
- match: {method: POST, path: /coupon/redeem}
  invariant: {type: max_successes, value: 1}       # no window => lifetime-scoped
  success_when: {status: 200, body_contains: "redeemed"}
  reject_when:  {status: 409}
  post_state_probe: {method: GET, path: /account/redemptions, extract: "$.redemptionCount"}

# Windowed form (rate-limit bypass) — same type, optional window; NOT a new type:
- match: {method: POST, path: /login}
  invariant: {type: max_successes, value: 5, window: {duration: 60s}}  # <=5 successes / fixed 60s window
  success_when: {status: 200}
  reject_when:  {status: 429}
```

### MVP candidate input (`candidate.yaml`, the manual-candidate path)
A Candidate = Workflow + Invariant, supplied directly by the operator. This is the
**MVP `scan --candidate` input**; no crawling/discovery is involved.

**Phase-1 contract (strict, minimal).** The MVP validates `candidate.yaml`
strictly against a fixed schema — no flexible/extensible schema system. Malformed
or unsupported candidates are **rejected with a clear error**, not silently
coerced. For Phase 1:
- **Required:** `workflow.requests` (≥1), `workflow.act_request` (must name one of
  those requests), `invariant` with `type: max_successes` and `value`, and
  `session_ref`.
- **Optional:** `setup_requests`, `success_when`, `reject_when`,
  `post_state_probe`, `reset_recipe`, `invariant.window`.
- **Rejected in Phase 1:** invariant types other than `max_successes`
  (`uniqueness`/`monotonic_limit`/`single_transition` are accepted only once
  implemented — see ROADMAP); unknown fields; an `act_request` not present in
  `requests`.
```yaml
# Candidate = Workflow + Invariant
workflow:
  requests:                                        # ordered; last is the "act" request
    - id: add_coupon
      method: POST
      url: /cart/coupon
      headers: {content-type: application/json}
      body: '{"code":"${COUPON_CODE}"}'
      bindings:                                     # variable/session bindings
        - {name: csrf, in: header, from: session.csrf}
        - {name: COUPON_CODE, in: body, from: literal:SAVE10}
  act_request: add_coupon                           # the ONLY request replicated in the burst
setup_requests: []                                  # optional; run once per trial, not replicated
session_ref: session.json                           # Session/auth to run under
invariant: {type: max_successes, value: 1}          # optional window: {duration: 60s} (fixed window)
success_when: {status: 200, body_contains: "redeemed"}   # optional; else inferred from baseline
reject_when:  {status: 409}                              # optional; else inferred from baseline
post_state_probe:                                        # optional (Level-4 corroboration)
  {method: GET, path: /account/redemptions, extract: "$.redemptionCount"}
reset_recipe:                                            # optional; EXECUTABLE — enables independent trials
  {kind: fresh_code, setup_ref: add_coupon, how: "issue new single-use code per trial"}
```
`reset_recipe.kind ∈ {fresh_code, reset_endpoint, fixture_command, setup_recipe}`
(DOMAIN.md). It must be **executable** by RaceVeil — `verify`/`replay` run it to
obtain fresh state, so a non-executable "recipe" does not grant independence.
**MVP supports only `fresh_code` and `setup_recipe`** (re-running a supplied
setup); `reset_endpoint`/`fixture_command` are v1.

Fields beyond `workflow` + `invariant` are optional: absent `success_when`/
`reject_when` are learned from the Baseline; absent `post_state_probe` means
Level-4 corroboration is unavailable (so CONFIRMED needs a Level-2 body
differential instead); absent `reset_recipe` means trials are `unknown`/
`dependent` and Confidence is capped at LIKELY.

## 3. Captured request (`requests.jsonl`, one per line)
```json
{
  "id": "req_8kx2",
  "endpoint": {"method":"POST","path":"/coupon/redeem"},
  "url": "https://staging.shop.internal/coupon/redeem",
  "headers": {"content-type":"application/json"},
  "body": "{\"code\":\"SAVE10\"}",
  "session_id": "sess_attacker",
  "bindings": [                                   // dynamic values resolved at send
    {"name":"csrf","in":"header","from":"session.csrf"},
    {"name":"code","in":"body","from":"literal:SAVE10"}
  ],
  "http_version_seen": "h2",
  "source": "import|crawl"
}
```

## 4. Workflow & Candidate (`candidates.jsonl`)
Every Candidate — whether supplied via `candidate.yaml` (MVP) or produced by
discovery (v1) — is normalized to this one representation. `source` distinguishes
origin; `score` is `null` for declared candidates.
```json
{
  "id": "cand_redeem",
  "source": "declared",                            // declared (candidate.yaml) | inferred (discovery)
  "score": null,                                   // null when source=declared; a number when inferred
  "workflow": {
    "id": "wf_redeem",
    "requests": ["req_addcart","req_addcoupon"],   // ordered; setup...act
    "act_request": "req_addcoupon",                // the ONLY request replicated in the burst
    "setup_requests": ["req_addcart"],             // run once per trial; not replicated
    "bindings_graph": [{"from":"req_addcart.$.cartId","to":"req_addcoupon.cartId"}]
  },
  "invariant": {"type":"max_successes","value":1,"source":"declared"},
  "score_signals": null                            // populated only when source=inferred
}
```
Discovery-origin example differs only in the origin fields:
`"source":"inferred"`, `"score":0.86`, `"invariant.source":"inferred"`, and a
populated `score_signals` (e.g. `{"verb":"POST","name_hits":["coupon","redeem"],
"idempotency_key_absent":true,"limit_observed_in_probe":true}`).

`source ∈ {declared, inferred}`; `invariant.source ∈ {declared, inferred}`. For
inferred candidates, if no invariant could be established, no Candidate row is
written (the Workflow is discarded at ranking). Declared candidates always get a
row. `act_request` identifies the request replicated concurrently; `setup_requests`
run once per trial and are bound into each Act instance (DOMAIN.md Act Request).

## 5. Baseline (`experiments/<id>/baseline.json`)
```json
{
  "candidate_id": "cand_redeem",
  "runs": 3,
  "expected_success_count": 1,
  "success_signature": {"status":[200],"body_markers":["redeemed"],"extractors":{"redemptions":"$.redemptionCount"}},
  "reject_signature":  {"status":[409],"body_markers":["already applied"]},
  "observables_baseline": {"redemptions":{"seq":[1,1,1],"variability":"none"}},
  "classifier_separation": "clean",               // clean|weak — caps confidence if weak
  "baseline_state_reset": "fresh_code",           // how each baseline run got fresh state
                                                  //   (calibration only; NOT part of K_independent)
  "reset_recipe": {"kind":"fresh_code","how":"issue new single-use code per run"}
}
```

## 6. Concurrent trials (`experiments/<id>/trials.jsonl`)
```json
{
  "trial": 4,
  "N": 2,
  "sync_strategy": "H2SinglePacket",
  "interarrival_dispersion_ms": 0.7,             // how tight the burst was
  "state_independence": "independent",           // fresh state this trial
  "responses_summary": {"success": 2, "reject": 0, "error": 0},
  "success_count_S": 2,                           // required_proof_effects = L+1 = 2 reached; do not escalate
  "violation": true,                              // S > L (L=1)
  "observables": {"post_state_redemptions": 2, "distinct_receipt_ids": 2}
}
```

Each trial records its own `state_independence`. The Experiment's aggregate
independence is derived as the **weakest** trial value that RaceVeil relies on,
and only trials whose value is `independent` are counted into the oracle's `K`/`r`
(below); `dependent`/`unknown` trials are kept as Evidence but excluded from the
statistics.

## 7. Oracle evaluation (`experiments/<id>/oracle.json`)
```json
{
  "candidate_id": "cand_redeem",
  "invariant": {"type":"max_successes","value":1},
  "required_proof_effects": 2,                     // L+1; must be <= scope proof.max_successful_effects
  "primary_oracle_levels": [3],                    // Level 3 = cross-request consistency (the violation)
  "levels_evaluated": [1,3,4],
  "trials": {"K_independent": 6, "r_violations": 5},   // independent reproduction trials only
  "independent_trials": 6, "dependent_or_unknown_excluded": 0,   // baseline K_b is separate, never folded in
  "reproduction": {"p_hat": 0.833, "wilson95": [0.436, 0.970]},  // computed by Wilson formula, not copied
  "state_independence": "independent",             // required for CONFIRMED
  "confidence_bands_version": "provisional-v0",    // thresholds pending corpus calibration
  "corroborating_observables": ["post_state"],     // Level 2 or 4 only; Level 3 is primary, not corroborating
  "confidence": "CONFIRMED",                        // Level-3 violation + >=1 corroborating (Level 4) + independent
  "severity": {"label":"HIGH","basis":"duplicate-effect integrity violation"},
  "why": "S>L in 5/6 independent trials; corroborated by Level-4 post-state (2 redemptions recorded vs expected 1)"
}
```

## 8. Minimization (`experiments/<id>/minimization.json`)
```json
{
  "concurrency": {"start_N":8, "minimal_N":2, "min_repro":{"r_violations":5,"K_independent":6,"wilson_lo":0.436}},
  "workflow": {"start_requests":["req_addcart","req_addcoupon"],
               "minimal_requests":["req_addcoupon"]},
  "method": "decreasing-sweep + delta-debugging with re-verification"
}
```

## 9. Finding / Reproduction artifact (`findings/<id>.rv`)
The `.rv` is a **single self-contained JSON bundle** — everything needed to
re-run without the rest of the run directory. Consumed by `replay`/`verify`.
```json
{
  "rv_version": "1",
  "finding_id": "find_redeem_01",
  "target": "https://staging.shop.internal",
  "scope_ref": {"authorized_by":"jane@corp — SEC-1421"},  // provenance/audit only; not verified
  "workflow": { "...": "minimized requests + bindings, secrets templated as ${ENV}" },
  "session_bootstrap": {"recipe":"login.yaml","isolation_group":"attacker"},
  "invariant": {"type":"max_successes","value":1,"source":"inferred"},
  "required_proof_effects": 2,
  "sync_strategy": "H2SinglePacket",
  "concurrency_N": 2,
  "oracle": {"primary_levels":[3],"corroborating_levels":[4],"success_signature":{"...":"..."},
             "reject_signature":{"...":"..."},"post_state_probe":{"...":"..."}},
  "expected": {"max_successes":1},
  "state_independence": "independent",
  "reset_recipe": {"kind":"fresh_code","how":"issue new single-use code per trial"},
  "observed_at_creation": {"S":2,"reproduction":{"r_violations":5,"K_independent":6,"wilson95":[0.436,0.970]},
                           "confidence_bands_version":"provisional-v0"},
  "confidence": "CONFIRMED",
  "severity": {"label":"HIGH","basis":"duplicate-effect integrity violation"},
  "evidence_refs": {"run_dir":"run-2026-08-28T.../experiments/cand_redeem"},
  "created": "2026-08-28T10:00:00Z",
  "rng_seed": 4815162342
}
```
The `.rv` **never stores live secrets**; credentials are templated
(`${RACEVEIL_SESSION_TOKEN}`) and re-sourced at replay from env or the auth
recipe. This makes findings shareable without leaking auth.

## 10. Audit log (`audit.jsonl`)
Append-only, one entry per outbound request: timestamp, method, host, path
(query redacted by rule), session isolation group, experiment id, response
status, byte counts. Purpose: accountability and post-hoc scope verification, not
analysis. Never contains bodies with secrets.

## 11. Report (`report.{json,sarif,md}`)
Derived, regenerable views over findings. SARIF for CI/code-scanning surfaces;
JSON for tooling; Markdown for humans. No new data — a projection of `findings/`.

## 12. Retention & size
Full Evidence (baselines, all trials) is retained for skeptical re-derivation.
Bodies are truncated past a configurable size and secret-redacted. A `--minimal`
flag keeps only findings + `.rv` for space-constrained CI.
