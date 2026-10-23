# Make SMP report generation and CI gating one obvious operation

## Approved local repair scope

Following a local review, the user approved a bounded repair of the existing
implementation. This approval supersedes the conflicting requirements in the
original handoff below:

- Keep `smp_ci.py` and `report_template.md.j2` as the production entry points.
- Use the Agent link configuration for every team; do not add link profiles.
- Trust schema-conforming v1 input; do not duplicate schema/count validation.
- Write outputs directly in the fresh CI workspace. Do not add atomic
  publication or stale-output cleanup infrastructure.
- Keep the CI Pass/Fail Decision section in Markdown to match the baseline
  PR-comment behavior.
- Fix the script and CI wiring, zero-data gate handling, error propagation,
  decision/marker consistency, and deferred job failure. Update documentation
  and add focused local tests.
- Leave experiment durations, gate selection, jobs, and the pinned SMP version
  unchanged. CI acceleration is outside this repair.
- Stop after local verification. Do not stage, commit, push, or run real CI.

## Original planning handoff (historical)

The remaining sections record the earlier planning handoff, not an approved
instruction to implement its full scope. Its unresolved investigations remain
future work; the local repair above is authorized independently.

## Context

The current draft CI migration in
`.gitlab/childs/smp-regression-child-pipeline.yml` splits one logical result across
several operations:

1. `test/regression/quality_gates.py` reads legacy `outputs/report.json`, writes a
   decision file, and returns a deferred policy exit code.
2. Shell stores that exit code in `gate_rc` across later GitLab `script` items.
3. A separate shell helper invokes `smp report render`.
4. Two `smp job result --signal ...` calls download/read backend regression and bounds
   decisions for Datadog tags.
5. The PR-comment job escapes Markdown with two `sed` calls, interpolates it into a
   shell-quoted JSON string, and invokes `curl`.
6. A final shell item turns the deferred quality-gate state into a tag and job exit.

The objective is one Python invocation that creates coherent local reporting and policy
outputs from a single report, followed by simple shell consumers. A valid policy
failure must still allow Markdown generation, JUnit upload, Datadog tagging, artifacts,
and the PR comment before the job exits 1. Operational errors must fail immediately and
must not look like a valid pass or policy failure.

## Decisions already made

These decisions came from the user and should not be reopened unless new code evidence
makes one impossible:

1. **Production Markdown:** use the existing
   `test/regression/parity_report_template.md.j2` behavior. Do not redesign or condense
   the GitHub report in this task.
2. **Decision display:** keep CI decisions outside the Markdown. The GitHub Markdown
   should remain a parity report.
3. **Script rendering scope:** the unified Python script owns the PR Markdown and policy
   outputs. JUnit remains a direct SMP built-in render in CI.
4. **Decision values:** use `passed` / `failed`, matching existing Datadog tag values.
5. **Severity view:** remove `severity_report.md`; the current v1 CLI does not provide
   that built-in.
6. **Job scope:** the intended production flow should work for the shared Agent and
   metal-runner CI base, but see the CI-acceleration requirement below before deciding
   which jobs remain in this change.
7. **No template rename/condensed work:** template renaming and the old condensed design
   are out of scope. The user archived that work elsewhere. Do not mention or modify a
   condensed template in the final plan.
8. **No 30-report corpus verification:** do not include a task to render the full parity
   corpus in this plan.
9. **Real CI gate:** after local implementation reaches sufficient confidence, stop and
   report back. Do not commit or push. The user will then explicitly allow commits and a
   push so the next phase can iterate against real Agent CI.

## New requirements from plan review

### 1. Temporary/initial CI acceleration

The first implementation task must reduce feedback time before changing report logic:

- remove as many non-essential CI jobs as safely possible;
- shorten the SMP experiment duration to 60 seconds;
- run only two quality gates.

The next session must inspect the child/parent pipeline and regression configuration to
identify the exact jobs and supported experiment-selection mechanism. These changes are
**temporary iteration scaffolding**: restore normal production jobs, gate selection, and
durations before presenting the final merge-ready diff.

Do not guess the YAML/config fields. The previous session was stopped before reading
them.

You're allowed to commit those changes in a dedicated commit

### 2. Legacy backend conversion

The current Agent-team SMP backend does **not** produce `report.v1.json`. The production
flow must therefore:

1. download `outputs/report.json` with `smp job sync`;
2. invoke `test/regression/convert_old_report_to_v1.py`;
3. write the converted `outputs/report.v1.json`;
4. use that v1 file for local decision computation and rendering.

The conversion is described as almost perfect, so the final plan must document its
known losses and decide which ones affect CI policy. The next session must read the
converter completely, including its CLI, error behavior, and output schema validation.
The previous attempt to read it was interrupted and returned no content.

### 3. One local decision replaces all backend result signals

The decision record should replace both `smp job result --signal regression-detector`
and `smp job result --signal bounds-check`, as well as the existing Agent quality-gate
script state.

Required shape:

```json
{
  "regressions": "passed",
  "all_bounds_checks": "passed",
  "job": "passed"
}
```

