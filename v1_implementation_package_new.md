# CLAUDE CODE — WORKSPACE INITIALIZATION PACKAGE
## Edition: NEW PROJECTS (Greenfield)

**Companion document:** `v1_claude_code_development_guide_new.md` (the Guide)
**Time required:** ~30-45 minutes, once at project birth — this now includes a real
interrogation (6 rounds) and a spec-document review/approval step, not just a
single prompt-and-done exchange.
**Outcome:** A repository born governed — prescriptive architecture, hook-enforced
covenants from commit #1, zero debt ever — plus an approved PRD/TRD/DB schema/user
flows/system design under `docs/` that the scaffold is built to match.

> **Status: V1 production-ready.** Specification verified through 3 audit rounds and mechanically tested: pre-commit fingerprint receipts, coverage threshold, CORE_FILES tier-3 escalation, identity-based debt ratchet, CI backstop, and IDE extension crash guard all confirmed functional. Install and proceed.

---

## STEP 1 — PRE-FLIGHT (human developer action)

1. Create the repository and make an initial commit (even an empty README is
   fine): `git init && git commit --allow-empty -m "chore: repository birth"`
2. Create a setup branch: `git checkout -b chore/claude-init`
3. Clone the governance framework and run the installer from within this repository's root:
   ```bash
   git clone https://github.com/Lakshya5876/Covenant ~/Covenant
   ~/Covenant/CovenantMac/install.sh
   ```
   (On Windows: `& '~\Covenant\CovenantWin\install.ps1'` from PowerShell.)
   To remove all framework traces if you want:
   ```bash
   ~/Covenant/CovenantMac/uninstall.sh
   ```
4. Decide three things BEFORE running the init (the prompt will ask):
   - **Stack:** language + framework (e.g. Python/FastAPI, TS/Node/Express,
     Go/chi)
   - **Persistence:** database/store, if known (it can be added later via the
     hard-stop process)
   - **Test framework + linter + type checker** for that stack
5. **Permission-mode check:** never run the init (or any session in a governed
   repo) with `--dangerously-skip-permissions`. The generated settings pin
   `defaultMode: "default"`; that pin only protects you if you don't override
   it at the CLI.

## STEP 2 — INITIALIZATION (run /init-governance)

Open Claude Code at the repository root and run:
```
/init-governance
```
install.sh already generated this command from the exact prompt below — no file
to open, no copy-paste. (If you're on an install that predates this command, or
you'd rather read the prompt first: paste the entire marked prompt section below
verbatim as your first message instead — same content, same result.)

Unlike the brownfield edition there is no existing codebase to discover — instead
the prompt runs 6 rounds of mandatory questions (product/domain, stack,
operational reality, risk posture, debt philosophy, CORE_FILES confirmation),
then drafts a full set of spec documents (PRD, TRD, DB schema, user flows,
system design, architecture decisions) under `docs/` and asks you to explicitly
approve them. Nothing gets scaffolded or written until that approval — expect a
real back-and-forth, not a single prompt-and-done exchange.

---------------------------------- PROMPT START ----------------------------------

Read the file `v1_claude_code_development_guide_new.md` in this repository's root.
It is the engineering standard this initialization implements — internalize
Sections 2 (configuration), 4 (stateful layer + enforcement hooks), 5 (covenants),
6 (testing discipline), and SECTION 2.5 (cognitive routing + execution covenants)
before doing anything.

CRITICAL: SECTION 2.5 rules are NON-NEGOTIABLE and fire on EVERY task in this
governed repository, starting immediately after initialization:

