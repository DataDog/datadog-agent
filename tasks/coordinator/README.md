# Coordinator

An agent-driven harness that iteratively proposes, implements, evaluates, and
ships changes to the observer anomaly-detection pipeline. A long-running loop
takes a candidate, mutates code under `comp/observer/`, runs
`q.eval-scenarios`, gates against a baseline + review, commits on approval,
and kicks off an async workspace validation. Coordinator decides what to try
next via a proposer subagent that reads past experiment outcomes.

👉 **To get a coordinator running from scratch, see [QUICKSTART.md](./QUICKSTART.md).**

Source plan: `~/.claude/plans/ad-harness-plan.md`
Behavioural spec: `~/.claude/plans/ad-harness.allium`

---

## What's running, at a glance

```
                                  ┌──────────────────────────┐
                                  │     USER (you)           │
                                  └────────────┬─────────────┘
                                               │
                          inbox.md ◄──(atomic)─┤─── github/coord-out.md
                                               │
┌──────────────────────────────────────────────▼──────────────────────────────────────┐
│  COORDINATOR  (long-running loop on claude/observer-improvements branch)           │
│                                                                                    │
│  ┌──────────┐  ┌─────────┐  ┌───────────┐  ┌────────┐  ┌─────────┐  ┌────────────┐ │
│  │ journal  │  │  db     │  │ metrics   │  │ inbox  │  │ coord-  │  │ validations│ │
│  │ .jsonl   │  │ .yaml   │  │  .md      │  │  .md   │  │ out.md  │  │ dict       │ │
│  └──────────┘  └────┬────┘  └───────────┘  └────────┘  └────┬────┘  └────────────┘ │
│                     │                                        │                     │
│                     │ source of truth                        │ → github PR         │
└─────────────────────┼────────────────────────────────────────┼─────────────────────┘
                      │                                        │
            ┌─────────┴─────────┐                    ┌─────────┴──────┐
            │  SDK agents       │                    │ workspaces     │
            │  (Opus / Sonnet)  │                    │ (remote ssh)   │
            │                   │                    │                │
            │ ▸ implement cand. │                    │ ▸ eval-        │
            │ ▸ review (2 pers) │                    │   component    │
            │ ▸ propose ideas   │                    │ ▸ fire & forget│
            │ ▸ interpret inbox │                    │ ▸ polled at    │
            └───────────────────┘                    │   iter start   │
                                                     └────────────────┘
```

- Coordinator is Python (`driver.py`), glued by `claude_agent_sdk`.
- All git state is owned by the coordinator (scratch branch, commits, pushes).
- Implementation agent is sandboxed: `Read/Edit/Write/Bash/Grep/Glob` only,
  no git commands (PreToolUse hook blocks them).

---

## Module layout

```
tasks/coordinator/
├── driver.py              iteration loop orchestrator
├── scheduler.py           candidate picker + diversity policy
├── proposer.py            SDK subagent: brainstorms new candidates
├── sdk.py                 SDK wrappers + git-block hook + retry policy
├── reviewer.py            persona prompts (leakage_auditor + hack_detector; Duplicate Hunter / Algorithm Expert / Greybeard ready for Phase 2+)
├── evaluator.py           subprocess wrapper for q.eval-scenarios
├── workspace_validate.py  post-ship async eval-component on ssh workspace
├── scoring.py             report → delta vs baseline → gate outcomes
├── git_ops.py             scratch-branch-only git plumbing
├── coord_out.py           coordinator→user channel (file + GitHub PR comment)
├── github_out.py          post PR comments on the run-log PR (fail-soft)
├── github_in.py           poll PR comments → append user replies to inbox.md
├── measure_sigma.py       one-time: multi-seed baseline → populate per-scenario σ
├── overfit_check.py       periodic lockbox eval + Spearman rank-corr tripwire
├── inbox.py               user→coordinator channel (atomic rename)
├── budget.py              wall-hour tracking + milestone escalations
├── config.py              frozen constants (τ, plateau, retries, …)
├── db.py                  atomic YAML persistence
├── schema.py              typed dataclasses for every persisted record
├── metrics.py             markdown dashboard renderer
├── journal.py             append-only JSONL event log
├── import_baseline.py     bootstrap: eval reports → db.baseline
├── import_validations.py  backfill: pulled eval-component → db.validations
├── seed_split.py          bootstrap: train/lockbox split
├── seed_candidates.py     bootstrap: candidates YAML → db
├── pyproject.toml         pytest config (pythonpath = ..)
├── README.md              this file
└── tests/                 88 tests
```

