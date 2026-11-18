# Test SMP feedback markers in your own session

Use this guide to evaluate Single Machine Performance (SMP) feedback from a local coding-agent session.
The audience is Agent contributors and SMP maintainers who want to review candidate feedback signals.

The helper runs the real Trajectory marker evaluator in a temporary database.
It copies only the session you select and opens the source cache read-only.
The copy includes completed turns and their tool calls. It does not follow child sessions.
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

4. Let the agent finish its answer. You can keep the session open.
5. In a separate terminal, list the Claude sessions in your Trajectory cache:

   ```sh
   python3 .trajectory/evaluate_smp.py \
     --list-sessions \
     --client claude-code \
     --cache-db "$HOME/.trajectory/trajectories/cache/trajectory.db"
   ```

   Select the row with your test checkout and a recent captured turn.
   Copy the full value from the `TRAJECTORY SESSION ID` column.
   The listing shows metadata, not prompts or responses. It does not need the Trajectory executable or Datadog credentials.

   Remove `--client claude-code` to see other clients.
   The listing shows the 20 most recent matches, ordered by their latest captured turn.
   This also finds resumed sessions that started earlier.

## Use a captured session ID

The evaluator needs the exact `session_id` stored in the selected Trajectory cache.
A Claude session ID can match that value, but its presence in Claude does not prove that Trajectory captured it.
Do not guess a prefix or substitute another UUID.

`trajectory status` shows a summary of recent captured work.
Its normal display in Trajectory 0.6.2 does not show the full ID needed here and can show another client's session.
Use the listing above to select by client, checkout, and capture time.

If your new Claude session is absent, there is no matching cached record to evaluate yet.
Check capture setup, cache location, and capture delay before you select another ID.
For client-specific checks, read `trajectory user-guide clients/claude-code`.
A Datadog keychain publishing warning is separate from this local lookup.

## Evaluate the selected session

Replace `SESSION_ID` with the full ID from the listing:

```sh
python3 .trajectory/evaluate_smp.py \
  --session SESSION_ID \
  --cache-db "$HOME/.trajectory/trajectories/cache/trajectory.db"
```

Add `--trajectory "$HOME/.trajectory/bin/trajectory"` if needed.
If your capture uses another cache path, pass that path to `--cache-db`.

Expect `smp-capability-question` and `smp-expressed-wish` findings with your prompts and the agent's responses.
Other findings depend on what the agent said or did.

The helper evaluates a snapshot of completed turns. It skips unfinished turns and tool calls from those turns.
For an open session, the report states that the session is still open and shows the number of skipped turns.
Repeat the same command after another answer to evaluate a new snapshot.
The helper does not collect continuously or add its results to earlier runs.

Trajectory can publish turn-level data after each completed turn. Session completion is not necessary for that data.
Session-end totals still depend on the session ending. Destination settings and capture delays affect when remote data appears.

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
- Missing cache: check the path and use straight shell quotes, as shown in the code blocks.
- Missing session: run `--list-sessions` with the same cache. Confirm the client, project, and latest captured turn.
- Only old Claude sessions appear: check Claude capture or cache ingestion. A native Claude transcript alone is not a Trajectory cache record.
- No completed turns: wait for an answer to finish and for Trajectory to update the local cache, then try again.
- No findings: inspect the wording and confirm that the cache contains the prompts and responses.
- SQLite schema error: the helper uses Trajectory's internal cache schema. Check the installed version against the tested version.

For this pilot, a completed turn has an end timestamp and an `ok` or absent lifecycle status.
The helper skips unknown lifecycle states rather than treating them as complete.

## Collection after the pilot

Committing the marker file does not enable automatic collection for every contributor.
The background service needs org or user markers, or an explicit `TRAJECTORY_PROJECT_MARKERS_PATH` in its environment.
That environment setting applies to every session the service processes.

For a team rollout, agree on rule quality, distribute the active pack, verify capture across clients, and verify metric delivery.
Request summaries also need task extraction, deduplication, and an approved team destination for derived records.
The PR description lists the rollout work separately from this local test.

See the [Trajectory repo marker guide](https://datadoghq.atlassian.net/wiki/spaces/Trajectory/pages/6959432755/Docs+Repo-Level+Marker+Guide)
and [marker reference](https://datadoghq.atlassian.net/wiki/spaces/Trajectory/pages/6934462849/User+Guide+Markers).