SECTION 2.5 — COGNITIVE ROUTING & EXECUTION COVENANTS (Always Active)

  2.5.1 — MODEL INTERCEPT (Cognitive Routing)
    Every task triggers automated model selection logic BEFORE execution:
    - Analyze task type: (architecture | security | performance | test | deploy)
    - Route to appropriate model if available; fallback to primary if not
    - This routing is NON-BYPASSING — the agent cannot override it
    - Task routing decisions are logged in covenant_state.json ledger

  2.5.2 — EXECUTION MODE DECLARATION (Menu Covenant)
    Before ANY file is written, the agent MUST declare ONE of:
      [ ] MUST OUTPUT: task is design/analysis/report — no code changes, only text
      [ ] HARD STOP: task requires human approval before proceeding (new dep,
          auth change, schema change, CORE_FILES edit, settings change)
      [ ] EXECUTE: task is code change — proceed with full pipeline
    Wrong declaration blocks the task. The agent cannot change mode mid-task.

  2.5.3 — GRAPH MEMORY PROTOCOL (Tool Invocation Rules)
    Every tool call is context-aware and logged:
    - Read tools: logged once per file per session (avoid re-reads)
    - Bash commands: cached stdout for 5-minute reuse window
    - Edit/Write tools: require prior Read call (trust-root rule)
    - Web/External tools: only after exhausting local codebase tools
    - If tool call is repetitive (same call within 10 mins): STOP and analyze
    - Graph memory (what the agent knows about the codebase) expires at
      session boundary; new session = fresh recon

  2.5.4 — GITFLOW BRANCH ENFORCEMENT (Hard Block on Protected Branches)
    These rules are MECHANICAL, never conditional:
    - BLOCK HARD: any git push to main / master / develop (pre-push hook refuses)
    - BLOCK HARD: git commit on main / master / develop (feature branch mandate)
    - Feature branch naming: feature/*, fix/*, docs/*, chore/* only
    - Pre-push hook verifies branch protection BEFORE auth/network checks
    - User cannot override; no --force, no --no-verify, no exceptions
    - Protected-branch bypass requires human-authored PR + code review
    - Protected branch = main, master, develop, production, release/*

  2.5.5 — TOKEN HARNESS & COST AWARENESS (Hard Blocks at Budget Thresholds)
    Token budgets are PER-TASK, non-negotiable:
    - Task token budget: 200,000 tokens (default, specified in system prompt)
    - Soft warning at 70% (140,000 tokens): agent must summarize & checkpoint
    - Hard block at 100% (200,000 tokens): execution STOPS immediately
    - History retransmission waste (repeated passages) hard block at 50%
      - If retransmitted content exceeds 50% of budget, task halts
    - When hard block fires: state what is known, what remains unfinished,
      write all checkpoints, then exit (never continue on next token window)
    - Token budget resets per task; cannot carry over or negotiate up
    - Cost warnings MUST be emitted to user at 80%, 90%, 100%

  2.5.6 — EXECUTION PIPELINE INVARIANTS (Always Fire, Never Bypass)
    Every code-change task auto-triggers this pipeline without confirmation:
    Step 1: RECON (read-only) — grep, locate, scope the change
    Step 2: CONTRACT (internal) — SCOPE/OBJECTIVE/CONSTRAINTS/VERIFY/OUTPUT
    Step 3: EXECUTE — write code, run tests, verify covenants
    Step 4: OUTPUT — change manifest, test results, commit message
    
    Hard stops within pipeline: new dependency, auth change, schema change,
    CORE_FILES edit, settings change, permission-mode change, hook modification.
    Agent states the hard stop, waits for human approval, cannot self-resolve.

  2.5.7 — VS CODE / IDE EXTENSION CRASH GUARD (Non-Bypassing Environment Rule)
    Before outputting the Execution Mode Menu (2.5.2/2.5.3) or blocking execution
    on an interactive human choice, the agent MUST check whether it is running
    inside a non-TTY or IDE-extension terminal wrapper (e.g. `[ -t 0 ]` is false,
    or extension env flags such as TERM_PROGRAM=vscode, VSCODE_PID,
    VSCODE_GIT_IPC_HANDLE, CURSOR_*, or a JetBrains terminal marker are present).
    Sequential interactive dialog loops choke the extension's sub-process pipeline
    and crash the editor window. If an extension / non-interactive environment is
    detected:
      1. DO NOT launch an interactive choice loop or call AskUserQuestions
         sequentially.
      2. EXECUTION MODE MENU: automatically select option [1] DIRECT as the safe
         default, log the auto-selection to .claude/session_state.json
         (mode=DIRECT, reason=extension_crash_guard, timestamp), and output the
         high-visibility line:
         "[CRASH GUARD] VS Code/IDE extension detected. Suppressing interactive
          menu and defaulting to Option 1 (DIRECT) to prevent UI crash."
      3. HARD STOPS: do NOT stall on an un-answerable prompt loop. State the
         hard-stop parameters clearly in plain text, emit a clean one-line
         warning, write a checkpoint file under .claude/checkpoints/ capturing the
         pending decision, and halt execution awaiting the human's next message —
         never a blocking interactive read.
    This rule is environment-detection only; it never weakens a hard stop's
    requirement for human approval — it changes HOW approval is solicited (async
    text + checkpoint) so the editor cannot freeze. In a real TTY (CLI / desktop
    app) the normal interactive menu and prompts apply unchanged.

  2.5.8 — LONG-SESSION CONTEXT ANCHORING (Ground Truth Re-verification)
    To prevent attention degradation or context amnesia over extended execution
    timelines, treat `CLAUDE.md` on disk as your unalterable ground truth:
    - At the start of every distinct feature phase, task branch, or covenant
      invocation, perform a full re-read of `CLAUDE.md` to refresh your
      architectural alignment.
    - covenant.sh mechanically detects CLAUDE.md hash drift and warns when the
      constitution has changed since the last verified pass — heed that warning
      by re-reading the file before proceeding.
    - At the start of every session, if `.claude/checkpoints/LATEST.md` exists,
      read it and run `git rev-parse HEAD`. SHA matches checkpoint → execute
      RESUME INSTRUCTION and announce "Resuming from checkpoint <timestamp>:
      <task>". SHA diverged → state divergence and ask. Clearly new task →
      ignore; the old checkpoint will be superseded at next write.
    - After every successful `git commit`, immediately write LATEST.md using the
      full checkpoint schema. Not optional — this is the mechanism that makes
      /clear safe across sessions. A fresh session reads LATEST.md and continues
      without loss. Write LATEST.md before any push attempt.
    - Before executing any remote branch push command, compile and write a
      comprehensive state snapshot to `.claude/checkpoints/LATEST.md` capturing:
      what changed, why, and the architectural delta against baseline. covenant.sh
      enforces this at the pre-push boundary for agent sessions: it performs an
      immutable process-tree traversal from PPID to PID 1 to confirm the push
      originates from the Claude binary, then hard-blocks if no checkpoint exists.
      Human pushes are never blocked — process-tree tracking isolates agent and
      human tracks at the OS level with no heuristic bypass surface.
    - Monitor for context degradation signals: re-reading files already read this
      session (SD1), reproducing a fixed mistake (SD2), narrating prior steps
      unprompted (SD3), hedging on previously unambiguous facts (SD4), or 5+
      phases / 8+ files / session > 3 hours since last /clear (SD5). When 2+
      signals fire simultaneously: stop, write LATEST.md, output the forced
      handoff message ("CONTEXT SATURATION DETECTED..."), and wait for /clear
      before writing further code.
    - Your source of truth is the disk ledger, not the chat transcript.

  2.5.9 — DYNAMIC INTERROGATION & COMPULSORY BRAINSTORMING
    Scale architectural skepticism to classified task complexity:
    1. Tier 1 (Trivial/one-liner) or Tier 2 (simple single-file): proceed
       directly to execution without conversational overhead.
    2. Tier 3 (complex/multi-file) and above, or if you possess any
       contextual doubt regarding specifications, edge cases, or upstream
       contracts: you are STRICTLY FORBIDDEN from writing code until you
       execute a dedicated Brainstorming Phase.
    3. Brainstorming Phase Protocol:
       - Explicitly lay out your architectural hypothesis.
       - List the top 3 potential hidden regressions or failure modes.
       - Interrogate the human developer with specific, targeted questions
         to validate assumptions. Do not assume or guess missing
         specifications — demand clarity before a single line of code is
         written to disk.
    covenant.sh enforces this as a hard pre-commit block for agent sessions only:
    `_is_claude_agent_process()` traverses the OS process tree recursively;
    ≥5 staged files with no `.claude/checkpoints/LATEST.md` exits 1. Human
    commits are unaffected — the process-tree check provides a clean separation
    with no environment-flag or session-wrapper bypass surface. The graph index
    lifecycle uses a kill-and-restart loop (`_ensure_graph_freshness`): any
    active indexer is terminated via kill -9 before a fresh build for the
    current HEAD is spawned, preventing stale-index drift across commits.

You are initializing a NEW repository. The prime directive: the constitution
you generate PRESCRIBES the ideal architecture. Every rule you write will be
enforced by mechanical covenants and SECTION 2.5 cognitive routing on every future
task. A rule that doesn't match the repo creates permanent noise.

PHASE A — MANDATORY INTERROGATION (6 rounds, each blocking — Phase A.5 does
not start until round 5 is explicitly confirmed, and Phase B does not start
until Phase A.5's spec documents are explicitly approved)

You are about to write a constitution that will be mechanically enforced on
every future task in this repository, and — per Phase A.5 below — a set of
spec documents that PRESCRIBE what gets built. The Blocking Questions rule
this same constitution imposes on all downstream work (Guide §2.5.6b: every
hard stop and genuine scope fork is a structured, blocking question — e.g.
AskUserQuestion — never a sentence buried in prose) applies to generating the
constitution and the specs themselves, not just to work that happens after
they exist.

HARD RULE, applies to every round below and to Phase A.5: if you do not have
enough information to answer a question confidently, or a human's answer is
ambiguous enough that two different specs would result, STOP and ask a
follow-up — do not guess, do not silently pick the more common default, and
do not fabricate product/technical detail that was never actually stated.
An assumed fact written into a PRD or DB schema is indistinguishable from a
confirmed one to whoever reads it later; the cost of asking one more question
is a few seconds, the cost of a silently-wrong spec is discovered much later
and much more expensively. Assume the human answering these questions may be
inexperienced with governance tooling and with writing specs — ask plainly,
prefer concrete short-answer or one-word-acceptable questions over open-ended
ones, and do not proceed past a round until it is answered.

  ROUND 0 — Product & domain (this feeds the spec documents in Phase A.5 —
  do not skip it or treat it as optional just because it isn't about code):
    1. In one paragraph: what is this product/system, and what problem does
       it solve? Who are the primary users?
    2. What are the 3-7 core features or user stories that must exist for a
       first version? (Not a wishlist — what's actually in scope now.)
    3. What are the core data entities and how do they relate to each other,
       at a conceptual level (e.g. "a User has many Orders, an Order has one
       Shipment")? Skip only if round 1 establishes there's genuinely no
       persistence layer yet.
    4. What are the 2-4 most important user journeys/flows end to end (e.g.
       "visitor signs up -> verifies email -> completes onboarding")?
    5. Any known non-functional requirements: expected scale (users/requests),
       performance or availability expectations, required third-party
       integrations? "Don't know yet" is a legitimate answer — record it as
       such in the spec rather than inventing a number.

  ROUND 1 — Stack (as before):
    1. Language + framework
    2. Persistence layer (or "none yet")
    3. Test framework, linter, type checker (propose stack-standard defaults
       I can accept with one word)

  ROUND 2 — Operational reality (do not assume a deploy pipeline or schema
  exist just because this is a new project):
    1. Does a deploy pipeline exist yet, or is this local-only for now? (If
       none yet: release/hotfix branch strategies and their hard stops get
       seeded as dormant placeholders, not active rules — activating them
       later is a human edit, not something the agent infers on its own.)
    2. Is there a real schema/migration story yet, or is persistence still
       undecided? (If undecided: SQL-layer-boundary rules get seeded as
       placeholders scoped to wherever persistence code eventually lands,
       not enforced against nothing.)

  ROUND 3 — Risk posture:
    1. Any compliance/regulatory requirements today (SOC2, HIPAA, PCI, none
       yet)? This changes which security invariants in Guide §2.2.1 are
       load-bearing from day one versus boilerplate to relax later.
    2. Team size and Claude Code governance experience (solo / small team new
       to this / team already used to mechanical covenants)? This changes default
       friction — e.g. whether Tier-3 brainstorming (Guide §2.5.9) should be
       stricter than the framework default from day one, or start lighter and
       tighten later.

  ROUND 4 — Debt philosophy:
    True zero-tolerance from commit one (any lint/test failure blocks,
    no ratchet — the framework default for greenfield, since install.sh does
    not seed a baseline.json for this basket), or brownfield-style ratchet
    leniency even though this is greenfield (some teams deliberately want a
    lenient on-ramp while the team is still forming habits)? State which, and
    why if it's not the default. If leniency is chosen: this is a mechanical
    consequence, not just a stated preference — Phase B must additionally
    create an UNPOPULATED `.claude/baseline.json`, in the exact shape
    install.sh's own brownfield-only seeding step writes (see install.sh's
    baseline.json heredoc: `ratchet_mode`, `populated: false`,
    `generated_at: null`, `generated_from_sha`, `lint_findings: []`,
    `summary.lint_count: 0`) — do not invent a different shape, since covenant.sh
    treats a missing baseline as zero-tolerance and only recognizes this
    exact schema as "grandfather nothing yet, but a ratchet mechanism
    exists." Do not claim leniency was configured without actually writing
    this file.

  ROUND 5 — CORE_FILES seed confirmation (mandatory, cannot be skipped):
    Derive a proposed CORE_FILES seed list from rounds 1-4's answers (the
    config module, src/domain/**, DI wiring, test fixtures, plus anything
    round 2/3 flagged as load-bearing). Show me the proposed list explicitly
    and wait for confirmation or edits before writing anything in Phase B.
    Never seed a list I haven't seen.

Only after round 5 is confirmed does Phase A.5 begin. Every "seeded with:" and
"per Guide §X" instruction in Phase B below means "per what rounds 0-5 above
actually established," not a hardcoded default — if an instruction in Phase B
seems to assume something rounds 0-5 didn't establish, that is itself a gap:
stop and ask, do not silently fill it in.

PHASE A.5 — SPEC DOCUMENTS & APPROVAL (mandatory, blocks Phase B entirely —
no scaffold, no CLAUDE.md, no code, until this phase's approval step
completes)

Using only what rounds 0-5 actually established (never filling a gap with
invented detail — if you catch yourself about to write something no round
answer supports, stop and ask instead), draft the following under `docs/` in
this repository:

  - `docs/PRD.md` — Product Requirements Document: the problem, the primary
    users, the core features/user stories from round 0 stated as scoped
    requirements (in/out of scope for v1), and the success criteria from
    round 0 item 5 (or "not yet defined" if that's what was said — do not invent
    a metric).
  - `docs/TRD.md` — Technical Requirements Document: the stack from round 1,
    the operational/deploy reality from round 2, the compliance and scale/
    performance constraints from round 3 and round 0 item 5, and the debt
    philosophy from round 4.
  - `docs/DB_SCHEMA.md` — only if round 1 or round 0 established a real
    persistence layer. The entities and relationships from round 0,
    expressed as a concrete schema (tables/collections, key fields, and
    relationships) consistent with round 1's stack choice. If persistence is
    genuinely undecided, write a one-line placeholder file stating that,
    not a speculative schema.
  - `docs/USER_FLOWS.md` — the 2-4 journeys from round 0, each as a numbered
    step-by-step flow.
  - `docs/SYSTEM_DESIGN.md` — a high-level component/service breakdown
    consistent with the four-layer architecture Phase B is about to scaffold
    (Guide §2.2), any third-party integrations from round 0 item 5, and how the
    layers in Phase B's B1 map onto the components described here — this
    document and the scaffold Phase B produces must agree with each other.
  - `docs/ARCHITECTURE_DECISIONS.md` — a short ADR-style log of the load-
    bearing choices made across rounds 1-5 (stack, debt philosophy, which
    security invariants are active vs. dormant per round 3) with a one-line
    "why this over the alternative" for each — mirrors the CHECKPOINT
    schema's existing "decisions locked: why this over the alternative"
    convention (Guide §4.1) so the two stay consistent in style.

APPROVAL GATE (cannot be skipped or auto-confirmed): once all applicable
documents above are written, present them to me — either paste the full
content or give a concise per-document summary with the file paths, your
call based on total length — and explicitly ask me to approve, or to specify
what to change. Do not proceed to Phase B on silence or on an ambiguous
reply; if my reply doesn't clearly confirm approval, ask again. If I request
changes, revise the affected document(s) and re-present before asking again.
Only an explicit approval unblocks Phase B.

PHASE B — SCAFFOLD DEPLOYMENT (write everything)

  CRITICAL EXECUTION ORDER: .claude/settings.json already exists — install.sh
  scaffolded it with the universal, repo-independent deny-list before this
  prompt ever ran (see §B3 below). That scaffold deliberately excludes
  Write/Edit denial on .claude/settings.json itself and CLAUDE.md, because
  neither exists yet and you need to create them. Your LAST edit in Phase B —
  after CLAUDE.md, .githooks/ contents, and quarantine.txt are all fully
  written to disk — must ADD exactly these two self-lock pairs to the existing
  permissions.deny array: "Write(.claude/settings.json)",
  "Edit(.claude/settings.json)", "Write(CLAUDE.md)", "Edit(CLAUDE.md)". Once
  that edit lands, these files are completely agent-immutable by design — the
  same "write-once lock" guarantee the original design had, just now scoped to
  two entries instead of the whole file. Re-running initialization or
  repairing these files is a human-only action (hand-edit + PR); the agent
  cannot self-repair.

  As part of this same final step, set `constitution_source_version` in
  `.claude/covenant_state.json` to this framework's version (read `install.sh`'s
  own `FRAMEWORK_SEMVER` constant, or the version noted in
  `v1_claude_code_development_guide_new.md` if present). This is how a future
  `install.sh --upgrade` knows whether the dev guide's content has changed
  since CLAUDE.md was generated, and whether `/reconcile-governance` needs to
  run — leaving it null means an upgrade can never detect drift.

  B1. Directory scaffold per Guide §2.1:
      src/domain/, src/application/, src/infrastructure/, src/presentation/,
      tests/ mirroring src/ exactly, each with a placeholder module proving
      the import path works. Adapt directory idiom to the stack (e.g. Go:
      internal/domain etc.) while preserving the four-layer model and the
      mirror rule for tests.

  B2. CLAUDE.md at the root — the PRESCRIPTIVE constitution, containing:
      - The four-layer architecture enforcement section from Guide §2.2
        verbatim, with directory paths matching the scaffold
      - Naming contracts from Guide §2.2 (repositories fetch_*/find_*/
        persist_*/remove_*, Execute*/Query* use cases, PascalCase entities,
        past-tense events, tests/<layer>/test_<module> mirror rule)
      - The universal security invariants (Guide §2.2.1) verbatim, including
        the single-config-module rule, weighted per round 3's compliance
        answer (a stated regulatory requirement makes the matching invariant
        load-bearing text, not boilerplate to soften later)
      - Hard stops (Guide §2.2.2 — the full table, which includes
        permission-mode/settings changes, CORE_FILES edits, and
        quarantining a test that covers a CORE_FILES module). If round 2
        established no deploy pipeline exists yet, mark the release/hotfix
        branch-strategy hard stops as dormant placeholders in the table
        (present in the text, explicitly noted as inactive until a pipeline
        exists) rather than omitting or silently activating them.
      - The boundary caveats P1–P3 from Guide §2.3 (permission mode pinned;
        git push ALWAYS standalone, never inside &&, ;, or | chains;
        refspec-force banned in text and refused by the pre-push hook)
      - The CORE_FILES constitution element per Guide §2.2.3, seeded with
        exactly the list confirmed in Phase A round 5 — not re-derived or
        expanded here — maintained as the dependency graph grows (any module
        imported by >5 others joins it; editing the list is a hard stop)
      - Testing rules N1–N5 (Guide §6.1) plus the full §6.2 scaling block
        verbatim: the 60-second tier trigger, transitive closure for
        CORE_FILES, grep-is-a-lower-bound escalation (T4), committed
        quarantine.txt with the core-coverage hard stop (T2), and
        ledger-enforced tier transition (T5) — with this stack's exact
        test commands
      - A requirement (Guide §2.5.6a / §3.2.1) that PHASE 2's design
        declaration always includes an explicit GOAL/VERIFY pair and, where
        multiple interpretations exist, an ASSUMING/ALTERNATIVE declaration —
        a checkpoint or commit lacking a stated verification target does not
        satisfy the design-declaration requirement, even if tests happen to
        pass; and the Scope Discipline check (Guide §2.5.6a) runs before
        every checkpoint write, not after the diff is already staged
      - The Blocking Questions rule (Guide §2.5.6b): every HARD STOP and
        genuine scope fork is surfaced via a structured, blocking question
        mechanism (e.g. AskUserQuestion), never buried as a sentence inside
        a longer text response — a hard stop a human can scroll past isn't
        one
      - The auto-pipeline (recon -> contract -> execute -> output) and the
        commit/push covenant INCLUDING mandatory push confirmation (Guide §5.3)
      - Checkpoint trigger rules C1–C5, the schema, and the resume protocol
        (Guide §4.1)
      - A governance note (Guide §2.2.3): CLAUDE.md, the CORE_FILES list,
        settings, hooks, and baseline change ONLY via human-authored PR, never
        via agent edit; the agent never self-maintains the constitution

  B3. .claude/settings.json per Guide §2.3 — MERGE, do not regenerate.
      install.sh already scaffolded this file at install time via
      `_write_trust_root_settings` (universal, repo-independent deny-list + a
      Bash-matcher PreToolUse hook at `.claude/hooks/pre_bash_trust_root_guard.sh`
      that inspects the actual Bash command text for trust-root paths — closes
      the gap a static prefix-matched deny-list cannot: it can express "starts
      with X," never "mentions path Y anywhere," so redirection/tee/sed -i/
      python writes to a protected file aren't catchable by deny-list strings
      alone). Verify the scaffold is present (`permissions.deny` non-empty,
      `hooks.PreToolUse` contains the Bash-matcher entry) before adding
      anything — if missing, re-run install.sh rather than hand-authoring the
      deny-list. Your only job here is to ADD:
      - "defaultMode": "default" (already present — do not remove it)
      - Allow: read-only commands + this stack's exact test/lint/typecheck/
        build commands + git add/commit/diff/status/log/update-index
      - The reference deny-list below is what install.sh already wrote —
        listed here so you can verify it's intact, not so you re-type it:
        git reset --hard, git rebase, git clean, rm -rf, sudo, raw DDL,
        nc/ssh/scp, ALL force-push variants ("git push --force",
        "git push -f", "git push --force-with-lease", "git push --mirror",
        "git push --delete"),
        HOOK-EVASION variants ("git commit --no-verify*", "git commit -n *",
        "git push --no-verify*", "git -c core.hooksPath*") — without these,
        the allow-listed git commit is a silent back door around the entire
        enforcement layer,
        "Bash(SKIP_COVENANT=*)" (verify the matcher catches env-prefixed
        commands; if it does not, the pre-commit hook must refuse SKIP_COVENANT
        when agent-environment markers like $CLAUDECODE are present),
        and credential reads:
        "Read(.env)", "Read(**/.env)", "Read(**/.env.*)", "Read(**/*.pem)",
        "Read(**/id_rsa*)", "Read(**/.aws/credentials)" plus equivalent
        Bash cat patterns,
        and TRUST-ROOT writes for the files install.sh itself owns (never
        agent-generated, so safe to lock from install time): "Write(.githooks/**)",
        "Edit(.githooks/**)", "Write(.claude/hooks/**)", "Edit(.claude/hooks/**)"
        (Module A10 — this is the Bash-guard hook's OWN script; without this
        an agent can overwrite it with a one-line no-op while its
        `hooks.PreToolUse` registration stays intact in settings.json, making
        the guard LOOK active while doing nothing — a stealthier bypass than
        any of the ones the guard itself closes),
        "Write(.claude/covenant_integrity.sha256)", "Edit(.claude/covenant_integrity.sha256)"
        — without this, an agent that weakens .githooks/covenant.sh can simply
        regenerate the pinned hash to match in the same turn, and the CI
        content-check added in Module A6 verifies nothing; only a
        human-authored PR may move this pin,
        "Write(.claude/covenant_state.json)", "Edit(.claude/covenant_state.json)"
        (Module A10 — this is the covenant's own ledger: receipts, token spend,
        the audit log. An agent that can Write/Edit it directly can fabricate
        a passing receipt or reset its own token budget, defeating every
        other control in this chain. covenant.sh's own legitimate mutations run
        via a git-hook subprocess calling python3 directly — never through
        Claude Code's Write/Edit tools — so this denial has zero effect on
        covenant.sh's normal operation, only on the agent's own tool calls),
        "Write(.mcp.json)", "Edit(.mcp.json)" (Module A10 — controls which
        MCP servers Claude Code connects to; install.sh-owned, static after
        install, never agent-edited),
        "Write(.github/workflows/covenant.yml)", "Edit(.github/workflows/covenant.yml)"
        (Module A9 — the CI workflow file that INVOKES the whole integrity
        chain was itself unprotected through three rounds of hardening;
        without this, an agent can gut the "Run governance covenant" step or the
        `on:` triggers directly, with none of the hash-pin or Bash-guard
        machinery ever coming into play — a simpler, undefended bypass than
        any of the ones those mechanisms close),
        "Write(v1_claude_code_development_guide_new.md)",
        "Edit(v1_claude_code_development_guide_new.md)",
        "Write(v1_implementation_package_new.md)",
        "Edit(v1_implementation_package_new.md)",
        "Bash(git notes*remove*)", "Bash(git update-ref -d*)",
        and PERSISTENT-HOOK-DISABLE + UNTRACKED-COMMIT variants:
        "Bash(git config core.hooksPath*)",
        "Bash(git config --add core.hooksPath*)", "Bash(git commit -a*)",
        "Bash(git commit -am*)", "Bash(git commit --amend*)" — the persistent
        git config form disables hooks for the whole clone, and -a/-am/--amend
        break the index-equals-commit-tree guarantee the covenant relies on
      - NOT in this install-time scaffold, by design: "Write(.claude/settings.json)",
        "Edit(.claude/settings.json)", "Write(CLAUDE.md)", "Edit(CLAUDE.md)" —
        neither file exists yet at install time and you need to create them.
        Add these four entries as your LAST edit in Phase B, per the CRITICAL
        EXECUTION ORDER note above — never before CLAUDE.md and settings.json's
        own allow-list are fully written
      - The Bash-matcher guard hook (`.claude/hooks/pre_bash_trust_root_guard.sh`)
        deliberately protects .githooks/, .claude/hooks/ (itself),
        covenant_integrity.sha256, .claude/covenant_state.json,
        .github/workflows/covenant.yml, .mcp.json, and the dev-guide/init-package
        filenames — NOT CLAUDE.md/settings.json, since you legitimately need
        to reference those via Bash/python during this very phase (reading
        current state, merging JSON). Do not add them to that hook's
        protected-paths array.
      - The trust-root deny-list has exactly ONE source of truth: a single
        `REQUIRED_DENY` Python list inside `_write_trust_root_settings` in
        install.sh, shared by both the fresh-install and the idempotent-merge
        code paths (`os.path.exists` selects which branch runs). There is no
        second hand-authored copy anywhere to fall out of sync — do not
        introduce one; if you need to check what install.sh already wrote,
        read `.claude/settings.json` directly.
      - `.claude/covenant_integrity.sha256` is a multi-file manifest (native
        `sha256sum`/`shasum -a 256` check format — one `<hash>  <path>` line
        per file), not a single hash. It covers every static,
        install.sh-owned governance script: `.githooks/covenant.sh`,
        `.githooks/verify_governance_integrity.sh`, `.githooks/pre-commit`,
        `.githooks/pre-push`, and `.claude/hooks/pre_bash_trust_root_guard.sh`.
        `verify_governance_integrity.sh` checks all five with one
        `sha256sum -c` / `shasum -a 256 -c` call. Do not narrow this back to
        checking only covenant.sh — the whole point is that weakening any one of
        the five scripts in the enforcement chain is caught the same way.
      - git push in NEITHER list (it must prompt interactively)

  B4. .claude/commands/ — four files with this stack's real commands in every
      verification block:
      - feature.md: Phases 0–5 per Guide §3.2 — pre-flight, recon, design
        declaration (Phase 2), stubs-first (mandatory at 3+ files), implementation
        in strict layer order (Domain -> Infra -> App -> Presentation -> Tests),
        three-strike verification with the corrected index protocol before
        every re-run (git update-index -q --refresh; git diff --no-ext-diff —
        refresh reconciles stat metadata ONLY and must be paired with a
        content-level check), checkpoint evaluation at phase boundaries,
        full suite at the end (cheap while young), and COST-WARNING FIRING
        per Guide §7.1.1 (alert when a task iteration or phase exceeds ~40,000
        context tokens or history-retransmission waste crosses 50%).
        PHASE 2 (Design Declaration): the agent must programmatically deduce
        the testing architecture from repository roots (§6.0 Dynamic Stack
        Inference) — inspect package.json, requirements.txt, pyproject.toml,
        go.mod, or CI config; never assume a fixed runner. If a frontend or
        proxy layer is present, declare Playwright E2E user journeys implicitly
        (web-first async assertions, network contract checks) — zero prompts
        seeking human instruction on test paths.
        PHASE 3 (Implementation): execute all inferred test suites completely
        autonomously using the deduced runner engine(s). When UI/routing/rendering
        paths change, auto-generate and run Playwright specs (*.spec.ts or stack
        equivalent). Sequence backend + E2E runners; both must exit 0. No
        conversational filler or prompts asking the developer for test
        specifications are permitted.
      - audit.md: scoped to the covenant script's change set (changed + staged +
        unstaged + untracked files); greenfield rule: ANY finding blocks
        (CRITICAL/HIGH await human; MEDIUM/LOW auto-fix then re-verify);
        checks: secrets patterns, injection vectors, bare excepts, missing
        auth on routes, layer violations.
        SEVERITY NORMALIZATION TABLE: generate a mapping from each chosen
        scanner's NATIVE levels (error/warning, E/W codes, HIGH/MEDIUM/LOW)
        AND test-runner output formats (JUnit XML, JSON reporters, Playwright
        HTML/matrix/JSON reporters, Go test -json, pytest exit codes) to the
        covenant actions {block-await-human, auto-remediate, record-only} and embed
        it in audit.md — "CRITICAL/HIGH blocks" is undefined for linters that
        only emit error/warning or for test suites that emit structured reports
        without severity labels.
        SELF-HEALING FAILURE BRANCH: if an auto-remediation attempt
        (MEDIUM/LOW) does not eliminate the finding on re-verify, treat
        it as a hard block and report to the human — do not retry. Apply
        the §3.3 three-strike rule to any auto-fix attempt: three failed
        fix-and-re-verify cycles on the same finding → STOP, report
        verbatim, await human.
      - review.md: ledger-aware pre-PR covenant — recompute the FULL fingerprint
        (Guide §4.2, including untracked files) and compare against
        covenant_state.json; SKIP loudly only on exact match, printing the
        script-generated COVENANT REPORT verbatim (never a model-composed one).
        Otherwise: diff inventory, lockfile assertion (lockfile diff without
        approved dependency = HARD STOP), per-file layer compliance,
        secrets-in-diff grep, coverage check (every new function has a named
        test), quarantine report (a quarantined test covering CORE_FILES =
        HARD STOP), conventional-commit check, PR body generation; finish by
        having the covenant script write the new receipt atomically
      - prep.md: natural language -> SCOPE/OBJECTIVE/CONSTRAINTS/VERIFY/
        OUTPUT contract, zero implementation, hard stops flagged at top

  B5. THE ENFORCEMENT LAYER:
      Do NOT generate or modify `.githooks/covenant.sh`, `.githooks/pre-commit`, or `.githooks/pre-push`. These files have already been placed in the repository by the installation script. Leave them untouched. You must only verify that the `.githooks/` directory exists.

  B6. Stateful-layer bootstrap:
      - .claude/covenant_state.json with empty receipts and last_pass_sha: null
        (written only by covenant.sh from here on)
      - .claude/checkpoints/ with README.md stating the schema (Guide §4.1)
        and the 10-file retention rule
      - quarantine.txt (empty, committed) with a header comment explaining
        Guide §6.2 T2

  B7. Project hygiene files:
      - .gitignore: stack-standard ignores + .env + .env.worktree +
        .claude/covenant_state.json + .claude/checkpoints/
      - .env.example with placeholder keys only (never real values)
      - Dependency manifest with EXACT pinned versions for the minimal
        stack (framework, test runner, linter, type checker) — nothing
        speculative; additional dependencies arrive via the hard-stop process
      - A config module (single source of truth for env access) matching the
        security invariant — feature code never reads the environment
        directly
      - README.md: project name placeholder, quickstart including
        `cc-init-hooks`, and a pointer to CLAUDE.md as the constitution

  B8. .team_aliases at the root: Read v1_claude_code_development_guide_new.md
      from disk and copy APPENDIX B (the section headed "APPENDIX B —
      CANONICAL .team_aliases") VERBATIM into .team_aliases, substituting
      only the <source-dirs> placeholder with the scaffold's source and test
      directories (e.g. "src/ tests/", or the stack idiom chosen in B1).
      Beyond that substitution the file is byte-identical to Appendix B.
      Do not invent, add, or omit functions — security-relevant shell is
      never generated from memory.

PHASE C — VERIFICATION AND MANIFEST

  C1. Prove the scaffold is alive: run the test suite (placeholder tests
      pass), the linter, and the type checker — all exit 0.
  C2. Make a no-op commit on the setup branch to prove the pre-commit hook
      fires and emits a COVENANT REPORT; then verify the pre-push hook refuses
      a dry-run push to a protected branch name.
  C3. Output a manifest table: File | Purpose | Key rules encoded.
  C4. Output the three-line team summary: what was installed, that the
      baseline is zero-and-stays-zero, what changes about daily workflow
      (type intent; hooks and the pipeline handle the rest).
  C5. Remind me to commit everything EXCEPT .claude/covenant_state.json and
      .claude/checkpoints/ (gitignored), and to open a PR so the team
      approves the constitution like any code.

----------------------------------- PROMPT END -----------------------------------

## STEP 3 — ACTIVATION (human developer action)

1. Review the generated CLAUDE.md end to end — it governs every future session.
2. Verify hooks are active: `git config core.hooksPath` must print `.githooks`.
3. Commit and PR the governance + scaffold:
   `git add -A && git commit -m "chore(claude): initialize governed greenfield environment"`
   (`git add -A` is acceptable ONLY for this birth commit — the constitution
   you just installed bans it from here on. The pre-commit hook fires on this
   very commit — that's the system working.)
4. **Configure CODEOWNERS + branch protection — do this now, not later.** The
   trust-root deny-list (`.claude/settings.json`) and the Bash-guard hook
   stop Claude Code from tampering with the enforcement chain, but neither
   stops a human contributor — or an agent operating outside Claude Code
   entirely, e.g. through a bare `git commit` in a terminal — from editing
   `.claude/covenant_integrity.sha256` and `.githooks/covenant.sh` together in the
   same PR and having CI pass, since CI only verifies internal
   self-consistency, not consistency against an externally-trusted
   reference. Closing that gap requires a human control GitHub itself
   enforces, not something `install.sh` can configure for you. Create
   `.github/CODEOWNERS` at the repo root:
   ```
   /.githooks/                         @your-org/platform-team
   /.claude/covenant_integrity.sha256      @your-org/platform-team
   /.claude/hooks/                     @your-org/platform-team
   /.claude/settings.json              @your-org/platform-team
   /.github/workflows/covenant.yml         @your-org/platform-team
   /CLAUDE.md                          @your-org/platform-team
   ```
   Then, in the repo's Settings → Branches → branch protection rule for your
   default branch: enable "Require a pull request before merging" and
   "Require review from Code Owners." Without this second step, CODEOWNERS
   is purely advisory — GitHub does not enforce it unless a branch
   protection rule says to.
5. Activate aliases: `source .team_aliases`, then lock in:
   `echo "source $(pwd)/.team_aliases" >> ~/.zshrc`
6. Every teammate, after cloning: `cc-init-hooks` (one time).

## STEP 4 — DAILY WORKFLOW (from now on)

The mental model to internalize: **you talk to Claude normally.** You do not write
execution contracts by hand — the auto-pipeline (CLAUDE.md §9) derives one from any
code-change message, and the git hooks enforce the covenant mechanically no matter what
the session does. Your only standing jobs are: use `/prep` for non-trivial work,
answer hard-stop questions, confirm pushes, and start a fresh session per task.

The loop is identical in both environments; only the surface differs. Read the
section for the client you use.

### 4A. CLAUDE CODE — CLI (terminal)

One-time, per clone (DO THIS FIRST or the local hooks never fire):

```
source .team_aliases                 # or rely on the ~/.zshrc line from STEP 3
cc-init-hooks                         # sets core.hooksPath + bypass-note refspecs
```

Then, end to end:

1. **Start a task.** `cc-feature "add the payments endpoint"` — the wrapper refuses
   to launch on main/master/develop ("create a feature branch first") before Claude
   even starts. For a small, well-scoped change this is all you type.
2. **Scope something bigger first.** `/prep <description>` inside the session →
   review the SCOPE/OBJECTIVE/CONSTRAINTS contract → correct it in plain English →
   it executes. Use `cc-scope <symbol>` to see a change's impact radius first.
3. **Answer hard stops.** When the agent hits a new dependency, schema change,
   auth change, a CORE_FILES edit, or a governance file, it STOPS and asks. Reply
   with the decision. It cannot edit CLAUDE.md / settings.json / .githooks — those
   are human-PR-only, and it will say so if you ask.
4. **Commit + push.** Say `commit and push`. The model runs audit → review →
   tests and prints a COVENANT REPORT. In a greenfield repo there is no baseline —
   ANY finding blocks. It then states the branch + remote and waits for your `yes`
   (Covenant Step 4). The pre-commit / pre-push hooks fire mechanically regardless.
   - Prefer `cc-push`: it prints branch + remote and makes you **type the branch
     name** at the terminal (`/dev/tty`) to confirm — a fourth, model-independent
     human covenant. The `git push` permission prompt appears as a terminal y/n.
5. **Next task = new session.** One task per session. A new session auto-resumes
   from `.claude/checkpoints/LATEST.md`; `cc-checkpoint` writes one by hand before
   you stop. Heed the §7.1.1 cost warning — `/compact` or restart.
6. **Emergency only.** `SKIP_COVENANT=1 git commit ...` typed BY YOU in a plain
   terminal (never through the agent — it's deny-listed). The hook demands a typed
   reason via `/dev/tty`, records a git note, and starts the 24h re-run clock.

### 4B. CLAUDE CODE — Desktop app

One-time, per clone (DO THIS FIRST or the local hooks never fire). The app has no
shell aliases, so run the activation directly in the repo, or use the setup target:

```
git config core.hooksPath .githooks
git config --add remote.origin.push  'refs/notes/bypasses:refs/notes/bypasses'
git config --add remote.origin.fetch '+refs/notes/bypasses:refs/notes/bypasses'
```

Then, end to end:

1. **Start a task.** Type the request in chat in plain English, or use `/feature
   <description>`. There is no `cc-feature` branch-guard wrapper here — instead the
   pipeline's Phase 0 pre-flight checks the branch and refuses to proceed on
   main/master/develop. Same protection, enforced inside the pipeline.
2. **Scope something bigger first.** `/prep <description>` → review the contract in
   chat → correct it → it executes. Recommended for anything multi-file or touching
   shared/core modules.
3. **Answer hard stops.** Same as CLI — the agent stops and asks on new deps,
   schema, auth, CORE_FILES, or governance files, and cannot edit the trust-root
   files.
4. **Commit + push.** Say `commit and push`. Same COVENANT REPORT (greenfield: any
   finding blocks), same hooks. The difference: **the `git push` permission prompt
   is a UI approval card**, not a terminal y/n, and there is no `cc-push` "type the
   branch" step — so the **Covenant Step 4 chat confirmation is your primary human
   covenant.** Actually read the branch + remote line before clicking Allow.
5. **Next task = new conversation.** Start a new conversation for an unrelated
   task. Checkpoints are written automatically at phase boundaries; say "write a
   checkpoint" if you want one before stopping. Heed the §7.1.1 cost warning.
6. **Emergency only.** Same rule: `SKIP_COVENANT=1 git commit ...` is typed BY YOU in a
   real terminal outside the app — the agent is deny-listed from forming it, and
   the hook's `/dev/tty` reason prompt cannot be answered by the app's subprocess.

## WHAT SUCCESS LOOKS LIKE (first two weeks)

| Signal | Expected |
|---|---|
| Birth commit | Suite, linter, type checker all exit 0; pre-commit hook emits its first COVENANT REPORT |
| First feature | Stubs compiled before implementation; one commit per layer |
| Any lint/security finding | Hook BLOCKS (Phase A round 4 default: zero-tolerance, no baseline). If round 4 chose ratchet leniency, an unpopulated baseline.json exists instead — see round 4. |
| Brand-new untracked file with a finding | Covenant runs (fingerprint includes untracked files) and BLOCKS |
| `SKIP_COVENANT=1` from a Claude session | Blocked by settings deny; TTY adds human-presence assurance where the agent has no terminal |
| Session killed mid-task, reopened | Resume from checkpoint in under a minute |
| Suite runtime | Recorded by covenant.sh every run; TIER TRANSITION REQUIRED fires automatically at the 60s threshold |

---

> Appendix B (CANONICAL .team_aliases) has moved to
> `v1_claude_code_development_guide_new.md`. Step B8 reads it from that
> file on disk — single source of truth, no truncation risk.
