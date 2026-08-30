# DB_SCHEMA.md — RaceVeil (Governance Spec)

**No database.** Per [`Context/DECISIONS.md`](../Context/DECISIONS.md)
ADR-011 (Round 2, confirmed): persistence is filesystem-only — a self-contained
per-scan run directory of JSON (finalized objects) and JSONL (append-as-you-go
streams). There is no SQL/NoSQL layer, no migration story, and none is planned
for this build.

The full on-disk layout and every entity's field-level shape (Scope, Session,
`candidate.yaml`, captured requests, Candidates, Baseline, ConcurrentTrials,
Oracle evaluation, Minimization, the `.rv` Finding/Reproduction artifact, audit
log, and derived reports) is authoritatively defined in
[`Design/DATA_MODEL.md`](../Design/DATA_MODEL.md). That document is the schema
of record; this file exists only so `docs/` has the file the governance
framework expects, and to state explicitly that "schema" here means
"run-directory file shapes," not database DDL.

Consequence for CLAUDE.md: the SQL-layer security invariant ("all queries
parameterised only") in Guide §2.2.1 has no matching surface in this codebase
and is recorded as such rather than enforced against nothing.
