# test/new-e2e

## Running tests

Always run from the **repo root** (not from inside `test/new-e2e`):

```bash
dda inv new-e2e-tests.run --targets=./tests/<area>/... --run <TestName>
# Examples:
dda inv new-e2e-tests.run --targets=./tests/containers/... --run TestKindSuite
dda inv new-e2e-tests.run --targets=./examples/... --run TestMyKindSuite
```

The `--targets` path is relative to `test/new-e2e/`, resolved by the invoke task.
Do **not** `cd` into `test/new-e2e` first — invoke handles the path resolution.
