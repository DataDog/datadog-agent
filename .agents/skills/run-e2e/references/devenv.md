# Running E2E tests inside a dev env

What the container does not inherit, why `up` refuses when it does, and how to get files out. For why
each bootstrap step is shaped the way it is, read the docstrings in
`/.agents/skills/run-e2e/scripts/devenv_e2e.py` — they sit next to the code they explain.

## What the container does not inherit

It does inherit the host's `~/.aws` (profile definitions), `AWS_PROFILE`/`AWS_REGION`/`AWS_DEFAULT_REGION`
— so a stray `AWS_PROFILE` follows you in — and `pulumi` at `/usr/local/bin/pulumi` from the base image.

- **AWS authorization.** The framework authenticates through a profile whose `credential_process` runs
  aws-vault, which keeps tokens in a keyring local to the container. There is no way to pre-authorize it
  from the host, so a new container needs an SSO flow completed inside it; the env opens the browser tab
  on your desktop. A healthy host session does not help.
- **The E2E config and keypair.** `~/.test_infra_config.yaml` does not exist there, and the config's key
  paths are host-absolute. The bootstrap supplies both.
- **Pulumi state.** `$HOME` is not a persistent volume, so `~/.pulumi` — backend, plugins and local stack
  state — is recreated per container and lost with it. Hence: never remove an env with live stacks.

## Constraints on the checkout

- It must be a directory named `datadog-agent`. `dda env dev start` mounts `<cwd>/../datadog-agent`.
- It must not be a git worktree. `.git` there points outside every mount, and `new-e2e-tests.run` reads
  the commit SHA unconditionally, so the run aborts before its own preflight.
- `env.dev.clone-repos` must be off, or the container gets a shallow clone of the default branch instead
  of your tree. No per-command override exists: `dda config set env.dev.clone-repos false`, then recreate
  the env. `up` verifies the result rather than trusting it, by comparing host and container
  `git rev-parse HEAD`.

`/.agents/skills/follow-pr/create_devenv.sh` sidesteps the naming rule with
`--repo "$(git rev-parse --show-toplevel)"`. That is POSIX-only — a Windows drive colon makes the
resulting `-v` spec unparseable — so do not copy it here.

## Env states

`start` accepts only `nonexistent` and `stopped` and refuses new mount options on a `stopped` env, so
changing the mounts means removing the env first. `remove` accepts only `error` and `stopped`, `stop`
only `started`. Full semantics in `/docs/public/tutorials/dev/env.md`.

| State | `up` | `down` |
|---|---|---|
| `started` | Continues | Checks for live stacks, then removes |
| `stopped` | Resumes, never removes | Refuses; stacks cannot be checked while it is down |
| `error` | Refuses | Refuses; the state covers both "never started" and "ran tests, then died" |
| `nonexistent` | Creates with the mounts | Nothing to do |

Envs start with `--no-pull`, so a container uses whatever image is already local rather than
re-downloading 12 GB. To move to a current image, remove the env and `docker pull
datadog/agent-dev-env-linux`.

## Provider coverage

The bootstrap brings in the key files listed in `KEY_FIELDS` — `configParams.aws` and
`configParams.local` — rewriting each path to its container mount, and probes the AWS profile. Azure and
GCP are not in that list, so those targets arrive with host-absolute key paths and unverified
credentials: run them with `--host`. Adding a provider means one entry in `KEY_FIELDS`.

Targets on the framework's local provisioners need no cloud credentials, but they do need
`configParams.local.publicKeyPath`, which the podman provisioner reads unconditionally
(`/test/e2e-framework/resources/local/podman/vm.go`) — hence its presence in that list. Pass
`--no-aws-check` for them so a cost-free run is not gated behind an SSO acceptance; the bootstrap cannot
infer that for itself, because it never sees the target.

## Getting files out

Test output goes to `$HOME/e2e-output/<suite>/<timestamp>` in the *container's* home. `dda env dev
fs export` is unusable under the nu shell, so:

```bash
docker cp dda-linux-container-e2e-run:/home/dd/e2e-output ./e2e-output
dda env dev shell -t linux-container --id e2e-run   # for anything interactive
```
