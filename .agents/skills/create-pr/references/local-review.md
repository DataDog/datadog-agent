# Local review details for /create-pr

Load this file from step 5 of `SKILL.md` when you need to know which reviewer the script picks, or when you start a subagent review.

## Reviewer order

`scripts/review.sh` uses the first reviewer that is available and isn't your own provider, and tries the next one if a review fails:

1. `codex` on the host, the same provider as CI, so findings should match what CI will post
2. `claude` on the host
3. `claude` in a `dda env dev` developer environment, which needs docker and authenticates through `ddtool`. The environment is kept for later runs.

So `--self claude` only tries `codex`, and `--self codex` tries `claude` on the host, then in the developer environment.

Without `--self`, or with an empty value, the script detects the provider from the environment: `CLAUDECODE` for Claude Code, `CODEX_*` for Codex. When it finds neither, or both (e.g. Codex started from a Claude Code shell), it skips no reviewer. Pass `--self` whenever you know your provider.

## Subagent review

Start the subagent with the `Agent` tool in Claude Code, or a spawned agent in Codex. Give it the code, not your conclusions: don't pass your own summary of the change or what you think is risky, so its review isn't anchored on yours. Fill in this prompt and pass nothing else:

```text
Review the branch <branch> against <base> in the repository at <repo path>.
Read the change yourself with `git diff <base>...HEAD`, and the changed files in full for context.
Apply these review guidelines: <root codereview_guideline.md>, <each codereview_guideline.md relevant to the changed files>.
Report correctness bugs and guideline violations only. Don't edit any files.
Answer with one line per finding:
  <path>:<line> — <the defect, in one sentence> — <concrete failure scenario: inputs or state, then the wrong outcome>
If you find nothing real, answer exactly: No findings.
```

Read the answer before you report it: a list of findings, `No findings.`, or anything else, which means the review failed. Tell the user which of the three it was.