### State directory (`.coordinator/`, gitignored)

```
.coordinator/
├── db.yaml                ← source of truth (experiments, candidates, split, baseline, validations)
├── journal.jsonl          ← append-only structured events (one per decision)
├── metrics.md             ← auto-regenerated dashboard (human-readable)
├── inbox.md               ← you write here; coordinator drains at iter start
├── inbox.md.reading       ← transient; present during an inbox drain
├── inbox-archive/         ← timestamped copies of drained messages
├── ack.log                ← coordinator's interpretation + planned change per ack
├── coord-out.md           ← coordinator → user signals (budget, phase exit, etc.)
├── candidates/            ← YAML seed files the loader reads into db.candidates
│   ├── A-tighten-scan-gate.yaml
│   ├── B-anomaly-rank.yaml
│   └── proposed-*.yaml    ← produced by proposer at runtime
└── reports/               ← per-experiment eval-scenarios JSON outputs
```

---

## One iteration, end-to-end

The critical path. Arrows are sequential; each box is a deterministic Python
step or an SDK subagent call.

```
                    iteration N starts
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│ 0.  poll pending validations                        │
│     workspace_validate.poll_pending_validations     │
│     (abandons stale >48h; pulls finished reports)   │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 1.  process inbox                                   │
│     inbox.claim_inbox (atomic rename)               │
│     sdk.interpret_inbox_message → (interp, plan)    │
│     inbox.ack_and_archive → ack.log                 │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 1a. sync upstream                                   │
│     git_ops.ensure_scratch_branch                   │
│       (first-time: fork off origin/q-branch-observer)│
│     git_ops.sync_from_upstream                      │
│       fetch → if ahead: merge --no-edit --no-ff     │
│       on CONFLICT: merge --abort, emit coord-out,   │
│       halt iteration                                │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 2.  pick next candidate                             │
│     scheduler.pick_next_candidate                   │
│       bans stuck approach_families (K=5 consec      │
│       non-improving → ban for 5 iters)              │
│       prefers candidates whose parents shipped      │
│     if none ⇒ sdk.propose (new YAMLs written to     │
│                 candidates/; retry pick once)       │
│     if still none ⇒ idle, save db, return           │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 2a. safety check                                    │
│     git_ops.is_clean(WATCH_PATHS)                   │
│       dirty? abort iteration (user may have edits)  │
│     pre_sha = current HEAD (post-merge baseline)    │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 3.  implement (SDK)                                 │
│     sdk.implement_candidate                         │
│       prompt = candidate.description                │
│       tools = Read/Edit/Write/Bash/Grep/Glob        │
│       PreToolUse hook: is_git_command → block       │
│       exceptions retried 3x on transient errors     │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 4.  eval                                            │
│     evaluator.run_scenarios                         │
│       dda inv q.eval-scenarios --only <detector>    │
│       rebuild binaries (new SHA from upstream sync) │
│       → report.json (~6 min on 10 scenarios)        │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 5.  score (vs FROZEN baseline)                      │
│     scoring.score_against_baseline                  │
│       train-scoped: lockbox observed, NOT gated     │
│       catastrophe filter:                           │
│         ΔF1 < -0.10 on any train scenario → reject  │
│         Δrecall < -0.10 (where baseline recall>5%)  │
│       blunt-but-honest: N=5 σ estimation was too    │
│         noisy to support per-scenario 3σ gating.    │
│       → strict_regressions[], recall_violations[]   │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 5a. phase state update (score-only, ε-gated)        │
│     if mean_f1 > best + ε: best=mean_f1; plateau=0  │
│     else: plateau++                                 │
│     ε = CONFIG.plateau_epsilon (0.01); noisy +0.001 │
│     bumps no longer reset the counter               │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 5b. strict-regression gate (pre-review, vs baseline)│
│     if strict_regressions or recall_violations:     │
│       git_ops.revert_working_tree                   │
│       candidate.status = REJECTED                   │
│       emit coord-out (type=strict_regression)       │
│       return                                        │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 6.  review (SDK, 2 personas in parallel)            │
│     sdk.review_experiment                           │
│       leakage_auditor: scenario/metric name leaks,  │
│         threshold-snapping, implicit identity,      │
│         hardcoded special cases                     │
│       hack_detector:   gain concentration,          │
│         complexity, proxy-gaming, prior-retread     │
│       both get `git diff HEAD` + scenario rosters   │
│       structured YAML output; per-check evidence    │
│         required — stub evidence → auto-reject      │
│       unanimity required (both must approve)        │
└─────────────────────────┬───────────────────────────┘
                          ▼
                 ┌────────┴────────┐
         unanimous?               NOT unanimous
                 │                        │
                 ▼                        ▼
  ┌──────────────────────────────┐  ┌──────────────────────┐
  │ save_db(SHIPPED, sha=pending)│  │ git checkout -- .    │
  │ git commit on coord branch   │  │ git clean -fd        │
  │ save_db(sha=<real>)          │  └──────────────────────┘
  │ git push -u origin           │
  │ if family plateaued AND      │
  │   component not yet in       │
  │   db.components_eval_dispatched:
  │   dispatch eval-component    │
  │ overfit_check.maybe_run      │
  │   (decorative at 2-scenario  │
  │    lockbox; kept for audit)  │
  └──────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 7.  persist                                         │
│     journal.append (every decision above)           │
│     sdk.consume_token_count → budget.api_tokens_used│
│     if tokens_used ≥ ceiling: BudgetCeilingHalt     │
│     metrics.regenerate → .coordinator/metrics.md    │
│     budget.check_milestones (50% / 80% → coord-out) │
│     save_db                                         │
└─────────────────────────┬───────────────────────────┘
                          ▼
                  iteration N+1 starts
```

