# Running the macOS suites

## Suites

| Suite | Entry point | `--targets` | `--run` |
|---|---|---|---|
| install | `TestMacosInstallScript`, `test/new-e2e/tests/agent-platform/tests/macos_install_test.go` and `macos_launchdaemons_test.go` | `./tests/agent-platform/tests` | `'^TestMacosInstallScript$'` |
| upgrade | `TestMacosInstallScriptUpgradeFromStable`, `test/new-e2e/tests/agent-platform/tests/macos_upgrade_test.go` | `./tests/agent-platform/tests` | `'^TestMacosInstallScriptUpgradeFromStable$'` |
| config | `TestFleetConfigMacOS`, `test/new-e2e/tests/fleet/config_macos_test.go` | `./tests/fleet` | `'^TestFleetConfigMacOS$'` |

Anchor `--run` with `^…$`. The package contains Linux and Windows tests too, and
an unanchored `TestMacosInstallScript` matches both macOS suites, which is what
CI runs. Both suites can share one LOCAL_VM host because Go runs them one after
the other, and each starts by removing the installed Agent.

Both agent-platform suites pick their host with `macosProvisioner` and install your build with
`macosInstallAgentUnderTest` (`macos_install_test.go`).

The config suite is in another package, so it can't share those helpers. It reads
the same `LOCAL_VM`, `LOCAL_USER`, `LOCAL_ARCH` and `E2E_MACOS_LOCAL_DMG` variables in
`TestFleetConfigMacOS`, starts its fakeintake with `configMacOSStartLocalFakeIntake`
(pinned to the framework's fixed Remote Config signing seed, or the Agent's TUF root
validation fails), and installs through `agent.Agent.Install`
(`test/new-e2e/tests/fleet/agent/install.go`). It clears the Remote Config
store in `SetupSuite`, so a reused fakeintake (`E2E_DEV_MODE`) starts clean.

Timings measured on an arm64 Parallels VM: install ~4.5 min (15 tests,
~30 s of it installing the dmg); upgrade ~1.5 min. On the AWS pool, add
Pulumi provisioning and, for a clean run, the root-volume replacement.

## Environment variables

| Variable | Meaning |
|---|---|
| `E2E_MACOS_LOCAL_DMG` | Absolute path to a local `.dmg`. `macosInstallLocalDMG` copies it and the embedded `install_mac_os.sh` to the host and runs them with `DD_DMG_PATH`. Used for the install suite's SetupSuite and `TestZYReinstallSameVersion`, and as the upgrade target. `TestZYPostinstFallsBackWhenTheInstallerFails` extracts its postinstall script from it. Unset means `E2E_PIPELINE_ID`'s build, or the released Agent when that is unset too |
| `LOCAL_VM` | SSH host or IP of a pre-existing macOS host. Setting it switches from AWS to a static byohost descriptor (`provisioners.NewStaticStackProvisioner`), with no Pulumi and no cloud |
| `LOCAL_USER` | SSH user on that host (default `$USER`) |
| `LOCAL_ARCH` | `arm64` (default) or `x86_64`; it sets the descriptor architecture only |
| `E2E_AWS_PRIVATE_KEY_PATH` | Key used for the test's own `ssh` calls that start fakeintake on a LOCAL_VM host (`macosSSHArgs`). `E2E_LOCAL_PRIVATE_KEY_PATH` is read first, but leave it unset for a passphrase-protected key: the framework's own SSH client then reads that file instead of using the agent, and fails with `this private key is passphrase protected` |
| `E2E_DEV_MODE` | `true` keeps the Pulumi stack after the run (AWS) |
| `E2E_REVERT_BEFORE_RUN` | `true` reverts your AWS pool instance to its golden baseline before the tests start |
| `E2E_STACK_NAME_SUFFIX` | Appended to the stack name, so your stacks stay distinct |

## LOCAL_VM

```bash
eval "$(ssh-agent -s)"; trap 'ssh-agent -k' EXIT; ssh-add <ssh-key>
LOCAL_VM=<vm-ip> LOCAL_USER=<user> LOCAL_ARCH=arm64 \
E2E_MACOS_LOCAL_DMG="$PWD/omnibus/pkg/<name>.arm64.dmg" \
E2E_AWS_PRIVATE_KEY_PATH=<ssh-key> \
dda inv -- new-e2e-tests.run --targets=./tests/agent-platform/tests --run='^TestMacosInstallScript$'
```

