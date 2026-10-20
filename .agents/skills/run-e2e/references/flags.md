# Flags and runner variables

Read this when the request needs more than a target and a test name — a locally built package or
image, dev mode, config-map overrides, retries — or when a single value from
`~/.test_infra_config.yaml` needs overriding for one run.

## Test command

`dda inv -- new-e2e-tests.run --help` describes every flag and is the reference. This file only groups
them by purpose and records the judgement calls the help text cannot make for you.

`--tags`, `--targets`, `--configparams`, `--run` and `--skip` are repeatable; the rest take one value.

- Choosing what runs: `--targets`, `--run`, `--skip`, `--tags`, `--osdescriptors`, `--no-recursive`
- Choosing what gets tested: `--agent-image`, `--cluster-agent-image`, `--local-package`,
  `--pipeline-id`, `--flavor`, `--configparams`
- Infrastructure lifecycle: `--keep-stack`, `--stack-name-suffix`, `--max-retries`, `--timeout`
- Output: `--verbose`, `--cache`, `--logs-folder`, `--result-json`, `--junit-tar`, `--extra-flags`

### Judgement calls

- `--targets` resolves against `/test/new-e2e/`, so repeating that prefix inside the target is wrong.
  It repeats as a flag, but prefer one target per run: each provisions its own stack.
- Anchor `--run` at both ends. `TestFlare` also selects `TestFlareOpts`.
- On the dev-env path, give `--stack-name-suffix` to `devenv_e2e.py up` rather than to the test. It sets
  the same variable the bootstrap uses to keep your stacks distinct from other developers', so passing
  it here replaces that instead of adding to it.
- `--keep-stack` in a dev env means keeping the env too, because the stack's state lives inside it.
- `dda build docker` is the supported way to produce and push an image for `--agent-image`; it prints
  the matching command when it finishes.
- Never `--profile ci` on a developer machine. It skips the local-config preflight that exists to fail
  early with a clear message, so the run fails later and less legibly instead.

## Overriding config values for one run

The runner resolves each parameter through
`parameters.NewCascadingStore(envValueStore, configFileValueStore)`
(`/test/e2e-framework/testing/runner/local_profile.go`), so an environment variable wins over
`~/.test_infra_config.yaml`. This is what lets the dev-env path keep the host's config file while
redirecting the key paths at the container's copies. The full mapping is
`/test/e2e-framework/testing/runner/parameters/store_env.go`; the ones that come up:

| Variable | Overrides |
|---|---|
| `E2E_KEY_PAIR_NAME` | `configParams.aws.keyPairName` |
| `E2E_AWS_PRIVATE_KEY_PATH` | `configParams.aws.privateKeyPath` |
| `E2E_AWS_PUBLIC_KEY_PATH` | `configParams.aws.publicKeyPath` |
| `E2E_AWS_PRIVATE_KEY_PASSWORD` | `configParams.aws.privateKeyPassword` |
| `E2E_PULUMI_PASSWORD` | `configParams.pulumi.passphrase` |
| `E2E_API_KEY`, `E2E_APP_KEY` | `configParams.agent.apiKey` / `.appKey` |
| `E2E_STACK_NAME_SUFFIX` | Same as `--stack-name-suffix` |
| `E2E_DEV_MODE` | Same as `--keep-stack` |
| `E2E_EXTRA_RESOURCES_TAGS` | Extra tags on provisioned resources |
| `E2E_OUTPUT_DIR` | Where test output and diagnostics are written |
| `E2E_FAKEINTAKE_IMAGE_OVERRIDE` | The fakeintake image, instead of the pinned tag |

Note `DD_API_KEY` is not part of this — the E2E path uses `E2E_API_KEY` and the `configParams.agent`
values, which are length-checked (32 and 40 characters).

Pass these through `dda env dev run` with an `env VAR=value ...` prefix, for the reason given in the
skill's run step.

## Passing raw flags to `go test`

`--extra-flags` is appended verbatim after `-args`, which covers suite-specific flags the task does not
model. Reach for that rather than calling `go test` yourself: the invoke task is what computes the build
tags, runs the local-config preflight and exports `PULUMI_CONFIG_PASSPHRASE`, and a run that skips those
can fail for reasons that have nothing to do with the code under test.
