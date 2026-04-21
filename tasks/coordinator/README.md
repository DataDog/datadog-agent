# Coordinator

An agent-driven harness that iteratively proposes, implements, evaluates, and
ships changes to the observer anomaly-detection pipeline. A long-running loop
takes a candidate, mutates code under `comp/observer/`, runs
`q.eval-scenarios`, gates against a baseline + review, commits on approval,
and kicks off an async workspace validation. Coordinator decides what to try
next via a proposer subagent that reads past experiment outcomes.

Source plan: `~/.claude/plans/ad-harness-plan.md`
Behavioural spec: `~/.claude/plans/ad-harness.allium`

---

## What's running, at a glance

```
                                  ┌──────────────────────────┐
                                  │     USER (you)           │
                                  └────────────┬─────────────┘
                                               │
                          inbox.md ◄──(atomic)─┤─── slack/coord-out.md
                                               │
┌──────────────────────────────────────────────▼──────────────────────────────────────┐
│  COORDINATOR  (long-running loop on claude/observer-improvements branch)           │
│                                                                                    │
│  ┌──────────┐  ┌─────────┐  ┌───────────┐  ┌────────┐  ┌─────────┐  ┌────────────┐ │
│  │ journal  │  │  db     │  │ metrics   │  │ inbox  │  │ coord-  │  │ validations│ │
│  │ .jsonl   │  │ .yaml   │  │  .md      │  │  .md   │  │ out.md  │  │ dict       │ │
│  └──────────┘  └────┬────┘  └───────────┘  └────────┘  └────┬────┘  └────────────┘ │
│                     │                                        │                     │
│                     │ source of truth                        │ → slack             │
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
├── reviewer.py            persona prompts (Skeptic, Conservative, …)
├── evaluator.py           subprocess wrapper for q.eval-scenarios
├── workspace_validate.py  post-ship async eval-component on ssh workspace
├── scoring.py             report → delta vs baseline → gate outcomes
├── git_ops.py             scratch-branch-only git plumbing
├── coord_out.py           coordinator→user channel (file + Slack)
├── slack_out.py           incoming webhook poster (fail-soft)
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
│       bans stuck approach_families (K=3 consec      │
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
│ 5.  score                                           │
│     scoring.score_against_baseline                  │
│       train-scoped: lockbox scenarios are observed  │
│         but not gated                               │
│       per-scenario ΔF1, Δprecision, Δrecall, ΔFPs   │
│       → strict_regressions[], recall_violations[]   │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 5a. phase state update (score-only)                 │
│     if mean_f1 > best: best=mean_f1; plateau=0      │
│     else: plateau++                                 │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 5b. strict-regression gate (pre-review)             │
│     if strict_regressions or recall_violations:     │
│       git_ops.revert_working_tree                   │
│       candidate.status = REJECTED                   │
│       emit coord-out (type=strict_regression)       │
│       return                                        │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│ 6.  review (SDK, 2 personas, unanimous required)    │
│     sdk.review_experiment                           │
│       Skeptic      → gain above noise?              │
│       Conservative → no regressions, no perf blow?  │
│     each returns YAML {approve, rationale}          │
└─────────────────────────┬───────────────────────────┘
                          ▼
                 ┌────────┴────────┐
         unanimous?               NOT unanimous
                 │                        │
                 ▼                        ▼
  ┌────────────────────┐      ┌──────────────────────┐
  │ git commit on      │      │ git checkout -- .    │
  │   coord branch     │      │ git clean -fd        │
  │ save_db(SHIPPED)   │      │ candidate=REJECTED   │
  │ git push -u origin │      │ return               │
  │ workspace_validate │      └──────────────────────┘
  │   dispatch (async) │
  │ save_db(validation)│
  │ return             │
  └────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 7.  persist                                         │
│     journal.append (every decision above)           │
│     metrics.regenerate → .coordinator/metrics.md    │
│     budget.check_milestones (50% / 80% → coord-out) │
│     save_db                                         │
└─────────────────────────┬───────────────────────────┘
                          ▼
                  iteration N+1 starts
```

Every "emit coord-out" also posts to Slack if `COORD_SLACK_WEBHOOK_URL` is set
(fail-soft).

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

