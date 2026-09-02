# /audit — Diff-Scoped Security + Architecture Audit

Scope: changed + staged + unstaged + untracked files (never the whole repo,
unless there is no diff — then full-repo). Greenfield rule (CLAUDE.md §12):
**ANY finding blocks.** There is no baseline to absorb noise.

## Scanners run

```bash
golangci-lint run --new-from-rev=HEAD~1 ./...   # or full ./... if no prior commit
gosec ./...                                      # security-specific: hardcoded
                                                  # creds, SSRF-shaped code, weak
                                                  # crypto, unsafe deserialization
gocyclo -over 10 .                               # complexity covenant (CLAUDE.md §6)
go vet ./...
go test ./... -count=1 -json                     # structured pass/fail
```
If `gosec` is not yet installed, install it once (`go install
github.com/securego/gosec/v2/cmd/gosec@latest`) rather than skipping the
security scan silently — record `NO_SECURITY_SCANNER` only if installation is
genuinely blocked (e.g. no network), never silently.

## Severity normalization table (Guide §Module: audit self-healing)

| Scanner / source | Native signal | Covenant action |
|---|---|---|
| `gosec` | `HIGH` severity | block-await-human |
| `gosec` | `MEDIUM` severity | auto-remediate, re-verify |
| `gosec` | `LOW` severity | record-only (still a finding — greenfield blocks it, but auto-fix is attempted first; if not cleanly fixable, escalate to auto-remediate tier rather than silently recording) |
| `golangci-lint` (govet, staticcheck, unused, errcheck, etc. — single-severity issues) | any reported issue | auto-remediate, re-verify (these are correctness/style, not security-critical, unless the specific linter is security-focused — treat `gosec`-sourced issues per the gosec rows above even when surfaced through golangci-lint) |
| `go vet` | any reported issue | block-await-human (vet findings are almost always real bugs) |
| `gocyclo -over 10` | any function over threshold | auto-remediate (refactor per CLAUDE.md §6 patterns), re-verify |
| `go test ... -json` | any `FAIL` event | block-await-human (a failing test always blocks; never auto-"fix" a test to pass — Guide loop invariant) |
| `gofmt -l` | any file listed | auto-remediate (`gofmt -w`), re-verify |

## Self-healing failure branch

If an auto-remediation attempt (MEDIUM/LOW-tier, or gofmt/gocyclo fixes) does
not eliminate the finding on re-verify, treat it as a hard block and report to
the human — do not retry a fourth time. Apply the Guide §3.3 three-strike rule
to any auto-fix attempt: three failed fix-and-re-verify cycles on the same
finding → STOP, report the finding verbatim with all three attempts and
outcomes, await human via `AskUserQuestion`.

## RaceVeil-specific checks (beyond generic scanners)

Because this is a dual-use security tool (Design/SECURITY.md), also grep the
diff for:
- Any new outbound-request code path that does not route through
  `internal/security/scope.Guard` — block-await-human unconditionally, this
  is never auto-fixable.
- Any hardcoded credential, session token, or API key literal.
- Any code that raises `proof.max_successful_effects` handling, retries past
  a cap, or otherwise weakens the proof-cap enforcement in
  `internal/application/scheduler` — block-await-human.
- Any new capability resembling stealth/evasion/persistence/exfiltration —
  refused as out-of-charter per Design/SECURITY.md §10, not merged pending
  human review; report and stop, do not attempt to "fix."

## Output

```
=== AUDIT REPORT ===
Scope: <N files>
golangci-lint: <clean | N issues>
gosec:         <clean | N HIGH, N MEDIUM, N LOW>
gocyclo:       <clean | N functions over threshold>
go vet:        <clean | N issues>
go test:       <N passed, N failed>
gofmt:         <clean | N files>
RaceVeil checks: <clean | findings listed>
Verdict: <PASS | BLOCKED — see findings above>
```
