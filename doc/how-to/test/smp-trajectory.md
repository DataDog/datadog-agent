# Test SMP feedback markers in your own session

Use this guide to evaluate Single Machine Performance (SMP) feedback from a local coding-agent session.
The audience is Agent contributors and SMP maintainers who want to review candidate feedback signals.

The helper runs the real Trajectory marker evaluator in a temporary database.
It copies only the session you select and opens the source cache read-only.
The copy includes its session, turn, and tool-call rows. It does not follow child sessions.
It prints matching turns, then removes the temporary database.

It does not install hooks, change capture settings, call a model, or publish results.
Your normal coding-agent session still follows your existing capture and publishing settings.

## Before you start

- Check out this PR branch.
- Use Python 3 and an installed Trajectory binary. Validation used Trajectory 0.6.2 on macOS.
- For your own session, use a coding agent that Trajectory already captures.
- Run the commands below from the repository root.

The SMP CLI is optional for questions and wishes.
The demo does not execute SMP or any command from its fixtures.

## Run the synthetic demo

```sh
python3 .trajectory/evaluate_smp.py --demo
```

If `trajectory` is not on `PATH`, pass its installed path:

```sh
python3 .trajectory/evaluate_smp.py --demo \
  --trajectory "$HOME/.trajectory/bin/trajectory"
```

Expect nine `PASS` lines and `9 cases passed.`
The helper also prints the developer prompt and agent response for each match.
For example:

```text
smp-agent-claimed-limit | session=smp-demo-claimed-limit-without-cli-call | turn=1
  Developer: Compare these two saved performance reports.
  Agent: SMP cannot compare these report formats. I will write a custom script instead.
```

This case has no SMP tool call.
It shows how conversation evidence can reveal a perceived limit that CLI failure counts miss.
The claim is synthetic and does not describe the actual SMP feature set.

Two cases show detection limits:

- `quoted-limit-needs-review` matches an outdated statement that the agent quotes. A reviewer must reject this candidate.
- `implicit-need-not-detected` has a custom script but no SMP reference. These rules miss it.

## Create your own test session

1. Start a new coding-agent session in this checkout. Use your normal client and capture settings.
2. Ask a capability question, for example:

   ```text
   Does SMP support comparing two saved performance reports?
   Check the available documentation. Do not run SMP or change files.
   ```

3. Add a wish in another turn:

   ```text
   I wish SMP could export a comparison as a Markdown table.
   Describe whether the current CLI supports this. Do not run SMP or change files.
   ```

4. Let the agent answer, then end that coding-agent session.
5. In a separate terminal, find the session ID:

   ```sh
   trajectory status
   ```

   The command shows the latest captured session.
   If you have several active sessions, select the one that contains your test prompts.

## Evaluate the selected session

Replace `SESSION_ID` with the ID from the previous step:

```sh
python3 .trajectory/evaluate_smp.py \
  --session SESSION_ID \
  --cache-db "$HOME/.trajectory/trajectories/cache/trajectory.db"
```

Add `--trajectory "$HOME/.trajectory/bin/trajectory"` if needed.
If your capture uses another cache path, pass that path to `--cache-db`.

Expect `smp-capability-question` and `smp-expressed-wish` findings with your prompts and the agent's responses.
Other findings depend on what the agent said or did.
The helper refuses an unfinished session so it does not treat partial capture as a complete result.
If capture has not finalized, wait for Trajectory to record the session end and try again.

The report contains captured conversation text. Share real-session reports only through a destination approved for that content.

## Review and refine the rules

For each candidate, record:

| Field | What to review |
| --- | --- |
| Requested outcome | What did the developer want? |
| Signal | Question, wish, agent claim, or command failure? |
| Support status | Confirmed support, confirmed limit, or unknown? |
| Next action | SMP use, workaround, another tool, or stopped work? |
| Cause | Feature gap, discovery, documentation, access, defect, or agent misuse? |

An agent claim is useful feedback even if SMP supports the task.
Keep that original claim separate from the cause you confirm during review.
Count repeated signals from one task as one need when you summarize demand.

Edit `.trajectory/markers.yaml`, then run the demo and your session evaluation again.
Both modes use the repo file explicitly. They do not depend on the background service loading it.

The regular expressions find candidate phrases, not confirmed requests or task outcomes.
They can match quoted text and miss paraphrases, pronouns, or needs that never mention SMP.
Shell detection also depends on the client tool name, command format, and recorded success status.
The helper shows turn context, not an automatic summary of the workaround or cause.

## If evaluation fails

- Missing binary: use `--trajectory` with the installed executable path.
- Missing session: check the selected ID and source cache path.
- Unfinished session: end the source session and wait for capture to finalize.
- No findings: inspect the wording and confirm that the cache contains the prompts and responses.
- SQLite schema error: the helper uses Trajectory's internal cache schema. Check the installed version against the tested version.

## Collection after the pilot

Committing the marker file does not enable automatic collection for every contributor.
The background service needs org or user markers, or an explicit `TRAJECTORY_PROJECT_MARKERS_PATH` in its environment.
That environment setting applies to every session the service processes.

For a team rollout, agree on rule quality, distribute the active pack, verify capture across clients, and verify metric delivery.
Request summaries also need task extraction, deduplication, and an approved team destination for derived records.
The PR description lists the rollout work separately from this local test.

See the [Trajectory repo marker guide](https://datadoghq.atlassian.net/wiki/spaces/Trajectory/pages/6959432755/Docs+Repo-Level+Marker+Guide)
and [marker reference](https://datadoghq.atlassian.net/wiki/spaces/Trajectory/pages/6934462849/User+Guide+Markers).
