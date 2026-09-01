---
name: run-e2e
description: >-
  Run one already-written new-e2e test locally and triage the setup failures that stop it — "run the
  containers e2e tests", "my e2e run fails before any test starts".
allowed-tools: Bash, Read, AskUserQuestion
argument-hint: "[target, e.g. ./tests/agent-subcommands/flare] [TestName] [--host] [--keep-stack] [--stack-name-suffix <s>]"
---

Run a single new-e2e target with `dda inv -- new-e2e-tests.run`. Most targets provision real
infrastructure in a cloud account — usually AWS, sometimes GCP or Azure — though the framework also has
local provisioners that cost nothing but time. Run duration is a property of the target: minutes for one
VM, considerably longer for a Kubernetes cluster. Either way, aim for one correct run rather than a fast
iteration loop.

| Reference | Load when |
|---|---|
| `references/devenv.md` | `devenv_e2e.py` exits non-zero, or the container behaves differently from the host |
| `references/setup.md` | Before offering to run `dda inv -- e2e.setup` |
| `references/troubleshooting.md` | Any failure before the first `--- PASS`/`--- FAIL` line |
| `references/flags.md` | The request needs more than a target and a test name |

## Step 1 — Get the target, or ask

A target is a package path relative to `/test/new-e2e/`, like `./tests/agent-subcommands/flare`; the
task resolves against that module, so repeating the prefix is wrong.

Requests usually name a test, not a package — "run the flare e2e test" is the normal shape. Use a target
if given, otherwise ask, offering candidates you already know. Never search the tree: a guessed target
provisions the wrong thing and you find out after paying for it.

Anchor a supplied test name (`--run '^TestFlareSuite$'`), or `TestFlare` also selects `TestFlareOpts`.

## Step 2 — Decide where it runs

```bash
test -f /.started && echo IN_DEVENV || echo ON_HOST
```

The dev env entrypoint creates `/.started`. `ON_HOST` → step 3A, `IN_DEVENV` → step 3B, `--host` → 3C.

## Step 3A — On the host (the usual case)

```bash
python .agents/skills/run-e2e/scripts/devenv_e2e.py up --json
```

Starts a dev env at id `e2e-run` if needed, gives it the host's E2E config and keypair, establishes
Pulumi's backend, checks AWS access, and prints the `run_prefix` for step 5. Idempotent, so a reused env
pays the setup cost once. Add `--no-aws-check` for a locally-provisioned target to skip the SSO
acceptance; the host still needs AWS config, because the run task requires it whatever the target is.

The AWS check needs the user present — authorizing a new container means completing an SSO flow whose
browser tab opens on their desktop. Warn them, and if it gives up, relay the `aws-vault login` it prints
and rerun `up`. That is normal on a new env.

Every failure prints an actionable message; relay it. The table picks the reference file and says which
machine the remedy belongs on.

| Exit | Meaning | What to do |
|---|---|---|
| 0 | Ready | Step 4, using the printed `run_prefix` |
| 2 | Host has no usable `~/.test_infra_config.yaml` | Read `references/setup.md`, offer `dda inv -- e2e.setup` **on the host**, retry |
| 3 | The container would not hold this working tree | Follow the printed remedy; `references/devenv.md` per case. Offer `--host` if the checkout cannot be used |
| 4 | The container cannot authenticate to AWS | Run the printed `aws-vault login` **inside the env**, then retry |
| 5 | Already inside a dev env | Step 2 misread the marker; go to 3B |
| 6 | Env is in `error`, so its stacks cannot be checked | Do not remove it for them; relay the message, which says when recreating is safe |
| other | No dedicated remedy | Relay the message; `references/troubleshooting.md` |

Azure and GCP targets are not handled — only AWS credentials reach the container. Use `--host`.

## Step 3B — Already inside a dev env

```bash
test -f ~/.test_infra_config.yaml && echo CONFIG_OK || echo CONFIG_MISSING
pulumi whoami >/dev/null 2>&1 && echo BACKEND_OK || echo BACKEND_MISSING
```

