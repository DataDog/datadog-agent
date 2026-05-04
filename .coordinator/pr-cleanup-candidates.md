# Observer AD PR Cleanup Candidates

Recorded on 2026-05-04 after copying useful candidate commits into
`ella/observer-ad-manual-eval-20260430`.

Do not delete branches yet. Wait until the manual eval matrix is complete and
final candidates are selected.

## Safe Cleanup Candidates After Eval

These open draft run-log PRs appear to contain no accepted candidate commits.
They have only `coord: run-log start` and/or inherited `q-branch-observer`
commits.

| PR | Branch |
| --- | --- |
| #49947 | `claude/observer-blank-20260427T1732` |
| #49952 | `claude/observer-blank-20260427T1813` |
| #49963 | `claude/observer-blank-20260427T2149` |
| #50011 | `claude/observer-full-20260428T1455` |
| #50012 | `claude/observer-full-20260428T1457` |
| #50013 | `claude/observer-full-20260428T1459` |
| #50039 | `claude/observer-full-20260428T2025` |
| #50040 | `claude/observer-full-20260428T2034` |
| #50041 | `claude/observer-full-20260428T2037` |
| #50042 | `claude/observer-full-20260428T2038` |
| #50045 | `claude/observer-full-20260428T2058` |
| #50046 | `claude/observer-full-20260428T2059` |
| #50096 | `claude/observer-full-20260429T1443` |
| #50097 | `claude/observer-full-20260429T1445` |
| #50098 | `claude/observer-full-20260429T1447` |
| #50101 | `claude/observer-full-20260429T1514` |
| #50102 | `claude/observer-full-20260429T1516` |
| #50104 | `claude/observer-full-20260429T1519` |
| #50105 | `claude/observer-full-20260429T1520` |
| #50106 | `claude/observer-full-20260429T1521` |
| #50107 | `claude/observer-full-20260429T1523` |
| #50128 | `claude/observer-full-20260429T1817` |
| #50245 | `claude/observer-full-20260501T2005` |
| #50246 | `claude/observer-full-20260501T2006` |

## Keep Or Defer Cleanup

These should stay open or at least not be deleted until traceability is no
longer needed.

| PR | Branch | Reason |
| --- | --- | --- |
| #49670 | `ella/claude-coordinator-harness` | Harness PR; keep. |
| #50126 | `claude/observer-full-20260429T1815` | Accepted anomaly-rank change represented on manual branch; keep until final traceability is no longer needed. |
| #50127 | `claude/observer-full-20260429T1816` | Accepted early BOCPD warmup/zfallback history represented/reverted on manual branch; keep until final traceability is no longer needed. |
| #50138 | `headless/observer-20260429-1931` | Headless run history/log artifact. Low code value, but useful for audit until final eval is done. |
| #50229 | `claude/observer-full-20260501T1403` | Main recent seedless run; produced many candidates copied into the manual branch. |
| #50248 | `claude/observer-full-20260501T2022` | Correlator/H2A workspace produced useful commits copied into the manual branch, even though the PR branch mostly shows run-log state. |
| #50249 | `claude/observer-full-20260501T2023` | Filter/H2B workspace produced useful commits copied into the manual branch. |

## Cleanup Command Shape

After eval is complete, close stale PRs with branch deletion one at a time or in
small batches:

```bash
gh pr close <PR_NUMBER> --repo DataDog/datadog-agent --delete-branch
```
