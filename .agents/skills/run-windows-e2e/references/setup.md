# Setup — Windows E2E Tests

One-time environment setup for running Windows E2E tests locally. Applies to
both MSI install tests (`test/new-e2e/tests/windows/`) and Fleet Automation /
installer tests (`test/new-e2e/tests/installer/windows/`).

## Prerequisites

Follow the general E2E setup guide first:
[docs/public/how-to/test/e2e.md](../../../../docs/public/how-to/test/e2e.md).
It covers AWS `agent-sandbox` account access, `aws-vault` setup, the
`PULUMI_CONFIG_PASSPHRASE` requirement, and required tooling (Go, Python,
`dda`).

The Windows tests provision real AWS EC2 Windows instances and require the same
AWS access as all other E2E tests. Without valid AWS credentials and
`PULUMI_CONFIG_PASSPHRASE` set, the run fails before any test code executes.

`dda inv e2e.setup` (defined in `tasks/e2e_framework/setup/`) walks through this
interactively: it validates CLI tools, installs Pulumi if missing, prompts for
AWS keypair paths, Datadog API key, and Pulumi passphrase, then writes
`~/.test_infra_config.yaml`.

## Persistent configuration

Most run parameters are easier to set once in `~/.test_infra_config.yaml` than
as environment variables each session. Relevant fields:

- AWS keypair paths (`privateKeyPath` is also the SSH key — see
  [vm-access.md](vm-access.md))
- Datadog API key — **required**; the agent MSI install needs a valid key
- Pulumi passphrase — alternative to the `PULUMI_CONFIG_PASSPHRASE` env var
- `devMode` — see below

## Dev mode

By default each run provisions a fresh VM and destroys it afterwards, which
makes local test-fix-rerun loops slow. Set `devMode: true` in
`~/.test_infra_config.yaml` to reuse the same VM across runs. The VM is
identified by the Pulumi stack name, derived from the test package and suite
name.

Per-run alternative:

```bash
E2E_DEV_MODE=true go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/install-test -run TestInstall$
```

Or in VSCode:

```json
"go.testEnvVars": { "E2E_DEV_MODE": "true" }
```

When reusing a VM, clean up the agent between runs so state does not leak — see
[running.md](running.md) ("Clean state between runs").