Every "emit coord-out" also posts a comment on the **run-log GitHub PR**
if `COORD_GITHUB_PR_NUMBER` is set. GitHub is pre-authed on DD
workspaces via the `gh` CLI — no new app, no admin approval, no token
management. `github_in.poll` also polls the same PR at iteration start
for user replies, routing them through `inbox.md` (same drain →
SDK-interpret → ACK flow).

The run-log PR is a long-lived draft PR from the scratch branch into
the upstream feature branch (e.g. PR #49678: `claude/observer-improvements`
→ `q-branch-observer`). It never merges; it's the canonical audit trail.

See QUICKSTART.md for the setup one-liner.

---

## Cross-iteration: phase, plateau, proposer

```
 seeds loaded                 plateau reached                phase exit
     │                             │                              │
     ▼                             ▼                              ▼
┌─────────────┐   iterate    ┌───────────────┐   K iters   ┌───────────────┐
│  PROPOSED   ├─────────────►│  plateau_     ├────────────►│  emit coord-  │
│  candidates │              │  counter++    │  without    │  out,         │
│  in queue   │              │  until K=5    │  improvement│  exit loop    │
└──────┬──────┘              └───────┬───────┘             └───────────────┘
       │                             │
       │ queue empty OR              │ queue empty
       │ all families banned         │
       ▼                             ▼
┌─────────────────────────────────────────────┐
│  sdk.propose(banned_families, last_10_exps) │
│    Opus reads: baseline, recent F1s,        │
│      review rationales, existing families   │
│    returns 3 fresh YAMLs                    │
│    written to .coordinator/candidates/      │
└─────────────┬───────────────────────────────┘
              │
              └─────► retry pick_next_candidate
```

### Diversity policy (anti-stuck)

```
experiments chronologically:
  … [fam=A, ΔF1=-.02] [fam=A, ΔF1=-.01] [fam=A, ΔF1=+.00] …
                              │
                              ▼
                scheduler._family_consecutive_non_improving(A) = 3
                              │
                              ▼
                scheduler.stuck_families → {A}
                              │
                              ▼
        pick_next_candidate filters out family A candidates
                              │
                              ▼
        if only A candidates exist → proposer runs with banned={A}
```

Ban implicitly clears when:
- another family runs an experiment (streak broken), OR
- any experiment produces a new phase-best score.

---

## Git flow

