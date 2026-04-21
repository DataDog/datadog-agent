# Coordinator Quickstart

Zero-to-running in under 20 minutes. For full architecture and design notes
see [README.md](./README.md).

---

## Prereqs

- Repo access; you can `git push origin`.
- AWS SSO / `dda inv workspaces.*` working locally.
- One `workspace-evals-<detector>` workspace per detector you want
  plateau-gated `eval-component` validation for. Currently:
  `evals-bocpd`, `evals-scanmw`, `evals-scanwelch`. Adding a detector
  later = provision one more workspace with the matching name; no code
  change required (the coordinator derives the ssh alias by convention
  from `workspace_for_detector()`).
  ```bash
  # If a convention-named workspace is missing:
  dda inv workspaces.create evals-bocpd
  # ... same pattern for any new detector.
  ```
- An Anthropic API key.
- (Optional but recommended) a long-lived draft PR to use as the run-log
  (GitHub mobile + desktop notifications for free). PR #49678 is
  already set up for this; see **GitHub run-log PR** below.

---

## 1. Create the driver workspace

This is where the Python loop lives for days. It does only short ops
(SDK calls, `q.eval-scenarios`, git, ssh dispatch) — compute budget is
modest. The long-running `q.eval-component` jobs run on the three
detector workspaces, not here.

```bash
dda inv workspaces.create coord-driver
dda inv workspaces.tmux-new coord-driver
```

You should now be inside a tmux session on the driver workspace.

---

## 2. Install deps on the driver

```bash
# inside the driver workspace
pip install claude-agent-sdk pyyaml invoke requests
export ANTHROPIC_API_KEY=sk-…

# optional — bidirectional GitHub PR comments (recommended; see below)
export COORD_GITHUB_PR_NUMBER=49678
```

Persist the env vars across tmux detach by adding them to `~/.bashrc` (or
equivalent) on the workspace.

### GitHub run-log PR (recommended interaction channel)

PR #49678 is the long-lived "coordinator run log" draft PR. It never
merges. Setup is one line because `gh` is pre-authed on DD workspaces.

1. Confirm `gh` works on the driver workspace:
   ```bash
   gh auth status
   gh pr view 49678 --json number,title
   ```
2. Set the env var:
   ```bash
   export COORD_GITHUB_PR_NUMBER=49678
   ```
3. Smoke-test outbound:
   ```bash
   PYTHONPATH=tasks python -c "from coordinator import github_out; print(github_out.post('validation_completed','hello from coordinator'))"
   ```
   Expect `(True, 'ok')` and a new comment on https://github.com/DataDog/datadog-agent/pull/49678.
4. Smoke-test inbound: drop a comment on PR #49678 from the GitHub web/mobile UI (any plain text), then:
   ```bash
   PYTHONPATH=tasks python -c "from pathlib import Path; from coordinator import github_in; print(github_in.poll(Path('.')))"
   cat .coordinator/inbox.md
   ```
   Your comment should appear in `inbox.md`. Re-running `poll` is
   idempotent (state file `github_state.json` tracks the last-seen
   comment ID).
5. Enable GitHub mobile notifications for PR #49678: on phone, open the
   PR → tap **Watch** → **All Activity**. You now get a push for every
   coordinator event.

### Interacting while the coordinator runs

- **Read**: watch PR #49678 — the **Conversation** tab shows coordinator
  status comments intermixed with every approved candidate's commit.
- **Steer**: drop a PR comment in plain English. Polled at iteration start,
  routed through `inbox.md` → SDK interpretation → ACK comment back on
  the PR. E.g.:
  > stop tuning thresholds, try a correlator change
- **Commits**: the **Commits** tab shows every approved candidate as a
  separate commit (message includes iteration + candidate + score).
- **Don't approve/merge** this PR. It's a run log, not a review target.

---

## 3. Check out the harness branch

```bash
# inside the driver workspace
cd ~/datadog-agent
git fetch origin
git checkout ella/claude-coordinator-harness
```

Note: first coordinator run will automatically create the scratch branch
`claude/observer-improvements` off `origin/q-branch-observer`. You don't
need to pre-create it. This checkout is just so the coordinator code itself
is available.

---

## 4. Pre-download scenarios

```bash
dda inv q.download-scenarios
```

(The driver runs `q.eval-scenarios` for every T0 evaluation, so the
parquets need to be local.)

---

## 5. Verify ssh to the detector workspaces

```bash
ssh workspace-evals-bocpd    "echo ok"
ssh workspace-evals-scanmw   "echo ok"
ssh workspace-evals-scanwelch "echo ok"
```

