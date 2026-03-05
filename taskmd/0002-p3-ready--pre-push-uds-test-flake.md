---
created: 2026-03-05
priority: p3
status: ready
---

# Pre-push hook: UDS test flake

## Summary

`pkg/trace/api.TestUDS` and `TestUDS/uds_permission_err` fail intermittently
during the `go-test` pre-push hook. These are Unix domain socket tests that
appear to be timing-sensitive.

Observed on clean `q-branch-observer` checkout with no local modifications.

## Acceptance Criteria

- [ ] Identify root cause of flakiness (permission race, socket cleanup, timeout)
- [ ] Fix or mark as known flake so it doesn't block pre-push hooks