Each field is exactly `passed` or `failed` and exists only to feed subsequent
`datadog-ci tag` / final-gate shell operations:

| decision field | expected consumer |
|---|---|
| `regressions` | `smp_optimization_goal` Datadog job tag |
| `all_bounds_checks` | `smp_bounds_check` Datadog job tag |
| `job` | `smp_quality_gates` tag and deferred job failure marker |

The mapping above is the current interpretation of the feedback. The next session must
verify it against current policy and v1 fields before finalizing the plan.

Do not add summaries, counts, or failed-check arrays to this JSON. The source
`report.json`/converted `report.v1.json` and Markdown already contain diagnostic data.

Policy facts already established:

- `job` preserves the Agent rule: only bounds checks on experiments whose names start
  with `quality_gate_` gate the job.
- A quality gate passes only when it has data and every comparison replicate passed.
  In v1 terms: `total_count > 0` and `pass_count == total_count`.
- `erratic: true` is not an exemption from an Agent quality gate.
- Optimization-goal regressions are reported/tagged but do not by themselves fail the
  Agent job.
- `all_bounds_checks` is intentionally broader than `job`.
- Planning analysis previously compared this v1 quality-gate rule with the existing
  corpus manifest's Agent CI result and found 30/30 agreement, including the important
  bounds-failed/Agent-CI-passed case. This is evidence only; do not add a full-corpus
  rerender task to the final plan.

The next session must derive the exact local predicates for `regressions` and
`all_bounds_checks` from the converted v1 schema and compare them with the current SMP
backend signal semantics. In particular, inspect how configured/runtime erratic state
affects each backend signal so removing `smp job result` does not silently change tags.

### 4. Generate the complete PR-comment JSON payload in Python

The unified script should publish both Markdown and the exact payload consumed by the
PR-comment service:

```json
{
  "org": "DataDog",
  "repo": "datadog-agent",
  "commit": "<CI_COMMIT_SHA>",
  "header": "Regression Detector",
  "message": "<rendered Markdown>"
}
```

Use Python JSON serialization. Do not use shell `sed` escaping or interpolate Markdown
inside a quoted shell JSON string. The payload's decoded `message` must equal the
published Markdown exactly, including newlines, quotes, backslashes, and Unicode.

The detector job should publish this file as an artifact. The PR-comment job should
validate that it exists and send it directly, for example with
`curl --data-binary @outputs/pr_comment_payload.json`.

## Target output contract

Names can be adjusted during final planning if the converter already imposes stronger
conventions, but the intended contract is:

| output | meaning |
|---|---|
| `outputs/report.v1.json` | converted v1 source used by all local logic |
| `outputs/report.md` | parity-template GitHub Markdown |
| `outputs/pr_comment_payload.json` | complete JSON payload for the PR-comment service |
| `outputs/decision.json` | only the three `passed` / `failed` fields above |
| `outputs/smp-failed` | zero-byte marker present iff `decision.job == "failed"` |
| `outputs/junit.xml` | direct SMP v1 built-in output, outside the unified Python script |

The marker is transient shell control; `decision.json` is the durable artifact. Whether
the marker itself should be uploaded is not required and should default to no.

## Recommended script boundary

Evolve `test/regression/render_parity_report.py` into a production wrapper (final name
to confirm after inspecting the converter). It should:

1. accept explicit paths for SMP, template, converted v1 input, Markdown output,
   PR-comment payload output, decision output, failure marker, and PR commit;
2. accept an explicit link profile if all shared jobs remain;
3. remove stale contract outputs before work, because `job sync` may download old
   server-rendered files;
4. require the exact v1 `$schema` and validate values used for decisions;
5. compute all three decision fields once from the parsed v1 object;
6. build symbolic `extra.links` from the same object;
7. render the parity template with `smp report render --report ... --extra ...`;
8. construct the PR-comment object from the returned Markdown string;
9. serialize Markdown, payload, and decision to temporary files in their destination
   directories and publish with `os.replace` only after rendering succeeds;
10. create/remove the failure marker last from `decision.job`;
11. return zero for both valid pass and valid policy failure; return nonzero for input,
    conversion-adjacent, render, publication, or marker errors.

After a zero exit, Markdown, decoded payload message, three decision values, and marker
must all describe the same converted report. After an operational failure, stale local
outputs must not survive and masquerade as success.

## Link profiles (if shared jobs remain)

Existing planning found a real suite difference:

- Agent reports configure metrics + profiles.
- Metal-runner reports configure metrics but not profiles.
- Agent experiment report links apply to current `quality_gate_*` cases and
  `python_openmetrics`.
- Metal-runner experiment report links apply to `tcp_rr`.
- Both suites use per-experiment logs and failed-replicate debug links.

If metal jobs remain, use explicit typed link profiles selected from an existing stable
CI variable such as `SMP_TEAM_NAME`; unknown values must fail. Do not infer a suite from
experiment names. If acceleration removes metal jobs permanently, simplify the plan to
an Agent-only profile rather than retaining unused abstraction.

