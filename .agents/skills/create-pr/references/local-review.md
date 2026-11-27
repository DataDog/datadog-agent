# Local review details for /create-pr

## Reviewers

`dda review --provider auto` skips the agent it detects from the environment (`CLAUDECODE` for Claude Code, `CODEX_*` for Codex, `GEMINI_CLI` for Gemini CLI) and tries, in order, until a review succeeds:

1. `codex` on the host, the same provider as CI
2. `claude` on the host
3. `gemini` on the host
4. `claude` in a `dda env dev` developer environment, which needs docker. It is kept for later runs, and recreated when it is too old.

When it detects no agent or several, it skips none. Other harnesses, such as opencode or pi, aren't providers yet: from one of them, `auto` tries every provider. The logic is in `tasks/libs/code_review/auto.py`.

## Subagent review

Start the subagent in the background (the `Agent` tool in Claude Code, a spawned agent in Codex), and wait for its answer before you present the draft PR. Fill in this prompt and pass nothing else, not your own summary of the change or what you think is risky:

```text
Review the branch <branch> against <base> in the repository at <repo path>.
Read the change yourself with `git diff <base>...HEAD`, and the changed files in full for context.
Apply these review guidelines: <root codereview_guideline.md>, <each codereview_guideline.md relevant to the changed files>.
Report correctness bugs and guideline violations only. Don't edit any files.
Answer with one line per finding:
  <path>:<line> — <the defect, in one sentence> — <concrete failure scenario: inputs or state, then the wrong outcome>
If you find nothing real, answer exactly: No findings.
```

If your harness can't start subagents, tell the user that no independent review ran.

Read the answer before you report it: a list of findings, `No findings.`, or anything else, which means the review failed. Tell the user which of the three it was, and that the reviewer is a subagent of your own provider, for example:

```text
Local review (subagent of claude, since no other provider could review):
- .agents/skills/foo/scripts/run.py:42 — `--dry-run` is ignored after the first argument — `run.py x --dry-run` deletes the files it should only list
```