```
                    origin/q-branch-observer  (upstream feature branch)
                                │
                                │ fetch + merge --no-ff (iter start)
                                ▼
         ┌──────────────────────────────────────────────┐
         │  claude/observer-improvements  (scratch)     │
         │                                              │
         │  ▸ forked off origin/q-branch-observer on    │
         │    first run (regardless of operator HEAD)   │
         │  ▸ coordinator commits here after review     │
         │  ▸ coordinator pushes here (never to other)  │
         │  ▸ never merged back into q-branch-observer  │
         │    by the coordinator (you do it manually    │
         │    if/when ready)                            │
         └──────────────────┬───────────────────────────┘
                            │
                            ▼
                origin/claude/observer-improvements
                (proof-of-value commit log; every commit = 1 reviewed candidate)
```

### Safety invariants enforced in code

- `commit_candidate` refuses to commit on any branch other than scratch (`WrongBranchError`).
- `push_scratch_branch` refuses to push any other branch.
- No push to upstream, ever — coordinator never touches `q-branch-observer`.
- Implementation agent's Bash is filtered by a `PreToolUse` hook:
  `is_git_command("true && git push") → True → block`.
  Regex tested against `ls -la git`, `gitk`, `git-foo`, chained forms, etc.
- `startup_cleanup` on every restart: reverts orphan working-tree diffs,
  pushes orphan commits from a crash between commit and push.

---

## Where things run

Four machines total — one driver, three dedicated evaluators. The driver
never does long-running work; all hours-scale jobs land on the evaluators.

```
┌────────────────────────────────────────────────────────────────┐
│  coord-driver workspace  (days of uptime, modest compute)      │
│                                                                │
│  ▸ driver.py iteration loop                                    │
│  ▸ SDK calls (Opus / Sonnet)                                   │
│  ▸ q.eval-scenarios subprocess (~6 min per iter)               │
│  ▸ git ops on claude/observer-improvements                     │
│  ▸ ssh dispatch to the three detector workspaces               │
│                                                                │
│  per-iteration wall-time: ~8-15 min                            │
└──────────────────────────┬─────────────────────────────────────┘
                           │ ssh + scp (ALL SHORT ops)
     ┌─────────────────────┼─────────────────────┐
     ▼                     ▼                     ▼
┌─────────────┐     ┌─────────────┐       ┌─────────────┐
│evals-bocpd  │     │evals-scanmw │       │evals-scanwelch
│             │     │             │       │             │
│ q.eval-     │     │ q.eval-     │       │ q.eval-     │
│  component  │     │  component  │       │  component  │
│ for bocpd   │     │ for scanmw  │       │ for scanwelch│
│ candidates  │     │ candidates  │       │ candidates  │
│             │     │             │       │             │
│ per-run:    │     │ per-run:    │       │ per-run:    │
│  2-4 hours  │     │  2-4 hours  │       │  2-4 hours  │
└─────────────┘     └─────────────┘       └─────────────┘
```

**Key invariant**: the driver never waits for a detector workspace.
Dispatch is fire-and-forget; polling at the next iteration start checks
for completion and ports results asynchronously. Results never gate
downstream decisions — they are purely informational, recorded on the
experiment for audit.

## Model routing

Deep-thinking SDK calls use Opus; lightweight summaries use Sonnet.
Tune in `config.py`:

| Role | Model | Task |
|---|---|---|
| Implementation agent | `model_deep` (Opus) | writing code for a candidate |
| Review personas | `model_deep` (Opus) | judging approve/reject |
| Proposer | `model_deep` (Opus) | brainstorming new candidates from history |
| Inbox interpreter | `model_light` (Sonnet) | summarizing a user message |

Set `CONFIG.model_deep` / `CONFIG.model_light` to an empty string to fall
back to the SDK default.

## Gates and what they actually do

The branch accumulates one commit per approved candidate — nothing is
reverted when pivoting families — so by iter 10, `q.eval-scenarios` runs
against the cumulative state of 10 prior wins. Gates must decide whether
the *new* candidate is acceptable on top of that state.

**Single reference:** `db.baseline` (frozen M0.1 scores). Gates always
compare to this, never to a rolling "last-shipped" reference.

