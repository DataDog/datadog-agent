# Stale APIs and where they are written

Load when working from an internal Confluence E2E page, an older branch, or any snippet whose form you cannot find in use under `test/new-e2e/tests/` — several otherwise-trustworthy sources teach forms that do not compile.

The third column is the point of this file. Knowing that a form is wrong helps only if you can recognise what taught it to you, and these sources are otherwise reliable, so a snippet from them reads as trustworthy.

## Will not compile

| Wrong | Right | Why it is reached for |
|---|---|---|
| `awshost.WithAgentOptions(agentparams...)` | `awshost.WithRunOptions(ec2.WithAgentOptions(agentparams...))` | It is the correct form on Azure and GCP, so it transfers wrongly to AWS |
| `ec2docker.WithExtraComposeManifest(...)` | it is a `dockeragentparams` option: `scenariodocker.WithAgentOptions(dockeragentparams.WithExtraComposeManifest(...))` | The option configures compose, so the scenario package looks like its home |
| `winazurehost` as the default Windows provisioner | `winawshost`; Azure's Windows provisioner has no test or CI precedent | Confluence recommends Azure for faster Windows boot |

## Compiles, behaves wrong

| Wrong | Right | Why it is easy to get wrong |
|---|---|---|
| `dockeragentparams.WithEnvironmentVariables` to configure the agent | `dockeragentparams.WithAgentServiceEnvVariable(key, value)` | The name reads like the general case, but it only reaches the docker-compose command and its interpolation, not the agent process |
| `os.Getenv("E2E_FAKEINTAKE_IMAGE_OVERRIDE")` and other `E2E_*` reads | the runner parameter store, e.g. `runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.SkipWindows, false)` | Direct reads bypass the profile and resolve differently in CI than locally |

## Confirming a form before you use it

AWS provisioners nest their options inside `WithRunOptions` while Azure, GCP, and local ones are flat, so a snippet moved between clouds needs its shape adjusted. Both shapes are current and neither is deprecated; `references/environments.md` § "Two option shapes" has the rule and the per-provisioner naming that goes with it.

```bash
grep -n '^func With' test/e2e-framework/testing/provisioners/<path>/*.go
grep -rn '<OptionName>' test/new-e2e/tests/ test/new-e2e/examples/ | head
```

A name with no usage anywhere in `tests/` or `examples/` is worth double-checking; the widely used forms are widely used because they work.

## Confluence pages

The internal E2E pages carry useful material — the cloud-selection rule, the credentials runbook, per-suite runtime measurements — alongside instructions that have since moved:

| Page says | Current |
|---|---|
| `inv …` | `dda inv …` |
| `.gitlab/e2e/e2e.yml` | `.gitlab/test/e2e/e2e.yml` |
| `test/new-e2e/pkg/utils/e2e` | `test/e2e-framework/testing/e2e` |
| framework lives in the `test-infra-definitions` repository | it lives in this repository under `test/e2e-framework/` |
| `compare_to: main` in a rule | `compare_to: $COMPARE_TO_BRANCH` |
| wrap commands in `aws-vault exec` | the runner wraps them; pass `--no-aws-vault` only if you manage credentials yourself |
| `deploy_deb_testing-a7_x64` as an artifact job | `agent_deb-x64-a7` |

## Fixing what you find

When a repository document turns out to be wrong, correct it in the same change. The root `AGENTS.md` asks for this, and it is the only thing that keeps this list from growing: a trap fixed at the source stops reaching the next agent, while a trap recorded here only helps whoever loads this file.