All three should print `ok`. If any fails, add a matching entry to
`~/.ssh/config` on the driver workspace or debug with `dda inv
workspaces.cmd --workspace-name evals-bocpd --command "echo ok"`.

---

## 6. Seed state

Run the bootstrap scripts in order. Each is safe to re-run (they
`--overwrite` or skip-existing).

```bash
cd ~/datadog-agent

# 6a. Import a fresh q.eval-scenarios baseline (uses prior rebench).
# Repeatable --detector NAME=PATH; add one flag per detector.
PYTHONPATH=tasks python -m coordinator.import_baseline \
    --detector bocpd=eval-results/bocpd/report.json \
    --detector scanmw=eval-results/scanmw/report.json \
    --detector scanwelch=eval-results/scanwelch/report.json \
    --sha $(git rev-parse --short HEAD)

# 6b. (STRONGLY RECOMMENDED) Measure per-scenario F1 σ so regression
# gates use 3·σ_s per scenario instead of the scalar τ=0.05. Per-scenario
# variance in eval-component data spans 0.02 → 0.15 across scenarios;
# a scalar τ either ships noise or rejects real gains. Takes ~30 min
# per detector (5 baseline repeats × 6 min).
PYTHONPATH=tasks python -m coordinator.measure_sigma --seeds 5

# 6c. Seed the 6/4 train/lockbox split.
PYTHONPATH=tasks python -m coordinator.seed_split

# 6d. Load seed candidates A + B (tighten-gate and anomaly-rank).
PYTHONPATH=tasks python -m coordinator.seed_candidates

# 6e. Backfill existing eval-component reports as historical audit
# (into db.validations). Does NOT seed components_eval_dispatched —
# the coordinator will re-run eval-component on first plateau of a
# family targeting each component, because branch-modified code is
# different from the baseline-measured version.
PYTHONPATH=tasks python -m coordinator.import_validations

# 6f. Before the real run, set a hard API-token ceiling. Edit
# tasks/coordinator/config.py:
#
#    api_token_ceiling: int | None = 20_000_000   # ≈ $50-200 of Opus
#
# Prevents a runaway edge case from burning $1-5k of API spend.
# Leave None while smoke-testing.
```

If `eval-results/` doesn't exist on this driver workspace yet, pull it
from the detector workspaces:

```bash
mkdir -p eval-results/{bocpd,scanmw,scanwelch}
scp -r workspace-evals-bocpd:/tmp/observer-component-eval/.    eval-results/bocpd/
scp -r workspace-evals-scanmw:/tmp/observer-component-eval/.   eval-results/scanmw/
scp -r workspace-evals-scanwelch:/tmp/observer-component-eval/. eval-results/scanwelch/
```

---

## 7. Smoke-test

```bash
PYTHONPATH=tasks python -m coordinator.driver --once --dry-run
```

Expected output: one-line iteration summary, no side effects. If this
errors you have a setup problem — fix before launching the real loop.

Then a real one-iteration run:

```bash
PYTHONPATH=tasks python -m coordinator.driver --once
```

Expected flow:
1. polls empty validations dict
2. drains empty inbox
3. syncs from `origin/q-branch-observer` (first run forks `claude/observer-improvements`)
4. picks candidate `A-tighten-scan-gate`
5. SDK implementation agent edits scan detector thresholds
6. `q.eval-scenarios --only scanmw` runs (~6 min)
7. scoring + review happen
8. approve → commit + push + dispatch post-ship validation, OR reject → revert

---

## 8. Launch the forever loop

```bash
PYTHONPATH=tasks python -m coordinator.driver --forever 2>&1 | tee -a .coordinator/driver.log
```

Detach from tmux with `Ctrl-b d`. The loop keeps running.

---

## 9. Watch it

From the driver workspace (or by sshing in from your laptop):

```bash
# dashboard — regenerated every iteration
watch -n 5 cat .coordinator/metrics.md

# structured event stream
tail -f .coordinator/journal.jsonl | jq .

# coordinator → user signals
tail -f .coordinator/coord-out.md

# if GitHub PR configured: watch PR #49678 on github.com or the GitHub mobile app
```

---

## 10. Steer

### Send the coordinator a message

```bash
echo "stop tuning thresholds; try filtering 093_cloudflare only" \
  >> .coordinator/inbox.md
```

Coordinator picks it up at the next iteration start, writes an ACK to
`ack.log`, archives the original under `inbox-archive/`.

### Drop a fresh candidate idea

```bash
cat > .coordinator/candidates/C-my-idea.yaml <<'EOF'
id: C-my-idea
description: |
  What to try, why, expected outcome.
source: seed
target_components: [scanmw]
phase: "1"
approach_family: my-approach-family
EOF

PYTHONPATH=tasks python -m coordinator.seed_candidates
```