Why no rolling reference: an earlier design compared to the immediately-
prior committed state. A panel review identified this as a noise-driven
ratchet — lucky ships permanently elevate the floor, and the next
candidate only needs to not-regress from the elevated floor, so a
candidate strictly worse than baseline can still pass. Over N ships
E[cumulative drift from baseline] ≈ σ·√(2 ln N) of pure noise. Dropped.

**Catastrophe filter, not statistical discrimination.** The strict-
regression gate fires when any train scenario drops ΔF1 < -0.10 vs
baseline; recall-floor when Δrecall < -0.10 (where baseline recall was
non-trivial). An earlier design tried per-scenario 3·σ_s gates, but
N=5 σ estimation has a 95% CI of [0.6σ, 2.2σ] — the σ numbers are
themselves too noisy for 3σ to mean what it says, and F1 is bounded/
skewed so Gaussian assumptions don't apply. The gate is intentionally
blunt: "did the detector visibly break?" The reviewer does the nuance.

**Two-persona review with forced evidence.** `leakage_auditor` hunts
for scenario-name/metric-name leakage, threshold-snapping, implicit
identity encoding, and hardcoded special cases. `hack_detector` covers
gain concentration, complexity-proportionality, proxy-gaming, and
prior-rejection retread. Both get the full `git diff HEAD` and both
train + lockbox scenario rosters in the prompt. Output is structured
YAML with per-check `{status, evidence}` fields; stub evidence or
missing checks block → auto-reject. Unanimity required.

**Plateau detection is effect-size aware.** An experiment only
"improves" when score > best + ε (ε = 0.01). A raw strict-greater
comparison let noisy +0.001 bumps keep dead-end families alive
indefinitely while abandoning real winners whose signal happened
to be flat for 5 draws.

**End-of-run framing:** these gates don't prove shipped candidates are
better. They're a noise filter + reviewer vote that produces a
short-list of candidates worth investigating. Run a proper offline
re-eval (N≥20 seeds against the frozen baseline) on the shipped set
before claiming any improvement.

## Async component validation (plateau-gated)

Fire-and-forget, once per new component. Never gates anything.

```
  ship approved (iter N, family=scan-gate-internal, components=[scanmw])
              │
              ▼
  ┌─────────────────────────────────────────────────────┐
  │ family_consecutive_nonimproving(scan-gate-internal) │
  │   < stuck_threshold?  → SKIP (still iterating)      │
  │   ≥ stuck_threshold?  → dispatch for components NOT │
  │                          yet in                     │
  │                          db.components_eval_dispatched │
  └──────────────────┬──────────────────────────────────┘
                     │ plateau hit, scanmw not yet validated on this branch
                     ▼
  ┌─────────────────────────────┐
  │ workspace_validate.dispatch │
  │   write dispatching record  │
  │   save_db                   │
  │   ssh: tmux new-session -d  │
  │     "dda inv q.eval-        │
  │      component --component  │
  │      scanmw --output-dir    │
  │      /tmp/…/exp-N"          │
  │   flip status: pending      │
  │   save_db                   │
  │   mark scanmw "dispatched"  │
  └──────────┬──────────────────┘
             │
             ▼ (hours later, iter N+K)
  ┌─────────────────────────────┐
  │ poll_pending_validations    │
  │   ssh test -f .../report.json │
  │   if yes: scp -r; parse;    │
  │     mark done; emit coord-out│
  │   if age > 48h: abandon     │
  └─────────────────────────────┘
```

**Rationale**: `eval-component` answers "does this detector pull its
weight vs random component subsets?" — a component-value question, not
a per-config correctness question. Running it on every ship was
expensive and often skipped due to workspace-busy. Running it once
per new component, after the search has exhausted improvement, matches
its actual purpose.

**`components_eval_dispatched` starts empty** — there is no pre-seed,
because even `bocpd/scanmw/scanwelch` get modified by the coordinator
on the branch, so their historical baseline eval-component reports
(in `eval-results/`, imported to `db.validations`) don't reflect the
current branch state. Each component gets exactly one dispatch on
first plateau of a family targeting it.

Workspace mapping:

| Detector   | Workspace                   |
|------------|-----------------------------|
| bocpd      | `workspace-evals-bocpd`     |
| scanmw     | `workspace-evals-scanmw`    |
| scanwelch  | `workspace-evals-scanwelch` |

One concurrent validation per workspace (`workspace_busy` check).

---