`BACKEND_MISSING` → `PULUMI_SKIP_UPDATE_CHECK=true dda inv -- e2e.setup --no-interactive`. Do not install
Pulumi; the image ships it, only its plugins and backend are missing. `CONFIG_MISSING` → stop; this env
has no E2E identity, so have them recreate it with `devenv_e2e.py up` from the host. Never the
interactive `dda inv -- e2e.setup` here — see `references/setup.md`.

This container needs its own AWS authorization, which nothing on the host provides. Have them run
`aws-vault login sso-agent-sandbox-account-admin-8h` first rather than discovering it ten minutes in.

Then step 4 with a suffix identifying them, because stack names take the container user name `dd` and
would otherwise collide. `git config user.email` is set from the host.

```bash
E2E_STACK_NAME_SUFFIX=<you> dda inv -- new-e2e-tests.run --targets=<target> [--run <regex>] [flags]
```

## Step 3C — `--host` escape hatch

Check `~/.test_infra_config.yaml`, `pulumi whoami`, and a live AWS session — here the host's own counts —
then run `dda inv -- new-e2e-tests.run` directly. Faster on a configured Linux or macOS machine, and
Pulumi state survives there. Not the default because an unconfigured or Windows host fails in ways the
dev env does not.

## Step 4 — Confirm before provisioning

Get an explicit yes for: the exact command, target and `--run`, where it runs, what it provisions and in
which account, roughly how long, and whether the stack is destroyed afterwards. If you cannot tell what
it provisions, say so — that is worth confirming before paying for it.

## Step 5 — Run it

Use the `run_prefix` from step 3A verbatim and append the test command. Do not add container paths of
your own: on Windows, Git Bash rewrites them before they reach the container, and everything the run
needs is already in the config the bootstrap installed.

```bash
dda env dev run -t linux-container --id e2e-run -- env <env_args...> \
  dda inv -- new-e2e-tests.run --targets=<target> [--run <regex>] [flags]
```

Start it with `run_in_background: true`; these outlast a foreground Bash call. The first run in a fresh
env is much the slowest — the test binary compiles from a cold cache before any infrastructure is
touched, so several minutes of silence is normal.

## Step 6 — Report

```
### E2E run — <target> [--run <regex>]
- Where:       dev env `e2e-run` | host
- Command:     <exact command as executed>
- Result:      PASS | FAIL | SETUP FAILURE (failed before any test ran)
- Duration:    <mm:ss>
- Stack:       <name> — destroyed | kept (--keep-stack)
- Failures:
  - <TestSuite/TestName> — <one-line reason>
- Diagnostics: <path>   (+ the `docker cp` to retrieve it, if it ran in a dev env)
- Next step:   <the single most useful action>
```

For a `SETUP FAILURE`, take the symptom to `references/troubleshooting.md` rather than reporting raw
stderr.

## Step 7 — Tear down

The env is reusable and costs nothing idle, so leave it unless asked. When asked:

```bash
python .agents/skills/run-e2e/scripts/devenv_e2e.py down
```

It exits 6 rather than removing an env whose Pulumi stacks are live or uncheckable — that state exists
nowhere else, so an orphaned cluster is the cost of getting this wrong. It prints the destroy command.
`--force` overrides it; only reach for that once you have confirmed nothing is running. `--keep-stack`
implies keeping the env too.

## Examples

> "run TestVMSuite in ./examples" — the default path, from a host that may not be configured

```bash
python .agents/skills/run-e2e/scripts/devenv_e2e.py up --json
dda env dev run -t linux-container --id e2e-run -- env E2E_STACK_NAME_SUFFIX=alice \
  dda inv -- new-e2e-tests.run --targets=./examples --run='^TestVMSuite$'
```

> "just run it here, my machine is already set up" — `--host`, skipping the container

```bash
dda inv -- new-e2e-tests.run --targets=./examples --run='^TestVMSuite$'
```