## CI flow to target

After `job sync`, the intended sequence is:

1. guard legacy `outputs/report.json`;
2. convert it to `outputs/report.v1.json`, classifying conversion failure as an
   operational reporting failure;
3. invoke the unified generator once;
4. render `junit.xml` directly from v1 (the current debug CLI supports `junit.xml [v1]`);
5. print the Markdown and upload JUnit;
6. read the three decision values with strict `jq -er` selectors;
7. validate each value is `passed` or `failed`;
8. validate `decision.job == "failed"` iff `outputs/smp-failed` exists;
9. send the three values directly to their corresponding `datadog-ci` tags;
10. remove both `smp job result --signal ...` calls and their dependency on downloaded
    signal decisions;
11. set the normal operational failure-mode tag;
12. as the final policy operation, exit 1 iff `outputs/smp-failed` exists.

The PR-comment job should consume the generated payload directly and remove:

- `report_as_json_string.txt`;
- both Markdown `sed` escaping passes;
- shell construction of `PR_COMMENT_JSON_PAYLOAD`.

## Files expected to change

Final paths depend on the next session's converter and acceleration investigation.
Expected scope:

- `.gitlab/childs/smp-regression-child-pipeline.yml`
- possibly `.gitlab/test/functional_test/regression_detector.yml` if non-essential jobs
  are defined/triggered there
- `test/regression/render_parity_report.py` (likely renamed to a production name)
- `test/regression/parity_report_template.md.j2` only if usage comments need updating;
  no visual/report redesign
- `test/regression/convert_old_report_to_v1.py` only if a correctness gap is found;
  prefer reusing it unchanged
- `test/regression/quality_gates.py` (remove after policy is merged)
- `test/regression/README.md`
- the exact experiment/config files required for 60-second runs and two gates
- `PLAN.md`

Explicitly out of scope:

- condensed report/template work;
- full parity-corpus rerendering;
- unrelated SMP Rust changes;
- commits, pushes, or real-CI iteration before the user grants the second-phase
  permission.

## Tasks to finalize this plan

The next planning session should perform these tasks in order and update this same file
after each discovery:

- [ ] Read `test/regression/convert_old_report_to_v1.py` completely. Record its CLI,
      inputs/outputs, schema checks, exit behavior, dependencies, and known lossy fields.
- [ ] Trace the current SMP backend signal predicates for regression and all-bounds
      decisions. Map them exactly to v1 fields and document any unavoidable behavior
      change from local computation.
- [ ] Inspect the parent and child GitLab YAML to list every job triggered by this flow.
      Propose the smallest retained set that still runs the Agent experiment and posts
      its PR comment; identify exactly which jobs to remove.
- [ ] Find the supported mechanism to select only two quality-gate experiments. Name
      those two gates and explain why they cover both pass/fail reporting paths or how a
      temporary fixture will cover failure.
- [ ] Find the exact duration field(s) for those gates and confirm that 60 seconds is
      valid. Determine whether setup/warmup durations also dominate feedback time.
- [ ] Define an explicit restoration checkpoint for the temporary acceleration edits:
      restore production jobs, gates, and durations after CI iteration and verify the
      merge-ready diff contains no speed-only changes.
- [ ] Decide whether metal-runner jobs remain. If yes, retain explicit Agent and
      metal-runner link profiles; if no, simplify to Agent only.
- [ ] Confirm the production script filename and exact CLI, accounting for the
      converter's own interface.
- [ ] Confirm artifact lists after removing backend result-signal dependence. Decide
      whether raw signal JSON remains useful debugging evidence or should also be
      removed.
- [ ] Specify the exact jq/tag shell snippet for all three decision fields and the
      decision/marker consistency check.
- [ ] Specify fail-closed cleanup/atomic publication behavior when conversion, SMP,
      JSON serialization, or filesystem publication fails.
- [ ] Update the implementation checklist and verification section from the findings,
      then submit the completed plan through Plannotator.

## Verification outline for the finalized plan

Do not add full-corpus rendering. Use focused local fixtures and stop before any push.
The final plan should include:

- pass, optimization-regression, all-bounds-failure, Agent-quality-gate-failure,
  broad-bounds-failed/Agent-job-passed, and no-gates decision scenarios;
- malformed legacy JSON, converter failure/loss, wrong v1 schema, impossible counts,
  SMP render failure, JSON publication failure, and stale-output cleanup;
- exact assertion that `json.load(pr_comment_payload)["message"] == report.md` for
  Markdown containing quotes, backslashes, Unicode, and newlines;
- exact assertion that marker presence equals `decision.job == "failed"`;
- both link profiles if metal jobs remain;
- v1 JUnit rendering with the exact command shipped in YAML;
- an offline extraction/execution of the changed shell for pass, policy fail,
  operational fail, and decision/marker mismatch;
- GitLab YAML validation, shellcheck, relevant Python formatting/linting, and
  `git diff --check`;
- a final report to the user describing evidence and residual risks.

At that point stop. The user will decide whether to authorize commits/push and real
Agent CI iteration.
