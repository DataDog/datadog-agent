# rshell permission-model E2E tests

The `rshell_matrix_*_test.go` files exercise the rshell allow-list truth table
from the [Confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/6608848267/Rshell+permission+model+allow+lists).

## Truth table

rshell has two independent allow-lists — `commands` and `paths` — each
computed as the **intersection** of a backend-provided per-task list and an
operator-provided datadog.yaml list. The expected effective list per cell:

| Backend ↓ \ Operator → | unset   | `[]` | non-empty, disjoint | non-empty, overlap |
|------------------------|---------|------|---------------------|--------------------|
| nil / field absent     | ∅       | ∅    | ∅                   | ∅                  |
| `[]`                   | ∅       | ∅    | ∅                   | ∅                  |
| non-empty              | backend | ∅    | ∅                   | intersection       |

- Backend is **authoritative**: when it's nil or `[]` the effective list is
  always empty, regardless of what the operator says.
- Operator can only **tighten**: an operator `[]` is the on-host kill-switch.
- The commands matrix and paths matrix are independent — each has its own
  12 cells.

## Test layout

Each test covers one cell on one axis. The non-tested axis is held in a
permissive state so the outcome is attributable to the axis under test.

| Suite file                                 | `operator.commands` | `operator.paths`       | Covers                               |
|--------------------------------------------|---------------------|------------------------|--------------------------------------|
| `rshell_matrix_both_unset_test.go`         | unset               | unset                  | commands col 1 + paths col 1         |
| `rshell_matrix_commands_killswitch_test.go`| `[]`                | unset (permissive)     | commands col 2                       |
| `rshell_matrix_commands_narrow_test.go`    | `["rshell:cat"]`    | unset (permissive)     | commands cols 3 & 4                  |
| `rshell_matrix_paths_killswitch_test.go`   | unset (permissive)  | `[]`                   | paths col 2                          |
| `rshell_matrix_paths_narrow_test.go`       | unset (permissive)  | `["/host/var/log"]`    | paths cols 3 & 4                     |

`rshell_matrix_base_test.go` holds the shared `matrixSuite` base, PAR-readiness
probe, and `enqueueAndWait` / `assertBlocked` / `assertAllowed` helpers.

## Naming schema

```
Test<Axis>_Operator<State>_Backend<State>_<Outcome>
```

- `<Axis>` is `Commands` or `Paths` — needed because the `BothUnset` suite
  covers both axes from one provisioner.
- `<Operator state>` ∈ `Unset`, `Empty`, `NonEmptyDisjoint`, `NonEmptyOverlap`
  (the four truth-table columns).
- `<Backend state>` ∈ `Absent`, `Empty`, `NonEmpty` (the three rows).
- `<Outcome>` ∈ `Allows`, `Blocks`.

24 tests total (12 per axis, one per cell).
