# SECURITY.md — RaceVeil

RaceVeil is dual-use. Its entire design intent is **verification, not
weaponization**: prove that a concurrency integrity violation exists, then stop.
This document is normative — the behaviors here are requirements, tested in
TEST_PLAN.md §Scope/Safety, not aspirations.

## 1. The core distinction

| Verification (RaceVeil does this) | Weaponization (RaceVeil refuses) |
|---|---|
| Prove `S > L` once, with minimal effect | Drain a balance, mass-redeem, mint resources |
| Read-only post-state probe to corroborate | Modify/delete to maximize impact |
| Stop at `proof.max_successful_effects` | Repeat for real-world gain |
| Emit reproducible evidence | Emit stealthy, persistent, or evasive exploits |
| Operate only within an authorized Scope | Pivot, escalate, exfiltrate |

RaceVeil optimizes every design choice toward the left column. There is no
command, flag, or mode that implements the right column.

## 2. Authorized-use model
- A scan **requires** a Scope file that names an `authorized_by` provenance
  string; it is printed in every confirmation and report. This string is
  **provenance/audit metadata, not proof of authorization** — RaceVeil cannot
  verify (cryptographically or organizationally) that the named ticket/person
  actually authorizes the test. The operator remains responsible for ensuring
  authorization genuinely exists.
- What RaceVeil actually guarantees is **technical enforcement** of the Scope
  (below), not a judgment that testing is permitted. It enforces where you may
  send traffic; it does not decide whether you *should*.
- **Default-deny**: only hosts/ports/path-prefixes explicitly listed are
  touchable. Everything else is refused before a packet is sent.
- Public targets: in `--safe-mode` (default) RaceVeil refuses targets that
  resolve to public IPs unless the operator passes `--i-am-authorized` **and**
  the Scope explicitly lists them. This is a friction/technical gate to prevent
  accidental scanning — **not** a determination that the target is authorized.
  There is intentionally no frictionless "scan any URL" path.

## 3. Scope enforcement (the `scope.Guard` chokepoint)
Every outbound request passes one guard that enforces, in order:
1. Host/port/path allowlist (default-deny).
2. **IP pinning**: each authorized host is resolved once at scope setup and the
   IP is pinned for the run (§4).
3. Private/loopback/link-local/metadata-IP block: `127.0.0.0/8`, `::1`,
   `10/8`, `172.16/12`, `192.168/16`, `169.254/16` (incl. `169.254.169.254`
   cloud metadata), `fd00::/8` — **blocked unless** the target is an explicitly
   declared private/localhost target (`allow_private_target: true`), which is the
   normal case for local test apps but must be deliberate.
4. Per-scope caps: max concurrency, max total requests, max rate, max wallclock.
5. Destructive-verb rules (§6).
A request failing any check is dropped and logged; the guard cannot be bypassed
by discovery, redirects, or replay.

## 4. SSRF / DNS-rebinding / redirects
- **DNS rebinding**: resolve authorized hosts once, pin the IP, and dial the
  pinned IP with SNI/Host set to the hostname. A later DNS answer pointing
  elsewhere cannot redirect traffic to an unauthorized host mid-run.
- **Redirects**: not followed automatically across scope. A 3xx to an
  out-of-scope host/path is recorded as an Observable but never fetched. Same-host
  in-scope redirects may be followed up to a small cap.
- **SSRF surface**: because RaceVeil only ever dials pinned, in-scope IPs, it
  cannot be steered into internal metadata or lateral targets by target-controlled
  redirects or hostnames.

## 5. Credential & secret handling
- Session secrets (cookies, bearer tokens, CSRF) live **in memory** during a run
  and in the `session.json` the operator supplies; they are **redacted** in
  `config.yaml`, `audit.jsonl`, trials, oracle output, reports, and `.rv`
  findings.
- `.rv` artifacts **template** secrets as `${ENV}` placeholders re-sourced at
  replay, so findings are shareable without leaking auth.