## User feedback loop

```
  ┌──────────────────┐                              ┌─────────────────────┐
  │    USER          │                              │   COORDINATOR       │
  └────────┬─────────┘                              └────────┬────────────┘
           │                                                 │
           │  writes free-form markdown                      │
           ▼                                                 │
     inbox.md  ──────── atomic rename ───────►               │
                        (claim_inbox)                        ▼
                                                   ┌────────────────────┐
                                                   │ sdk.interpret_     │
                                                   │   inbox_message    │
                                                   │   → (interp, plan) │
                                                   └────────┬───────────┘
                                                            │
                                                            ▼
     ack.log  ◄──── appends (echo+interp+plan, archive) ────┤
     inbox-archive/<ts>.md                                  │
                                                            │
                                                            │ (at milestones,
                                                            │  phase exits,
                                                            │  strict regress)
                                                            ▼
     coord-out.md  ◄──── appends + gh pr comment ───────────┘
     run-log PR     (COORD_GITHUB_PR_NUMBER; mobile + desktop
                     notifications via GitHub app)
```

**Atomic rename** (not truncate): a user writing `inbox.md` mid-drain is
safe. The coordinator renames `inbox.md` → `inbox.md.reading` atomically;
the user's subsequent write creates a fresh `inbox.md`.

---

## Restartability

Crash scenarios and what happens on restart:

| Crash point                                  | Data at risk                | Recovery                                 |
|----------------------------------------------|-----------------------------|------------------------------------------|
| Between iterations                           | None                        | `load_db` resumes from last save         |
| Mid-implementation (agent edited files)      | Orphan working-tree diffs   | `startup_cleanup` reverts WATCH_PATHS    |
| After commit, before push                    | Unpushed local commits      | `startup_cleanup` pushes on startup      |
| After commit, before save_db                 | **Prevented**: save_db runs BEFORE push | N/A                          |
| Mid-validation-dispatch (ssh ran, db unsaved)| **Prevented**: `dispatching` status written BEFORE ssh; startup reaps stuck `dispatching` as `failed` | N/A |
| Mid-inbox-drain (renamed, never acked)       | **Prevented**: `recover_orphan_reading` archives `inbox.md.reading` with `orphan-recovery` tag on startup | N/A |
| User hand-edited WATCH_PATHS between runs    | Could merge-clobber on sync | Dirty-tree guard runs BEFORE `sync_from_upstream` — iteration aborts instead of clobbering |
| Mid-SDK call (network blip, rate limit)      | None                        | `_with_retries` (3 attempts, exp backoff) on transient errors |
| Workspace killed with pending validation     | Orphan remote job           | `poll_pending_validations` abandons >48h |
| User edited inbox.md mid-drain               | None                        | atomic rename preserves both writes      |
| Upstream conflict mid-run                    | **Halt, not wedge**: `UpstreamConflictHalt` exits `--forever` loop; user rebases + restarts | N/A |
| Token budget exceeded                        | **Halt, not wedge**: `BudgetCeilingHalt` exits `--forever`; user raises ceiling or investigates | N/A |

### db.yaml is source of truth

- All other files (`metrics.md`, `journal.jsonl`, `coord-out.md`) are derived
  or append-only.
- SDK session IDs are not persisted. Restart spins up a fresh SDK session.
- `db.yaml` writes are atomic (`tempfile + os.replace`).

---

## Setup (first run)

See [QUICKSTART.md](./QUICKSTART.md) for the full workspace-based walkthrough.
Summary of the bootstrap scripts:

