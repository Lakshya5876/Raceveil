# /prep — Natural Language → Execution Contract

Zero implementation. Converts a vague task description into the Guide §1.3
execution-contract shape, scoped to this repo's actual layout. Hard stops are
flagged at the top, before the rest of the contract, using `AskUserQuestion`
per CLAUDE.md §8 if any hard-stop trigger (CLAUDE.md §2.2 table) applies.

## Output shape

```
HARD STOPS (if any): <trigger from CLAUDE.md §2.2, or "none">

SCOPE:       <explicit dirs/files — e.g. internal/domain/,
              internal/application/oracle/, testdata/oracle/>
OBJECTIVE:   <the condition that must be TRUE when done — an observable
              outcome, not a vague verb>
CONSTRAINTS: <rules that cannot be broken — e.g. "no new go.mod dependency",
              "internal/domain stays zero-dependency", "does not touch
              internal/security/scope without a hard-stop conversation">
VERIFY:      <exact deterministic command(s) — e.g.
              "go test ./internal/application/oracle/... -count=1 -v">
OUTPUT:      File table + full test output + Conventional Commit message
```

## Rules
- Never guess an ambiguous requirement into the contract — if the task text
  leaves two reasonable readings, surface both via `AskUserQuestion` before
  writing SCOPE/OBJECTIVE, rather than picking one silently.
- If the task touches a CORE_FILES glob (CLAUDE.md §4), say so explicitly in
  HARD STOPS even before the human replies — the contract itself is the
  moment to flag it, not Phase 3 of `/feature`.
- Do not write, edit, or run anything beyond producing this contract.