Coordinator picks it up when its turn arrives (diversity policy may
prefer it if you've been stuck on one family).

---

## 11. Stop it

```bash
dda inv workspaces.tmux-attach coord-driver
# inside tmux, stop the loop with Ctrl-C
```

The state in `.coordinator/db.yaml` persists. Restart any time with
`python -m coordinator.driver --forever` — `startup_cleanup` reconciles
any mid-iteration crash state.

---

## Fetch results locally

From your laptop at any point:

```bash
# dashboard snapshot
ssh workspace-coord-driver "cat ~/datadog-agent/.coordinator/metrics.md"

# db dump
scp workspace-coord-driver:~/datadog-agent/.coordinator/db.yaml ./db-snapshot.yaml
```

Or view the branch directly on GitHub:
`origin/claude/observer-improvements` — every commit there is a
reviewed, approved candidate with its experiment id in the message.

---

## Model routing (already configured)

- **Opus** — implementation agent, hack-detector reviewer, proposer (deep thinking).
- **Sonnet** — inbox message interpretation (lightweight).

Override in `tasks/coordinator/config.py`:

```python
model_deep: str = "claude-opus-4-7"
model_light: str = "claude-sonnet-4-6"
```

Set to `""` to use SDK default.

---

## What to expect

- First iteration: 6–15 min (eval + review + any post-ship dispatch).
- Steady state: one iteration every ~8–15 min.
- **Regression gates** compare to `db.last_shipped_per_scenario[detector]`
  (rolling reference) — so a candidate that adds on top of prior gains
  must improve from the LAST ship, not from the original baseline.
- **Eval-component** runs once per new component, only when the family
  iterating on that component plateaus (K=5 consecutive non-improving).
  Takes 2–4h on the detector workspace; result is lagging audit, never
  gates.
- **Overfit tripwire** fires every 5 ships: evaluates all shipped
  candidates on the lockbox, computes Spearman ρ between train-rank and
  lockbox-rank. ρ<0.5 → `tripwire` coord-out warning. Lockbox scores
  never appear in agent prompts.
- **Halt events** (exit the `--forever` loop):
  - Phase plateau (5 consecutive non-improving iterations)
  - Upstream sync conflict (manual rebase required)
  - Token ceiling exceeded (raise `CONFIG.api_token_ceiling` or halt)
- GitHub PR comments (if `COORD_GITHUB_PR_NUMBER` configured): budget
  milestones, phase exit, strict-regression auto-reject, validation
  completion/abandonment, overfit tripwire, upstream conflict,
  budget halt. Watch PR #49678 on mobile for push notifications.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `claude-agent-sdk not installed` | missing dep | `pip install claude-agent-sdk` |
| `ssh: host unreachable` on dispatch | ssh config missing on driver | add to `~/.ssh/config` or `dda inv workspaces.cmd` |
| Coordinator exits with "upstream conflict: halting" | someone pushed to `q-branch-observer` conflicting with the scratch branch | rebase `claude/observer-improvements` onto new upstream tip manually, re-run |
| Coordinator exits with "budget halt" | token ceiling reached | inspect spend (journal `tokens_used` entries), raise `CONFIG.api_token_ceiling`, re-run |
| `overfit tripwire` posted on PR | Spearman ρ between train and lockbox rankings < 0.5 across shipped candidates | investigate most-recently-shipped candidates — coordinator may be overfitting to train noise |
| Every candidate auto-rejected with strict_regression | per-scenario σ not measured; scalar τ=0.05 rejecting real gains on noisy scenarios | run `measure_sigma.py --seeds 5` |
| `metrics.md` shows ⚠ LIVENESS banner | no journal event in > 30 min; coordinator stuck | `ssh workspace-coord-driver "tmux attach -t coord-driver"` to see what it's doing |
| `working tree dirty; aborting iteration` | stray edits or orphan from prior crash | normally auto-handled by `startup_cleanup`; if not, `git checkout -- comp/observer tasks/q.py` |
| `upstream sync CONFIG` halts | someone pushed to `q-branch-observer` conflicting with `claude/observer-improvements` | rebase manually or reset scratch branch to new upstream tip |
| Bayesian / eval-bayesian appearing | it shouldn't — driver never invokes it | grep `journal.jsonl` for context |
| SDK retrying repeatedly | API rate limit or network | journal logs `github_post_failed` or SDK transient; auto-recovers |

Full design / flow diagrams in [README.md](./README.md). Spec:
`~/.claude/plans/ad-harness.allium`. Plan: `~/.claude/plans/ad-harness-plan.md`.
