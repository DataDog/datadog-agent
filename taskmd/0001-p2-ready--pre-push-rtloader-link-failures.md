---
created: 2026-03-05
priority: p2
status: ready
---

# Pre-push hook: rtloader link failures

## Summary

The `go-test` pre-push hook fails because three packages can't link against
`libdatadog-agent-rtloader`:

- `cmd/agent/subcommands/run`
- `cmd/dogstatsd/subcommands/start`
- `comp/metadata/inventorychecks/inventorychecksimpl`

The hook runs `dda inv test --only-modified-packages`, which picks up these
packages transitively. They require `dda inv rtloader.build` to have been run
first, but that isn't part of the normal dev loop.

## Acceptance Criteria

- [ ] Pre-push `go-test` hook passes on a fresh worktree without manual rtloader build
- [ ] Either exclude rtloader-dependent packages from the hook, build rtloader as a prerequisite, or use `--build-exclude` tags
