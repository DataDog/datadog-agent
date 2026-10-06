
## Replay over many pipelines

Replay the evaluation in batch over the branch's pipelines (or explicit
pipeline ids), to measure Jev on many samples at once - from ANY checkout:

each pipeline's Jev decisions are computed from its own commit (a temporary
worktree checked out at the pipeline's SHA provides the diff and test
sources), so the current branch does not matter:

```bash
dda inv dyntest.replay-jev-evaluation \
  --ref kfairise/jev-e2e-selection-eval --limit 30 \
  --output /tmp/jev-replay.json
```

- Targets: explicit `--pipeline-id` (repeatable) and/or the most recent
  `--limit` pipelines of `--ref`. Pipelines without completed e2e test
  jobs, whose commit can no longer be fetched, or whose events are past CI
  Visibility retention are skipped with a report, never failures.
- Per pipeline: the same executor index (GitLab completed e2e jobs + the
  committed candidates file) and the shared evaluation flow; the executed
  tests come from ONE pipeline-wide CI Visibility query (30-day window)
  instead of one query per job.
- Output: one line per pipeline and an aggregate (executed test
  occurrences, the would-skip rate, and per-test miss counts - a failing
  executed test Jev would have skipped, counted per occurrence across
  pipelines); `--output` writes the full JSON.
