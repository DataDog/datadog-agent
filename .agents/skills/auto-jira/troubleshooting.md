# Troubleshooting

Common issues and recovery patterns for auto-jira in the datadog-agent.

**Read this when:** something is not working and you need to debug or recover.

---

## General Debugging Checklist

1. Check `AUTO_JIRA.md` for notes from previous iterations
2. Search Confluence for relevant team processes or known issues before guessing
3. Re-read the ticket comments — context is often added after the ticket was created
4. Check if the issue is already fixed or addressed in a recent PR

---

## Authentication Failures

### Jira MCP authentication fails

**Symptom:** MCP tool calls return auth errors.

**Action:**
1. STOP immediately. Do not work around it.
2. Mark the current ticket HOLD with a comment explaining the auth failure.
3. Print `TASK FAILED` and report the issue to the user.

Do not attempt to use `curl` with hardcoded credentials as a workaround.

### GitHub authentication fails

**Symptom:** `gh` commands return 401 or "not authenticated".

**Action:**
1. Check: `gh auth status`
2. If expired: `gh auth login` — but this requires interactive input, so stop and ask the user to re-authenticate.
3. Mark the current ticket HOLD and report.

---

## Git Issues

### Merge conflicts

When rebasing onto `main` produces conflicts:
1. Read the conflicting files carefully
2. If the conflict is simple (overlapping edits to different logic): resolve it
3. If the conflict requires understanding the other PR's intent: mark ticket HOLD, add a comment explaining the conflict, ask for human help
4. Never resolve conflicts by blindly accepting one side

```bash
git rebase origin/main
# resolve conflicts in editor
git add <resolved files>
git rebase --continue
git push --force-with-lease
```

### Force push rejected

If `--force-with-lease` fails (someone else pushed to your branch — unlikely but possible):
```bash
git fetch origin
git log origin/auto-jira/<KEY>-slug..HEAD  # see what they added
```
If it is just CI-added commits, reset to your work and re-push. If it is something unexpected, stop and ask.

### Pre-commit hook failures

If a pre-commit hook fails:
1. Read the error carefully — hooks exist for a reason
2. Fix the underlying issue the hook is flagging
3. Do NOT use `--no-verify`
4. Common hooks: license headers, file size limits, secret detection

---

## CI Issues

### CI not starting

Wait 5 minutes, then check:
```bash
gh pr checks <PR_NUMBER>
```

If still no checks:
- Ensure the branch was pushed: `git log origin/auto-jira/<KEY>-slug`
- Check if the PR is still a draft (some CI only runs on ready PRs)
- Try closing and re-opening the PR

### CI stuck / no progress

If a check has been "in progress" for more than 2 hours:
- For GitLab: check the pipeline page for a queue or infra issue
- Re-trigger: `gh run rerun <RUN_ID>` or retry the GitLab pipeline

### Same CI failure after 3 fix attempts

Mark the ticket HOLD. Add a comment with:
- What the failure is
- What was tried (all 3 fix attempts)
- Link to the failing CI job
- What human action would help

---

## Jira Transition Failures

### Transition to "In Progress" fails

Available transitions vary by project configuration. If "In Progress" is not available:
```
mcp__atlassian__getTransitionsForJiraIssue(
  cloudId="datadoghq.atlassian.net",
  issueIdOrKey="<KEY>"
)
```

Look for the closest equivalent (e.g., "Start Progress", "Begin Work"). Use that transition ID.

If no in-progress transition is available at all, continue working but note it in `AUTO_JIRA.md`.

### Cannot assign ticket to self

Some projects require specific permissions to self-assign. If assignment fails:
1. Skip the claim step
2. Continue with implementation
3. Note in the PR description that self-assignment was not possible

---

## PR Issues

### PR creation fails

Check that:
- The branch was pushed successfully: `git log origin/auto-jira/<KEY>-slug`
- You are not already on a PR for this branch: `gh pr list --head auto-jira/<KEY>-slug`
- The title does not contain characters that break shell quoting — use a HEREDOC for the body

### Merge conflicts after PR is created

If `main` has moved while the PR was in review:
```bash
git fetch origin
git rebase origin/main
# resolve any conflicts
git push --force-with-lease
```

---

## Recovery Patterns

### Abandoning a PR

If a PR is stuck and cannot be fixed:
1. Close the PR: `gh pr close <number> --comment "Abandoning: <reason>"`
2. Delete the branch: `git push origin --delete auto-jira/<KEY>-slug`
3. Transition the Jira ticket to HOLD with explanation
4. Update `AUTO_JIRA.md`

### Starting fresh on a ticket

If the branch is in a bad state:
```bash
git checkout main
git pull
git branch -D auto-jira/<KEY>-slug           # delete local
git push origin --delete auto-jira/<KEY>-slug  # delete remote (if exists)
git checkout -b auto-jira/<KEY>-slug
```

Then re-implement from scratch.

### Resuming after interruption

Check `AUTO_JIRA.md` first. Then:
```bash
gh pr list --author @me --state open
```

For each open PR, check CI status and review status. Resume from where you left off.

---

## Self-Improvement

If you solve a hard problem or discover something not documented here, update this file before moving to the next ticket. Future iterations benefit from what you learn now.