```bash
# 1. Install deps
pip install claude-agent-sdk pyyaml invoke requests
export ANTHROPIC_API_KEY=…
export COORD_GITHUB_PR_NUMBER=49678  # optional — run-log PR number

# Before running for real, edit tasks/coordinator/config.py and set:
#   api_token_ceiling = <N>   # e.g. 20_000_000 ≈ $50–200 of Opus
# Prevents a multi-day edge case from burning $1–5k of API spend.

# 2. Seed baseline from a fresh q.eval-scenarios run.
# --detector NAME=PATH is repeatable; add a flag per detector.
PYTHONPATH=tasks python -m coordinator.import_baseline \
    --detector bocpd=eval-results/bocpd/report.json \
    --detector scanmw=eval-results/scanmw/report.json \
    --detector scanwelch=eval-results/scanwelch/report.json \
    --sha $(git rev-parse --short HEAD)

# 3. (optional) Populate per-scenario σ in db.yaml for post-run
# diagnostics. The live gate is now a catastrophe filter (ΔF1 < -0.10)
# which doesn't use σ — N=5 σ estimation was too noisy to support
# statistical gating. The σ data is still useful when auditing shipped
# candidates offline. Takes ~30 min per detector (5 baseline × 6 min).
PYTHONPATH=tasks python -m coordinator.measure_sigma --seeds 5

# 4. Seed the train/lockbox split
PYTHONPATH=tasks python -m coordinator.seed_split

# 5. Seed initial candidates from YAML files
PYTHONPATH=tasks python -m coordinator.seed_candidates

# 6. (optional) Backfill any existing validation reports into
# db.validations as historical audit. Does NOT seed
# components_eval_dispatched — every component gets a fresh dispatch
# on first plateau after the coordinator modifies it.
PYTHONPATH=tasks python -m coordinator.import_validations
```

## Running

```bash
# Single iteration (safe smoke test)
PYTHONPATH=tasks python -m coordinator.driver --once

# Forever loop until phase plateaus
PYTHONPATH=tasks python -m coordinator.driver --forever

# Dry-run (no git, no SDK, no eval, no db writes)
PYTHONPATH=tasks python -m coordinator.driver --once --dry-run
```

## Watching

```bash
# Human-readable dashboard, auto-regenerated every iteration
watch -n 5 cat .coordinator/metrics.md

# Structured event stream
tail -f .coordinator/journal.jsonl | jq .

# Reverse channel from coordinator to you
tail -f .coordinator/coord-out.md

# if GitHub PR configured: watch PR on github.com or the GitHub mobile app
```

## Steering

```bash
# Tell the coordinator something
echo "stop tuning thresholds; try a filter on 093_cloudflare only" \
  >> .coordinator/inbox.md

# Drop a new candidate idea
cat > .coordinator/candidates/C-my-idea.yaml <<'EOF'
id: C-my-idea
description: |
  …
source: seed
target_components: [scanmw]
phase: "1"
approach_family: my-family
EOF
PYTHONPATH=tasks python -m coordinator.seed_candidates
```

---

## Testing

```bash
cd tasks/coordinator
/tmp/coord-venv/bin/python -m pytest -q
# 90 tests, ~20s
```

Tests span git safety (branch guards, command regex, clean checks), inbox
atomic-rename races, scoring gate semantics, scheduler diversity bans,
proposer YAML round-trips, workspace validation dispatch/poll, dashboard
rendering, schema persistence, retry policy, upstream sync (merge + conflict
+ abort).

---

## Gaps and non-goals (as of M1-setup)

Per rev-7 / rev-8 triage of the design plan:

**Explicitly deferred** (not implemented, may never be):

- Per-scenario σ-calibrated τ — tried and dropped. N=5 σ estimation is itself too noisy (95% CI on σ spans [0.6σ, 2.2σ]) to support 3σ gating, and F1 is bounded/skewed so Gaussian assumptions don't apply. Replaced by a flat ΔF1 < -0.10 catastrophe filter.
- T3 async Bayesian tuning (the rev-6 "divert while workspace runs 7+h" design) — cost premise didn't survive reality; `q.eval-bayesian` on scan detectors was taking 24+h and was abandoned. `q.eval-component` on workspace is fine because it's fire-and-forget post-ship (see workspace_validate).
- Rebaseline automation — manual `import_baseline` is fine for now.
- Fixed Phase 3 / Phase 4 milestones — per-signal-class routing and
  HP consolidation are just possible `approach_family` tags the proposer
  may surface if the data warrants.

**In scope if the data shows need**:

- Upstream sync conflict *resolution* (currently: abort + human takes over).
- Phase-2 review personas (leakage_auditor + hack_detector ship today;
  Duplicate Hunter, Algorithm Expert, Greybeard ready in `reviewer.py`
  to add once db.yaml fills up).
- Additional notification transports (Slack, email, etc.). GitHub PR
  comments cover mobile + desktop + push notifications without new
  creds; reach for anything else only if GitHub fails a specific need.