Why the private ssh-agent: the framework's SSH client for a `local` host
authenticates only through an agent, and the default macOS launchd agent can
refuse `ssh-add` for some keys ("agent refused operation"). A short-lived agent
avoids both problems and is gone when the shell exits.

There is no revert. Each run starts from whatever the last one left behind,
and both suites remove and reinstall the Agent anyway. For a truly clean host, restore a
VM snapshot.

`TestNPMTracesConnection` is skipped on LOCAL_VM; see `troubleshooting.md`.

## AWS pool: clean run vs fast run

How a local (non-CI) pool run works. Source:
`test/e2e-framework/scenarios/aws/ec2/vm.go` (pool acquisition) and
`test/e2e-framework/resources/aws/ec2/pool/pool.go`.

- `pool.Acquire` only considers instances tagged
  `dd:macos-e2e-pool-owner=<your OS username>` (`OwnerUsernameTagKey`). If you
  own none, `Found` is false and the run **provisions a new instance on a new
  Dedicated Host** (24-hour billing minimum). If yours is already leased by a
  different stack, acquisition fails with `no idle instance available`.
- Locally provisioned leases are `Persistent`. `pool.RevertAndRelease` never
  reverts a persistent lease's root volume at teardown; it only releases the
  lease. The disk keeps everything the last run did.
- Teardown always releases the lease (`BaseSuite.releasePoolInstanceIfAny`
  runs through a `defer`), including in dev mode. Dev mode only skips the
  Pulumi stack destroy.

### Full clean run: revert the machine first

```bash
aws-vault exec sso-agent-sandbox-account-admin-8h -- env \
  E2E_MACOS_LOCAL_DMG="$PWD/omnibus/pkg/<name>.x86_64.dmg" \
  E2E_DEV_MODE=true \
  E2E_REVERT_BEFORE_RUN=true \
  E2E_STACK_NAME_SUFFIX=<name> \
  dda inv -- new-e2e-tests.run --targets=./tests/agent-platform/tests --run='^TestMacosInstallScript$'
```

With `E2E_REVERT_BEFORE_RUN=true`, after acquiring your existing instance the
framework calls `pool.RevertInPlace`. That resolves the lease's baseline AMI to
its root EBS snapshot, replaces the instance's root volume with it
(`CreateReplaceRootVolumeTask`, polled until it succeeds), and re-publishes the
lease as in-use. The tests then run on a host identical to the golden image.
This adds several minutes. It only applies to an instance you already own; a
freshly provisioned one is clean already.

Use it for a final verification before asking for review, after a run that
crashed mid-experiment or left launchd or `/opt/datadog-packages` in an odd
state, or whenever a failure might be leftover state rather than your change.

### Fast run: reuse the machine as-is

```bash
aws-vault exec sso-agent-sandbox-account-admin-8h -- env \
  E2E_MACOS_LOCAL_DMG="$PWD/omnibus/pkg/<name>.x86_64.dmg" \
  E2E_DEV_MODE=true \
  E2E_STACK_NAME_SUFFIX=<name> \
  dda inv -- new-e2e-tests.run --targets=./tests/agent-platform/tests --run='^TestMacosInstallScript$'
```

With no revert and the Pulumi stack kept by dev mode, a rerun skips both the
root-volume replacement and re-provisioning. The suites still reinstall the
Agent: both suites call `removePreInstalledAgent` first. The trade-off is that
anything the previous run left outside those paths survives, for example stale
launchd jobs, leftover experiment LaunchDaemons, or files under
`/opt/datadog-packages`. The kept stack also keeps its fakeintake. If a fast run
fails in a way that doesn't make sense, rerun clean before debugging.

### AWS auth

`aws sts get-caller-identity --profile …` on its own doesn't refresh an expired
SSO token. Use `aws-vault login <profile>` (device-code browser flow) or
`aws-vault exec <profile> -- …`. If SSO prompts come back faster than every
8 hours, a non-`-8h` profile is being used somewhere.

## Watching a run

Run the command in the background with output redirected to a log. Go prints
`--- PASS/FAIL` for subtests only when the whole suite finishes, so watch for
these lines while it runs:

- `Error Trace` / `Messages:` — an assertion failed (the suite keeps going)
- `DD_DMG_PATH=/tmp/e2e-local-agent.dmg` — SetupSuite is installing your local build
- `=== RUN   Test…/` — which subtest is running now

A failure in under a minute, before any `=== RUN` of a subtest, is almost
always environmental. See `troubleshooting.md`.