## Async validation (post-ship)

Fire-and-forget, lagging data point only. Never gates anything.

```
      ship approved (iter N)
              │
              ▼
  ┌─────────────────────────────┐
  │ workspace_validate.dispatch │
  │   pick workspace-evals-<det>│
  │   ssh: tmux new-session -d  │
  │     "dda inv q.eval-        │
  │      component --component  │
  │      <det> --output-dir     │
  │      /tmp/…/exp-N"          │
  │   store PendingValidation   │
  │   save_db                   │
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
     coord-out.md  ◄──── appends + Slack webhook post ──────┘
     Slack DM       (COORD_SLACK_WEBHOOK_URL)
```

**Atomic rename** (not truncate): a user writing `inbox.md` mid-drain is
safe. The coordinator renames `inbox.md` → `inbox.md.reading` atomically;
the user's subsequent write creates a fresh `inbox.md`.

---

## Restartability

Crash scenarios and what happens on restart:

| Crash point                                  | Data at risk                | Recovery                                 |
|----------------------------------------------|-----------------------------|------------------------------------------|
| Between iterations                           | None                        | load_db resumes from last save           |
| Mid-implementation (agent edited files)      | Orphan working-tree diffs   | `startup_cleanup` reverts to HEAD        |
| After commit, before push                    | Unpushed local commits      | `startup_cleanup` pushes on startup      |
| After commit, before save_db                 | **Prevented**: save_db runs BEFORE push | N/A                            |
| After validation dispatch, before save_db    | **Prevented**: save_db runs immediately after dispatch | N/A                |
| Mid-SDK call (network blip, rate limit)      | None                        | `_with_retries` (3 attempts, exp backoff) on transient errors |
| Workspace killed with pending validation     | Orphan remote job           | `poll_pending_validations` abandons >48h |
| User edited inbox.md mid-drain               | None                        | atomic rename preserves both writes      |

### db.yaml is source of truth

- All other files (`metrics.md`, `journal.jsonl`, `coord-out.md`) are derived
  or append-only.
- SDK session IDs are not persisted. Restart spins up a fresh SDK session.
- `db.yaml` writes are atomic (`tempfile + os.replace`).

---

## Setup (first run)

```bash
# 1. Install deps
pip install claude-agent-sdk pyyaml invoke requests
export ANTHROPIC_API_KEY=…

# optional: Slack outbound notifications
export COORD_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/…

# 2. Seed baseline from a fresh q.eval-scenarios run
PYTHONPATH=tasks python -m coordinator.import_baseline \
    --bocpd     eval-results/bocpd/report.json   \
    --scanmw    eval-results/scanmw/report.json  \
    --scanwelch eval-results/scanwelch/report.json \
    --sha $(git rev-parse --short HEAD)

# 3. Seed the train/lockbox split (defaults match plan §4 rev-7)
PYTHONPATH=tasks python -m coordinator.seed_split

# 4. Seed initial candidates from YAML files
PYTHONPATH=tasks python -m coordinator.seed_candidates

# 5. (optional) Backfill any existing validation reports
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

# (if Slack configured) just watch the channel
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

- Per-scenario σ-calibrated τ — `M0.1` was once-per-combo by user policy, no σ to compute. Scalar `CONFIG.tau_default` (0.05) is used instead.
- T3 async Bayesian tuning (the rev-6 "divert while workspace runs 7+h" design) — cost premise didn't survive reality; `q.eval-bayesian` on scan detectors was taking 24+h and was abandoned. `q.eval-component` on workspace is fine because it's fire-and-forget post-ship (see workspace_validate).
- Rebaseline automation — manual `import_baseline` is fine for now.
- Fixed Phase 3 / Phase 4 milestones — per-signal-class routing and
  HP consolidation are just possible `approach_family` tags the proposer
  may surface if the data warrants.

**In scope if the data shows need**:

- Upstream sync conflict *resolution* (currently: abort + human takes over).
- Multi-persona review (Skeptic + Conservative today; Duplicate Hunter,
  Algorithm Expert, Greybeard ready in `reviewer.py` once db.yaml fills up).
- Bidirectional Slack (inbound requires bot token + slack_sdk).
