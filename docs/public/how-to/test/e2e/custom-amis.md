# Custom AMIs

Many E2E tests rely on extra dependencies being installed - instead of installing them at runtime (via an `apt install` or similar), we prefer to "bake" them into the underlying AMI that is used for provisioning the test VMs, as installing at runtime is a big source of flakiness (see [here](dependencies.md))
These images are pinned by ID in this repo (`test/e2e-framework/resources/aws/platforms.json`) and always associated with a specific OS Descriptior in the E2E framework (see below.)

The images themselves are built in [ami-builder](https://github.com/DataDog/ami-builder/tree/master/ami/images/e2e). This page covers the consumer side — how an image is chosen, how to add or bump one, and how to pin or inspect one.

## How an AMI is chosen

1. `ec2.WithOS(e2eos.Ubuntu2204E2E)` resolves to a `Descriptor{Ubuntu, "22-04-e2e", x86_64}` — a `(flavor, version, architecture)` triple.
1. No `WithOS` at all? The descriptor comes from a flavor default instead (`LinuxDescriptorsDefault`, `WindowsDescriptorsDefault`, or `MacOSDescriptorsDefault`).
1. Either way, `aws.GetAMI(descriptor)` looks that triple up as `platforms.json[flavor][arch][version]` and returns the pinned AMI ID.

| Piece | File |
|---|---|
| Descriptors (`Ubuntu2204E2E`, `WindowsServer2025`, …) | `test/e2e-framework/components/os/{linux,windows,macos}_descriptors.go` |
| Flavor defaults (`UbuntuDefault`, `WindowsServerDefault`, …) | same files |
| The AMI table | `test/e2e-framework/resources/aws/platforms.json` |
| The lookup | `test/e2e-framework/resources/aws/platforms.go` (`GetAMI`) |
| Resolution and fallbacks | `test/e2e-framework/scenarios/aws/ec2/os_resolver.go` |

The `version` field is a literal `platforms.json` key — hence `"22-04-e2e"`, not `"22.04"`.

/// tip | Seeing `version '22-04-e2e' not found in platforms.json`?
The descriptor exists but the table entry does not — check `platforms.json` for a typo'd or missing key.
///

/// details | Bypassing the pin with `ec2.WithLatestAMI()`
    type: info
    open: false

`ec2.WithLatestAMI()` bypasses `platforms.json` entirely and resolves the latest published image through SSM. It is deliberately rare in tests — it gives up reproducibility and it never gets you a prebaked image. Not every flavor can resolve a latest image; those log a warning and fall back to the pinned ID.

For manual QA the same switch is `dda inv aws.create-vm --latest-ami` (which sets `ddinfra:osImageIDUseLatest`). That is the right way to check whether a new upstream image breaks something, before bumping the pin.
///

## The `-e2e` AMIs

Most OS types have an `-e2e` variant defined in `platforms.json`: 

```json
"ubuntu": {
  "x86_64": {
    "22-04":     "ami-0347d82d55205687d",
    "22-04-e2e": "ami-09bcb097bdb06fabf"
  }
}
```

Both derive from the same upstream base image, but the `-e2e` one contains extra stuff: common test dependencies, maybe some config files etc. See [Prebake into the machine image](dependencies.md#prebake-into-the-machine-image) for more details.

/// danger | Prefer using the `-e2e` variant in new tests.

Unless overriden, this should be the default on both Linux and Windows — `UbuntuDefault` is `Ubuntu2204E2E` — so a test that passes no OS at all gets the prebaked image. A test that explicitly asks for `e2eos.Ubuntu2204` opts *out* of it, usually by accident.

If you need an extra dependency in your test, **prefer adding it to the existing `-e2e` variant over creating your own fully-custom AMI** if at all possible. Feel free to ask #agent-devx-help for help.
///


## Adding or bumping an image

The build lives in [ami-builder](https://github.com/DataDog/ami-builder); this repo only holds the resulting ID.

1. Make the image change in ami-builder under `ami/images/e2e/` and open a PR there.
1. Run the manual `build:<os>-<arch>-<version>` job in that branch's child pipeline. The build jobs are manual-only, and a feature branch produces a real usable AMI even without merging.
1. Read the ID out of the job log:
   ```
   Sharing new AMI ami-0123456789abcdef0 to account datadog-agent-sandbox
   ```
1. Update `test/e2e-framework/resources/aws/platforms.json` in this repo with that ID. Trigger a full pipeline on that PR — an image change can affect any suite, not only the ones whose files you touched.

/// warning
The scheduled `[bot] Update E2E AMIs` PRs **only refresh keys that already exist** in `platforms.json`. A brand-new key — a new OS version, a new `-e2e` variant — is built and shared but then silently dropped until someone adds the key by hand. Add the key in the same PR that starts consuming it.
///

### Adding a new image

To add a new custom AMI, you also need to tell the e2e framework about it: this is done by creating a new _OS descriptor_ alongside the `platforms.json` update
1. Update `platforms.json`, adding your new key in the appropriate spot.

1. Use that descriptor in your test: `ec2.WithOS(e2eos.yourNewDescriptor)`
1. `components/os/<family>_descriptors.go` — create a matching descriptor (`Ubuntu2204E2E` style naming).

## Pinning one test to a specific AMI

/// warning
Avoid doing this if at all possible - prefer using the `-e2e` variants, adding your dependency to them if needed.
///


`ec2.WithAMI` skips resolution altogether:

```go
ec2.WithAMI(
    "ami-04e7f0e0bde783f77", // https://gitlab.ddbuild.io/DataDog/ami-builder/-/jobs/1232214462
    compos.AmazonLinux2,
    compos.AMD64Arch,
)
```

The descriptor and architecture arguments must still match the image — they drive package-manager and path selection, not the lookup.

This is an escape hatch for a one-off image, as in `test/new-e2e/tests/npm/ec2_1host_selinux_test.go`. Always leave the build-job link in a comment, as above: an unattributed AMI ID cannot be rebuilt or audited. Do not reach for it to avoid getting a dependency prebaked properly — the ID will never be bumped by the bot and will rot.

## Inspecting an AMI

To poke at an image by hand — check that a tool really is installed, look at a service's state:

```bash
aws-vault exec sso-agent-qa-account-admin-8h -- \
  dda inv ami.launch-instance --ami-id ami-0123456789abcdef0 --key-name <your-key>
# then ssh (or RDP) to the printed private IP
```

`dda inv ami.create-ami` / `ami.delete-ami` can snapshot such an instance back into an AMI.

/// warning
That snapshot path is for **debugging only**. An image built by hand-editing a running instance is unreproducible, untracked, and will not be rebuilt when the base image is refreshed. To add a dependency for real, change the ami-builder provisioner script.
///

## Windows

Windows images need a few extra precautions when building or bumping one. Skip this section unless you are touching Windows E2E infrastructure.

- **SSH bootstrap runs from EC2 user data.** A fresh Windows instance has no SSH server; `test/e2e-framework/components/os/scripts/setup-ssh.ps1` (wired up in `scenarios/aws/ec2/os_win.go`) installs OpenSSH and the test's key on boot. If a Windows host is unreachable, read this script first.
- **A Packer build must end with `EC2Launch.exe reset -c`** (or its v1 equivalent, `Scripts\InitializeInstance.ps1 -Schedule`). EC2Launch tracks which boot-time setup tasks it has already run in a marker file, and Packer snapshots that marker along with the disk — so without a reset, every instance launched from the image thinks setup already happened and skips it. The task that actually bites is `extendRootPartition`: skip it, and `C:` never grows to the volume size the test asked for.
- **Build custom images from the `EC2LaunchV2-*` base AMIs**, not the plain `Windows_Server-*` ones. The plain bases ship EC2Launch v1, which bakes its IMDS route into the image itself; an image built in one subnet and launched in another can lose IMDS entirely, which silently breaks user data and SSH bootstrap with it.
- **Never set `<persist>true</persist>` on the build-time bootstrap script.** Only the consumer-facing user data (the one `setup-ssh.ps1` ships as) should persist. On EC2Launch v1, a persisted build-time script becomes a permanent scheduled task that re-runs on every instance launched from the image, fighting the real bootstrap.
