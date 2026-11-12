---
name: run-macos-e2e
description: Build a macOS Agent .dmg locally and run the macOS agent-platform new-e2e suites (TestMacosInstallScript, which includes the LaunchDaemons tests, TestMacosInstallScriptUpgradeFromStable, and the Fleet configuration-experiment suite TestFleetConfigMacOS) against it, on a local/BYO macOS host over SSH (LOCAL_VM) or on your own AWS mac pool instance, with a full clean (reverted) run or a fast reuse run. Use when the user says "run the macOS e2e locally", "test my branch on a mac VM", "build an arm64/x86_64 dmg", "run TestMacosInstallScript against my Parallels VM", "test the macOS upgrade from stable locally", "run the macOS Fleet config experiment tests locally", "revert my mac pool instance", or a macOS e2e run fails before any test starts (Colima/Docker, SSH, Ruby, dmg path). Prefer this over run-e2e (Linux, dda env dev) and run-windows-e2e for anything macOS.
allowed-tools: Bash, Read, Glob, Grep, AskUserQuestion, Monitor
argument-hint: "[install|upgrade|config] [--target local-vm|aws-pool] [--arch arm64|x86_64] [--clean|--fast] [--dmg <abs-path>]"
---

Build an unsigned macOS Agent `.dmg` from the current checkout and run a macOS
E2E suite against it, without waiting for a CI pipeline.

**Owning team:** `@DataDog/windows-products`

Load the references only when a step calls for them:

- [`references/setup.md`](references/setup.md) — one-time prerequisites: dmg build toolchain (Ruby, bundler, gems), LOCAL_VM host setup, AWS pool access
- [`references/running.md`](references/running.md) — full commands, env vars, timings, and the AWS pool clean-vs-fast run modes
- [`references/troubleshooting.md`](references/troubleshooting.md) — symptom → cause → fix for every failure seen so far; read it before changing any code

## Instructions

### Step 1 — Pick the suite, target, and architecture

| Suite | Test | `--targets` |
|---|---|---|
| install | `TestMacosInstallScript` (15 tests, LaunchDaemons ones included, ~4.5 min) | `./tests/agent-platform/tests` |
| upgrade | `TestMacosInstallScriptUpgradeFromStable` (1 test, ~1.5 min) | `./tests/agent-platform/tests` |
| config | `TestFleetConfigMacOS` (32 tests, the longest suite) | `./tests/fleet` |

The install suite's tests live in `macos_install_test.go` and
`macos_launchdaemons_test.go`. The LaunchDaemons tests re-run the postinstall
script extracted from your dmg (`TestZYPostinstFailsWhenTheInstallerFails`)
and reinstall your dmg over itself (`TestZYReinstallSameVersion`). The upgrade
suite installs the latest released Agent with the public install script, so the
host needs internet access, and then upgrades it to your dmg. It is skipped
unless `E2E_MACOS_LOCAL_DMG` or `E2E_PIPELINE_ID` is set.

The config suite is in `test/new-e2e/tests/fleet/config_macos_test.go`. It
installs the Agent itself (your dmg when `E2E_MACOS_LOCAL_DMG` is set, otherwise
the pipeline build), points the Agent and the installer daemon at its own
fakeintake for Remote Config, and drives Fleet configuration experiments (start,
promote, stop, crash and reboot recovery) through it. On `local-vm` it starts
that fakeintake as a container on the host, so Docker has to answer there too.

| Target | When | dmg arch |
|---|---|---|
| `local-vm` | an SSH-reachable macOS box you own (Parallels/UTM VM, spare Mac). Free and fast to iterate on | the host's arch, usually `arm64` |
| `aws-pool` | you need the CI-like `mac1.metal` host. Your first run provisions your own Dedicated Host, which bills a **24-hour minimum**, so confirm with the user before the first run | `x86_64` for `mac1.metal` |

Ask the user for anything they didn't give you. For `local-vm` that's the host
IP, SSH user and key path; never guess these.

### Step 2 — Build the dmg (skip if the user already has one)

The build has to run natively on Apple Silicon from inside the checkout you want
to package. The scripts locate the repo root themselves.

