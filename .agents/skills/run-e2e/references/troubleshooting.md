# Troubleshooting E2E runs

Read this on the first failure that is not a test assertion — in particular anything that fails
before the first `--- PASS` or `--- FAIL` line, which means the run never reached a test.

The **Run it on** column is the important one. Some remedies only work on the host, because that is
where the canonical E2E config and keypair live. AWS authorization is the opposite: the host and the
container each hold their own, so authorizing one does nothing for the other.

## Symptom to remedy

| Symptom | Cause | Remedy | Run it on |
|---|---|---|---|
| `Local E2E config is missing or incomplete. Run \`dda inv -- e2e.setup\` once to configure` | No `~/.test_infra_config.yaml`, or it has no `configParams.aws.keyPairName` | `dda inv -- e2e.setup` — see `references/setup.md`, then re-run `devenv_e2e.py up` so the container picks up the new config | **Host.** Never the interactive form in a dev env |
| `pulumi: command not found` in a dev env | A broken image; Pulumi ships at `/usr/local/bin/pulumi` | Recreate the env; do not install Pulumi by hand | Env |
| `pulumi whoami` fails, or Pulumi cannot find a plugin, in a fresh env | Pulumi's backend selection and plugins live in `~/.pulumi`, which is not a persistent volume | `PULUMI_SKIP_UPDATE_CHECK=true dda inv -- e2e.setup --no-interactive` — this is what `devenv_e2e.py up` already does | Env |
| `No valid credentials sources found`, `ExpiredToken`, or an SSO prompt that never resolves, **in the dev env** | The container's aws-vault keyring holds no usable token, and a healthy host session does not help | `dda env dev run -t linux-container --id e2e-run -- aws-vault login sso-agent-sandbox-account-admin-8h`, which opens the flow on your desktop through the env's browser proxy, then rerun `up` | Env |
| The same symptoms **on the host**, with `--host` or during `dda inv -- e2e.setup` | The host's own SSO session has expired | `aws-vault login sso-agent-sandbox-account-admin-8h` | **Host** |
| `User: arn:aws:sts::... is not authorized to perform: ecr:BatchGetImage` | A stray `AWS_PROFILE` overrides the framework's profile selection, and `dda env dev start` forwards it in | `unset AWS_PROFILE`, then recreate the env so it does not inherit it | **Host** |
| `error: the stack is currently locked by 1 lock(s)` | A lock left behind by an interrupted run | `dda inv -- new-e2e-tests.clean`. If that says `Cleanup supported for local state only`, run `pulumi login --local` first | Wherever the interrupted run happened |
| Pulumi plans to replace or delete resources that should not exist yet | Local Pulumi state has diverged from what is actually in the cloud | Stop the run as soon as you see it. Retry with `--stack-name-suffix <short>` for a clean stack, and reconcile the old one with `dda inv -- new-e2e-tests.clean -s` | Same place |
| `fatal: not a git repository`, or the run aborts before its preflight | The container's checkout is a worktree, whose `.git` points outside every mount | Run from the main clone, or use `--host`. See `references/devenv.md` | — |
| `fatal: detected dubious ownership` | The bind-mounted checkout has a different uid than the container user | `git config --global --add safe.directory /repos/datadog-agent` | Env |
| `Local repository not found: datadog-agent` from `dda env dev start` | Started from a directory whose parent has no `datadog-agent` directory | Start from a clone whose directory is named `datadog-agent`; `references/devenv.md` explains the mount rule | **Host** |
| The env's `/repos/datadog-agent` is at a different revision than your tree | Usually created while `env.dev.clone-repos` was on, so it holds a clone of the default branch | `dda config set env.dev.clone-repos false`, then recreate the env. `devenv_e2e.py up` refuses to continue and prints the commands | **Host** |
| A container path in a command arrives mangled, e.g. `C:/Users/.../git/.e2e/key.pem` instead of `/.e2e/key.pem` | MSYS path conversion, on Windows, through Git Bash | Prefix the command with `MSYS_NO_PATHCONV=1`. `devenv_e2e.py` avoids the issue by keeping container paths off the test command line | **Host**, Git Bash only |
| Diagnostics are missing after a failure that ran in a dev env | Output lands in the *container's* home | `docker cp dda-linux-container-e2e-run:/home/dd/e2e-output ./e2e-output`. `dda env dev fs export` shares the quoting bug that breaks `fs import` under the nu shell | **Host** |
| The config or keypair looks wrong in a way not listed above — key missing in the region, ssh-agent not running, bad key format | Various | `dda inv -- e2e.setup.debug` diagnoses these and prints what it finds; `e2e.setup.debug-keys` covers the keypair alone | **Host** |
| Cloud resources outlive the env they were created from | The env was removed while a stack was live, taking `~/.pulumi` with it | Find them in whichever account the target provisions into, by the stack-name suffix the bootstrap injected or by team tag — not by the `username` tag, which reads `dd` for every dev-env run. Prevent it by never removing an env with live stacks | **Host**, cloud console or CLI |

## Cleanup commands

```bash
pulumi stack ls --all --project e2elocal --json   # what a container still knows about
dda inv -- new-e2e-tests.clean                   # remove local Pulumi locks
dda inv -- new-e2e-tests.clean -s                # also destroy and remove local stacks
dda inv -- new-e2e-tests.clean --output          # clear local test output
```

Inside a dev env, prefix each with `dda env dev run -t linux-container --id e2e-run --`.

## Related

`/.agents/skills/run-windows-e2e/references/troubleshooting.md` — the same Pulumi-lock and `AWS_PROFILE`
ground from the Windows angle, plus the crash dumps and event logs those suites collect.
