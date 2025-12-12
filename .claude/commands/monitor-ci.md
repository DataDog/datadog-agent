---
description: Monitor CI for the current branch and block until it finishes.
model: haiku
---

# Monitor CI Command

Monitor CI pipelines for the current branch until completion, checking both GitLab CI (via Datadog) and GitHub Actions (via gh CLI).

## Instructions

You MUST follow these monitoring rules:

### Sleep Pattern (CRITICAL)
- Use `Bash(sleep N; echo "message")` - blocking, NOT background
- Echo messages must be PLAIN STRINGS - NO subshells, NO command substitution
- ✗ WRONG: `echo "Status: $(cmd)"` or `echo "Time: $(date)"`
- ✓ CORRECT: `echo "Checking status"` or `echo "CI checkpoint"`

### Monitoring Workflow

#### 1. Initial Setup
```bash
# Get current branch and commit
git branch --show-current
git rev-parse HEAD
```

#### 2. Check for PR (determines required checks)
```bash
# Try to find associated PR
gh pr view --json number,url,statusCheckRollup
# OR if that fails: gh pr list --head <branch> --json number,url
```

**Key insight:** GitHub knows which checks are "required" via branch protection rules. Use `gh pr checks` as the source of truth for required vs optional.

#### 3. Initial Status Check (parallel)
Check both sources immediately:

**A. Datadog - GitLab CI jobs:**
```
Query: @git.branch:<current-branch> ci_level:pipeline from:now-2h
```
If pipeline found, get job details:
```
Query: @ci.pipeline.id:<id> ci_level:job page_limit:50
```

**B. GitHub - All checks:**
```bash
gh pr checks
```
This shows ALL checks (GitLab + GitHub Actions) with their required status.

#### 4. Monitoring Loop
While any checks are still running:

- **First 20 minutes**: Check every 2-3 minutes
  ```bash
  sleep 180; echo "CI checkpoint"
  ```

- **After 20 minutes**: Check every 5 minutes
  ```bash
  sleep 300; echo "CI checkpoint"
  ```

After each sleep, check BOTH sources:
1. `gh pr checks` - quick overview of all checks
2. Datadog query for any newly failed jobs

Stop when: All checks show completed status (success, failure, or timeout)

#### 5. Final Report (when complete)

**CRITICAL: DO NOT trust pipeline-level status alone**

Query ALL failed jobs from Datadog:
```
Query: @ci.pipeline.id:<id> @ci.status:error ci_level:job page_limit:50
```

Get final GitHub checks status:
```bash
gh pr checks
```

### Reporting Requirements

**Structure your report as:**

1. **Summary Line:**
   - "CI complete: X/Y checks passed"
   - If any required checks failed: "❌ REQUIRED checks failed - cannot merge"
   - If only optional checks failed: "⚠️ Optional checks failed - can merge with caution"

2. **Required Check Failures (if any):**
   ```
   REQUIRED FAILURES (blocking merge):
   - [check-name] - [source: GitLab/GitHub] - [error summary] - [duration]
   ```

3. **Optional Check Failures (if any):**
   ```
   OPTIONAL FAILURES (non-blocking):
   - [check-name] - [source: GitLab/GitHub] - [error summary] - [duration]
   ```

4. **Detailed Errors:**
   - For GitLab jobs: Include error message from Datadog
   - For GitHub Actions: Include failure reason from `gh` output

5. **Action Items:**
   - Required failures: "Fix X before merging"
   - Optional failures: "Consider fixing Y" or "Known flaky test Z"

**Language rules:**
- Use "required" and "optional" (NOT "critical", "blocking", "non-blocking")
- Never say "Pipeline succeeded" if ANY check failed
- Always enumerate ALL failures, even optional ones
- Distinguish between GitLab jobs (from Datadog) and GitHub Actions (from gh)

### Why This Works

**Problems we're solving:**
1. ❌ Old: Trusted pipeline status without checking jobs
   ✅ New: Always query individual job status

2. ❌ Old: Hardcoded "critical" job names that go stale
   ✅ New: Use GitHub's branch protection to determine required checks

3. ❌ Old: Checked Datadog and GitHub separately/sequentially
   ✅ New: Check both in parallel, cross-reference results

4. ❌ Old: Missed GitHub-only workflows (Label analysis, etc.)
   ✅ New: `gh pr checks` catches ALL checks

**Data source strengths:**
- **GitHub (`gh pr checks`)**: Source of truth for required/optional, sees all checks
- **Datadog CI**: Detailed error messages and timing for GitLab jobs, some GitHub Actions

## Example Usage

**User:** "Monitor CI until it completes"

**Assistant executes:**
1. Gets current branch: `sopell/feature-x`
2. Finds PR #12345
3. Initial check shows: 8 checks running (6 GitLab via Datadog, 2 GitHub Actions)
4. Monitors with 3-min intervals for 15 minutes
5. At completion:
   - Queries Datadog: `@ci.pipeline.id:86532850 @ci.status:error ci_level:job`
   - Runs: `gh pr checks`
6. Reports:
   ```
   CI complete: 7/8 checks passed
   ⚠️ Optional checks failed - can merge with caution

   OPTIONAL FAILURES (non-blocking):
   - single-machine-performance-regression_detector - GitLab - Timeout after 70min - Optional performance test

   All required checks passed ✓

   Action items:
   - Optional: Investigate performance regression timeout (known issue, tracked in #issue-123)
   - Ready to merge when approved
   ```

**User:** "Monitor CI"

**Assistant executes:**
1. Gets current branch: `main`
2. No PR found (direct push to main or test branch)
3. Can only monitor via Datadog GitLab CI
4. Reports:
   ```
   GitLab CI complete: pipeline 86532851
   2/5 jobs failed

   FAILED JOBS:
   - lint_python - Error: "golangci-lint found 3 issues" - 4min
   - unit_tests - Error: "TestFoo failed" - 8min

   Note: Cannot determine which jobs are required (no PR). All failures shown above.
   Check branch protection rules to see if these are required.
   ```