```bash
.agents/skills/run-macos-e2e/scripts/build_local_dmg_arm64.sh   # ~2 min warm, ~6 min cold
.agents/skills/run-macos-e2e/scripts/build_local_dmg_x86.sh     # ~10 min, runs under Rosetta
```

Before the first build, check the prerequisites in
[`references/setup.md`](references/setup.md): system Ruby on `PATH`,
`omnibus/.bundle/config`, gems built for the right architecture, and the Go
release from `.go-version`. Each one fails in a confusing way when it's missing.
The build is unsigned; setup.md lists how it differs from CI's signed build. Run the build in the background
and redirect output to a log.

Verify the result so you don't spend 30 minutes testing the wrong artifact:

```bash
pkgutil --expand-full omnibus/pkg/<name>.pkg /tmp/dmgcheck
file /tmp/dmgcheck/datadog-agent-core.pkg/Payload/bin/agent/agent   # expect arm64 / x86_64
```

### Step 3 — Check the target is ready

- **local-vm:** `ssh -i <ssh-key> <user>@<vm-ip> 'uname -m; eval "$(/opt/homebrew/bin/brew shellenv)"; docker ps'`.
  Fakeintake runs as a container on the host, so Docker has to answer. If SSH times out or
  Docker is down, go to [`references/troubleshooting.md`](references/troubleshooting.md).
- **aws-pool:** `aws-vault exec sso-agent-sandbox-account-admin-8h -- aws sts get-caller-identity`.
  Then decide between a clean and a fast run (Step 4).

### Step 4 — Choose clean or fast (aws-pool)

| Mode | Env | Cost | Use when |
|---|---|---|---|
| clean | `E2E_REVERT_BEFORE_RUN=true` | adds a root-volume replacement (several minutes) | final verification, after a run broke the host, results look polluted |
| fast | leave it unset, keep `E2E_DEV_MODE=true` | skips revert and re-provisioning | iterating on a fix |

Your local pool instance is never reverted on teardown (its lease is
persistent), so a fast run starts from whatever the previous run left behind.
The suites still reinstall the Agent either way. Details in
[`references/running.md`](references/running.md).

LOCAL_VM has no revert step. The host is reused as-is, and a clean run there
means restoring a VM snapshot by hand.

### Step 5 — Run

Show the user the full command and confirm before running it. Run it in the
background and watch the log. Go prints `--- PASS/FAIL` only when the whole
suite finishes, so look for `Error Trace` while it runs.

```bash
# local-vm — private ssh-agent: the framework's local SSH client only uses an agent,
# and the macOS launchd agent can refuse to load the key
eval "$(ssh-agent -s)"; trap 'ssh-agent -k' EXIT; ssh-add <ssh-key>
LOCAL_VM=<vm-ip> LOCAL_USER=<user> LOCAL_ARCH=arm64 \
E2E_MACOS_LOCAL_DMG="$PWD/omnibus/pkg/<name>.arm64.dmg" \
E2E_AWS_PRIVATE_KEY_PATH=<ssh-key> \
dda inv -- new-e2e-tests.run --targets=./tests/agent-platform/tests --run='^TestMacosInstallScript$'
```

For the config suite, keep the same variables and use
`--targets=./tests/fleet --run='^TestFleetConfigMacOS$'`.

The aws-pool commands for each mode are in
[`references/running.md`](references/running.md).
`E2E_MACOS_LOCAL_DMG` has to be an absolute path, because the test binary does
not run from the repo root.

### Step 6 — Report

Report pass/fail per subtest with durations, and confirm that the log shows
`DD_DMG_PATH=/tmp/e2e-local-agent.dmg`. That line proves your build was
installed, not the released Agent. For example:

```
TestMacosInstallScript — FAIL (14/15 passed, 9m02s) on <vm-ip> (arm64), dmg 7.86.0-devel.git.392.90221d1
  FAIL TestZYReinstallSameVersion (48s): <first Error Trace message>
  install: DD_DMG_PATH=/tmp/e2e-local-agent.dmg (local build confirmed)
```

On failure, check [`references/troubleshooting.md`](references/troubleshooting.md)
before touching code. Most early failures (under a minute, before any subtest)
come from the environment: Docker, SSH, the dmg path, or the AWS token.
