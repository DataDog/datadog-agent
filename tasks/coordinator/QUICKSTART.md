# Coordinator Quickstart

Zero-to-running in under 20 minutes. For full architecture and design notes
see [README.md](./README.md).

---

## Prereqs

- Repo access; you can `git push origin`.
- AWS SSO / `dda inv workspaces.*` working locally.
- The three detector workspaces already exist (`evals-bocpd`,
  `evals-scanmw`, `evals-scanwelch`). If not:
  ```bash
  dda inv workspaces.create evals-bocpd
  dda inv workspaces.create evals-scanmw
  dda inv workspaces.create evals-scanwelch
  ```
- An Anthropic API key.
- (Optional) a Slack Incoming Webhook URL for notifications.

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

# optional — outbound Slack notifications
export COORD_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/…
```

Persist the env vars across tmux detach by adding them to `~/.bashrc` (or
equivalent) on the workspace.

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

Run the four bootstrap scripts in order. Each is safe to re-run (they
`--overwrite` or skip-existing).

```bash
cd ~/datadog-agent

# 6a. Import a fresh q.eval-scenarios baseline (uses prior rebench).
PYTHONPATH=tasks python -m coordinator.import_baseline \
    --bocpd     eval-results/bocpd/report.json   \
    --scanmw    eval-results/scanmw/report.json  \
    --scanwelch eval-results/scanwelch/report.json \
    --sha $(git rev-parse --short HEAD)

# 6b. Seed the 6/4 train/lockbox split.
PYTHONPATH=tasks python -m coordinator.seed_split

# 6c. Load seed candidates A + B (tighten-gate and anomaly-rank).
PYTHONPATH=tasks python -m coordinator.seed_candidates

# 6d. Backfill existing eval-component results as historical validations.
PYTHONPATH=tasks python -m coordinator.import_validations
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

# if Slack: just watch the channel you hooked up
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

- **Opus** — implementation agent, review personas, proposer (deep thinking).
- **Sonnet** — inbox message interpretation (lightweight).

Override in `tasks/coordinator/config.py`:

```python
model_deep: str = "claude-opus-4-7"
model_light: str = "claude-sonnet-4-6"
```

Set to `""` to use SDK default.

---

## What to expect

- First iteration: 6-15 min (eval + review + any post-ship validation dispatch).
- Steady state: one iteration every ~8-15 min.
- Post-ship eval-component: kicks off async on the detector workspace; typically 2-4 hours; lands on a later iteration's poll.
- Phase exit: after 5 consecutive non-improving iterations. Emits a coord-out message and stops.
- Slack notifications (if configured): budget milestones, phase exit, strict-regression auto-reject, validation completion, upstream conflict.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `claude-agent-sdk not installed` | missing dep | `pip install claude-agent-sdk` |
| `ssh: host unreachable` on dispatch | ssh config missing on driver | add to `~/.ssh/config` or `dda inv workspaces.cmd` |
| `working tree dirty; aborting iteration` | stray edits or orphan from prior crash | normally auto-handled by `startup_cleanup`; if not, `git checkout -- comp/observer tasks/q.py` |
| `upstream sync CONFIG` halts | someone pushed to `q-branch-observer` conflicting with `claude/observer-improvements` | rebase manually or reset scratch branch to new upstream tip |
| Bayesian / eval-bayesian appearing | it shouldn't — driver never invokes it | grep `journal.jsonl` for context |
| SDK retrying repeatedly | API rate limit or network | journal logs `slack_post_failed` or SDK transient; auto-recovers |

Full design / flow diagrams in [README.md](./README.md). Spec:
`~/.claude/plans/ad-harness.allium`. Plan: `~/.claude/plans/ad-harness-plan.md`.
