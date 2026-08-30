# RaceVeil

A black/grey-box concurrency-integrity verification tool: it tests running
web applications for observable integrity violations caused by concurrent
requests (duplicate coupon redemption, inventory overrun, rate-limit bypass,
...) and produces reproducible, minimized evidence for each one.

See [`Design/PRD.md`](Design/PRD.md) for the full problem statement and
[`docs/PRD.md`](docs/PRD.md) for this build's scoped requirements (MVP =
Phases 0–2, [`Context/ROADMAP.md`](Context/ROADMAP.md)).

## Quickstart

```bash
git clone https://github.com/Lakshya5876/Raceveil.git
cd Raceveil
git config core.hooksPath .githooks   # activate the governance hooks in this clone
go build ./...
go test ./... -count=1
```

This repository is governed by [`CLAUDE.md`](CLAUDE.md) — read it before
making any change. `git commit` and `git push` run the mechanical covenant
(`.githooks/pre-commit`, `.githooks/pre-push`) automatically; it cannot be
skipped except via the audited `SKIP_COVENANT=1` bypass documented in the
Guide, and that path requires an interactive terminal and a typed reason.

## Layout

```
cmd/raceveil/            CLI entrypoint (thin)
internal/domain/         entities (Design/DOMAIN.md)
internal/application/    orchestration: scheduler, oracle, minimize (+ discovery/ranking, Phase 3)
internal/infrastructure/ I/O: sync engine, filesystem persistence (+ crawler, Phase 3)
internal/presentation/   Cobra CLI commands
internal/security/scope/ the Scope/Authorization guard chokepoint
internal/config/         single config-access module
internal/wiring/         DI wiring
Design/                  full product specification (frozen for Phase 0-2)
Context/                 architecture decisions + roadmap
docs/                    governance-scoped spec derivatives (PRD/TRD/etc.)
```

## Status

Pre-Phase-0: scaffold only, no domain logic implemented yet. See
[`Context/ROADMAP.md`](Context/ROADMAP.md) for the build sequence.
