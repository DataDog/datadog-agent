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

## CI two-job pattern (provision/install split)

```bash
# Job 1: provision infra, dump descriptor
dda inv new-e2e-tests.run --targets=./tests/... --dump-env-descriptor=env.json

# Job 2: install agent + run tests (no Pulumi)
dda inv new-e2e-tests.run --targets=./tests/... --env-descriptor=env.json
```

Or set the env var directly: `E2E_ENV_DESCRIPTOR=/path/to/env.json`. When set,
`SetupSuite` loads the environment from the descriptor instead of running Pulumi,
runs PostProvision (agent install via SSH/Helm), and then runs tests. Teardown is
skipped — Job 1 is responsible for destroying the infrastructure.

## Writing tests

See `test/e2e-framework/AGENTS.md` for the full framework guide including:
- Provisioner options (WithAgentOptions, WithHelmValues, etc.)
- Agent reconfiguration without Pulumi (Agent.Configure)
- Attach mode (pre-provisioned env descriptor)
- Custom environments

## Test tiers

Tests fall into four tiers based on how they interact with the installer layer:

| Tier | Pattern | Migration |
|------|---------|-----------|
| **T0** — excluded | Uses `ProvisionerNo*Agent*` or installs agent themselves (installer/, agent-platform/) | None — keep as-is |
| **T1** — transparent | Standard provisioner + `WithAgentOptions` | Already decoupled: provisioner handles install via PostProvision |
| **T2** — mechanical | Uses `UpdateEnv` to change agent config | Migrate `UpdateEnv(provisioner-with-agent-opts)` → `s.Env().Agent.Configure(t, opts...)` |
| **T3** — bespoke | Custom Pulumi RunFunc with agent install | Migrate to `WithPostProvision` or explicit `hostagent.Install` in `SetupSuite` |
| **T4** — blocked | ECS/Fargate, Windows, macOS (stubs) | Sequence: wait for their installer to land |