- **Cross-target credential leakage** is prevented structurally: a Session is
  bound to its Scope's hosts; the guard refuses to attach a Session's credentials
  to a request for any other host. `verify`/`replay` refuse if the current Scope
  doesn't cover the finding's target.
- **Session isolation groups**: multi-account tests (attacker/victim) keep
  credentials in separate groups; the engine never mixes cookies/tokens across
  groups within one Workflow instance.

## 6. Destructive-action controls
- Mutating verbs are allowed (the tool tests state changes), but **DELETE and
  configured high-risk paths are blocked by default**. Enabling them requires an
  explicit `destructive.allow_verbs` entry in Scope and is surfaced in the
  pre-scan confirmation.
- `deny_paths` globs (e.g. `/admin/**`) are hard-blocked regardless.
- The **proof cap** distinguishes two quantities:
  (1) `required_proof_effects` — the *minimum* successful effects needed to
  establish a violation for the specific Invariant (`L + 1` for
  `max_successes(L)`; two same-key resources for `uniqueness`; one boundary-
  crossing effect for `monotonic_limit`; a twice-fired transition for
  `single_transition` — see DOMAIN.md); and
  (2) `proof.max_successful_effects` — the operator's **safety ceiling** on how
  many successful **state-changing workflow effects RaceVeil may cause** during
  proof. **Read-only observations do not consume this budget** — a Level-4
  post-state probe is a safe/read-only request and never counts as a caused
  effect (causing an effect and observing one are not the same thing).
  RaceVeil stops as soon as `required_proof_effects` is reached and **never
  continues to maximize effect**; `replay` honors the same ceiling. The example
  value `2` is correct for `max_successes(L=1)` and is not a universal constant.
- If `required_proof_effects > proof.max_successful_effects` (e.g. proving
  `max_successes(L=5)` needs 6 but the ceiling is 2), RaceVeil **refuses the
  Experiment as unprovable-under-cap** rather than exceeding the ceiling. The
  safety ceiling always wins over the desire to prove.

## 7. Proving without maximizing damage
- Prefer **synthetic/disposable state**: the recommended workflow uses test
  coupons, disposable accounts, and fixtures with synthetic balances (the seeded
  corpus is built this way). Documentation steers users here first.
- Against real authorized targets, the proof cap and minimization mean RaceVeil
  demonstrates the invariant break with the **smallest** number of successful
  effects, not the largest. A CONFIRMED finding needs only a handful of duplicate
  successes plus a read-only post-state probe.
- Post-state probing is restricted to Requests classified **read-only** (safe
  methods / user-declared read probes). RaceVeil will not "verify" state by
  mutating it further.

## 8. Denial-of-service prevention
- Concurrency/rate/total caps are enforced at the guard and scheduler; the
  concurrency ladder never exceeds Scope caps.
- One experiment at a time by default; warm-up bursts are counted against caps.
- A global wallclock and request budget bound total load; Ctrl-C drains.
- RaceVeil is not a stress tool; bursts are sized to line up race windows
  (tens of requests), not to exhaust the server.

## 9. Local artifact security
- Run directories may contain sensitive response bodies; bodies are
  secret-redacted by rule and truncated past a size cap. `--minimal` keeps only
  findings + `.rv`.
- Artifacts are written with restrictive file permissions (`0600`/`0700`).
- The audit log is append-only and records every request for post-hoc scope
  verification.

## 10. Explicitly excluded capabilities (never built)
No stealth or evasion (no WAF-bypass, no traffic obfuscation, no timing
concealment); no persistence; no credential theft or harvesting; no automated
privilege escalation; no lateral movement; no automated destructive exploitation;
no data exfiltration; no exploit-chain assembly. RaceVeil produces **proof and
evidence**, and stops. Feature requests in these directions are refused as
out-of-charter (DECISIONS.md).

## 11. Responsible disclosure posture
Documentation instructs users to test only authorized targets, to prefer staging
with synthetic state, and to report real findings through the target's disclosure
program. The `.rv` format is designed to hand a maintainer a clean, minimal,
reproducible report — the artifact of verification, not an exploit kit.
