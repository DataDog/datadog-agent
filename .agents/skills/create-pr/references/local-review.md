# Local review details for /create-pr

Load this file from step 5 of `SKILL.md` when `scripts/review.py` exits with anything other than 0 or doesn't finish in time, or when you need to know which reviewer it picks.

## Reviewer order

`scripts/review.py` uses the first reviewer that is available and isn't your own provider, and tries the next one if a review fails:

1. `codex` on the host, the same provider as CI, so findings should match what CI will post
2. `claude` on the host
3. `claude` in a `dda env dev` developer environment, which needs docker and authenticates through `ddtool`. The environment is kept for later runs. When git or npm doesn't work inside it, the script removes it, so the next run recreates it with newer `dda` and image versions.

So `--self claude` only tries `codex`, and `--self codex` tries `claude` on the host, then in the developer environment.

Without `--self`, or with an empty value, the script detects the provider from the environment: `CLAUDECODE` for Claude Code, `CODEX_*` for Codex. When it finds neither, or both (e.g. Codex started from a Claude Code shell), it skips no reviewer. Pass `--self` whenever you know your provider.

## When the review doesn't complete

Handle each outcome as follows; each one tells the user why the usual review didn't run, so they can fix it for next time:

| Outcome | What to do |
| --- | --- |
| Exit 1: every attempted review failed | Tell the user, with the errors, then do a subagent review. |
| Exit 2: no reviewer from another provider is available | Show the user the reasons the script printed, then do a subagent review. |
| Exit 3: the script can't run as invoked (an invalid `--self` value, or not in a git checkout) | No review ran. Fix the invocation (`--self` takes `claude` or `codex`, lowercase; run it from the checkout) and run it again. |
| No result after 20 minutes | Stop the script (`TaskStop` in Claude Code): it only prints locally, but output arriving later would give the user a second set of findings to reconcile. If your harness can't stop it, ignore its output when it arrives and say so. Then do a subagent review rather than stalling PR creation. |

## Subagent review

Start the subagent with the `Agent` tool in Claude Code, or a spawned agent in Codex, in the background so the rest of `/create-pr` can go on, and wait for its answer before you present the draft PR. It brings a fresh context, not a different model, so it is less independent than a review by the other provider; say so when you report it. If your harness can't start subagents, tell the user that no independent review ran. Give it the code, not your conclusions: don't pass your own summary of the change or what you think is risky, so its review isn't anchored on yours. Fill in this prompt and pass nothing else:

```text
Review the branch <branch> against <base> in the repository at <repo path>.
Read the change yourself with `git diff <base>...HEAD`, and the changed files in full for context.
Apply these review guidelines: <root codereview_guideline.md>, <each codereview_guideline.md relevant to the changed files>.
Report correctness bugs and guideline violations only. Don't edit any files.
Answer with one line per finding:
  <path>:<line> — <the defect, in one sentence> — <concrete failure scenario: inputs or state, then the wrong outcome>
If you find nothing real, answer exactly: No findings.
```

Read the answer before you report it: a list of findings, `No findings.`, or anything else, which means the review failed. Tell the user which of the three it was, for example:

```text
Local review (subagent, claude: codex wasn't available, so this isn't the reviewer CI uses):
- .agents/skills/foo/scripts/run.py:42 — `--dry-run` is ignored after the first argument — `run.py x --dry-run` deletes the files it should only list
```

## Script tests

`scripts/test_review.py` covers argument parsing, provider detection and which reviewers are tried for each `--self`, without running any reviewer. After changing `scripts/review.py`, run `python3 .agents/skills/create-pr/scripts/test_review.py`.
