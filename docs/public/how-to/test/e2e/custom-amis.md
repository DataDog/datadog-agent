# Custom AMIs

E2E tests on AWS launch from AMI IDs pinned in this repository, not from whatever the distro publishes today. That buys two things: a test run is reproducible, and the image can ship the [dependencies a test would otherwise install at runtime](dependencies.md).

The pinning is the important half. If tests always took the latest published image, an upstream refresh would change the substrate under **every branch at once**, with no way to roll back and no bisectable commit — a class of incident we have had. Pinning converts that into a normal, reviewable PR that bumps one ID. For the same reason, what goes *into* an image is defined by a config file in ami-builder rather than by hand, so any image can be rebuilt.

We also deliberately keep images for platforms that upstream no longer supports, such as CentOS 7, because the Agent still supports them.

The images themselves are built in [ami-builder](https://github.com/DataDog/ami-builder/tree/master/ami/images/e2e). This page covers the consumer side — how an image is chosen, how to add or bump one, and how to pin or inspect one.

/// warning
`test/e2e-framework/resources/aws/platforms.json` (AMI IDs, the subject of this page) is a different file from `test/new-e2e/tests/agent-platform/platforms/platforms.json`, which lists `platform/arch/version` strings for the agent-platform install-test matrix and contains no AMI IDs.
///

## How an AMI is chosen

```
ec2.WithOS(e2eos.Ubuntu2204E2E)   ──▶  Descriptor{Ubuntu, "22-04-e2e", x86_64}
                                            │
        (no WithOS?  the flavor default from LinuxDescriptorsDefault
         / WindowsDescriptorsDefault / MacOSDescriptorsDefault)
                                            ▼
                       aws.GetAMI(descriptor)  ──▶  platforms.json[flavor][arch][version]
                                            ▼
                                        ami-0123…
```

| Piece | File |
|---|---|
| Descriptors (`Ubuntu2204E2E`, `WindowsServer2025`, …) | `test/e2e-framework/components/os/{linux,windows,macos}_descriptors.go` |
| Flavor defaults (`UbuntuDefault`, `WindowsServerDefault`, …) | same files |
| The AMI table | `test/e2e-framework/resources/aws/platforms.json` |
| The lookup | `test/e2e-framework/resources/aws/platforms.go` (`GetAMI`) |
| Resolution and fallbacks | `test/e2e-framework/scenarios/aws/ec2/os_resolver.go` |

A descriptor is a `(flavor, version, architecture)` triple, and `version` is a literal `platforms.json` key — hence `"22-04-e2e"`, not `"22.04"`. If you see

```
version '22-04-e2e' not found in platforms.json
```

the descriptor exists but the table entry does not.

/// info
`ec2.WithLatestAMI()` bypasses `platforms.json` entirely and resolves the latest published image through SSM. It is deliberately rare in tests — it gives up reproducibility and it never gets you a prebaked image. Not every flavor can resolve a latest image; those log a warning and fall back to the pinned ID.

For manual QA the same switch is `dda inv aws.create-vm --latest-ami` (which sets `ddinfra:osImageIDUseLatest`). That is the right way to check whether a new upstream image breaks something, before bumping the pin.
///

## The `-e2e` convention

Most flavors have two keys in `platforms.json`: a plain one and an `-e2e` sibling.

```json
"ubuntu": {
  "x86_64": {
    "22-04":     "ami-0347d82d55205687d",
    "22-04-e2e": "ami-09bcb097bdb06fabf"
  }
}
```

Both derive from the same upstream base image. The `-e2e` one has been booted by Packer, provisioned with the common test dependencies, and re-snapshotted. See [Prebake into the machine image](dependencies.md#prebake-into-the-machine-image) for what that includes.

Three kinds of entry end up in the table:

| Kind | How it is produced | Example |
|---|---|---|
| Copy of a public image | `ec2:CopyImage` of the upstream base into our account, so it cannot disappear from under us | `windows-server` `2025` |
| Modified public image | Packer boots the base, runs a provisioner script, snapshots the result | any `-e2e` entry |
| Hand-built custom image | The Packer workflow reproduced manually, for images that resist automation | old Ubuntu versions |

Avoid the third kind. It is unreproducible by definition, and it is why [Inspecting an AMI](#inspecting-an-ami) below carries a warning.

**Prefer the `-e2e` descriptor in new tests.** On Linux it is already the default — `UbuntuDefault` is `Ubuntu2204E2E` — so a test that passes no OS at all gets the prebaked image. A test that explicitly asks for `e2eos.Ubuntu2204` opts *out* of it, usually by accident.

## Adding or bumping an image

The build lives in ami-builder; this repo only holds the resulting ID.

1. Make the image change in ami-builder under `ami/images/e2e/` and open a PR there.
1. Run the manual `build:<os>-<arch>-<version>` job in that branch's child pipeline. The build jobs are manual, so a feature branch produces a real AMI without merging.
1. Read the ID out of the job log:
   ```
   Sharing new AMI ami-0123456789abcdef0 to account datadog-agent-sandbox
   ```
1. Update `test/e2e-framework/resources/aws/platforms.json` in this repo with that ID. Trigger a full pipeline on that PR — an image change can affect any suite, not only the ones whose files you touched.

/// warning
The scheduled `[bot] Update E2E AMIs` PRs **only refresh keys that already exist** in `platforms.json`. A brand-new key — a new OS version, a new `-e2e` variant — is built and shared but then silently dropped until someone adds the key by hand. Add the key in the same PR that starts consuming it.
///

### A brand-new `-e2e` variant

Three edits, in this order, so each commit is inert until the last:

1. `platforms.json` — the new `-e2e` key, alongside the plain one.
1. `components/os/<family>_descriptors.go` — the matching descriptor (`Ubuntu2204E2E` style naming).
1. Only once the AMI is smoke-tested: point the flavor default (`…Default`) or a version list at it.

Step 3 is the one with blast radius. `WindowsServerDefault`, for instance, is used by roughly fifty call sites, and `WindowsServerVersionsForE2E` seeds the random per-pipeline version choice for every Windows job. Verify the image by launching it (see [Inspecting an AMI](#inspecting-an-ami)) before flipping a default; a bad image takes out the whole platform rather than one suite.

## Pinning one test to a specific AMI

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

## Windows: what differs

Windows images need care that Linux ones do not. Skip this section unless you are touching Windows E2E infrastructure.

### Nothing runs before EC2 user data

A freshly-launched AWS Windows instance has no SSH server, so the framework cannot connect until something has installed one. That something is **EC2 user data**: a blob attached at launch which the guest agent — `cloud-init` on Linux, **EC2Launch** on Windows — fetches from the instance metadata service (`169.254.169.254`) and executes. It needs no inbound connection, which is what makes it the bootstrap escape hatch.

```
RunInstances(UserData = "<powershell>…setup-ssh.ps1…</powershell><persist>true</persist>")
   ↓
Windows boots, EC2Launch fetches the user data and runs it
   ↓  installs OpenSSH, writes the test's public key to administrators_authorized_keys
the framework's SSH connection succeeds → test runs
```

That script is `test/e2e-framework/components/os/scripts/setup-ssh.ps1`, embedded and wrapped by `scenarios/aws/ec2/os_win.go`. It is the first thing to read when a Windows host is unreachable, and it is shared with the Azure Windows provisioner.

### `.run-once`: why a custom Windows image needs a reset

EC2Launch does two jobs at boot. First its **built-in tasks**, which turn a generic disk image into a working instance; each has a *frequency*:

| Task | Frequency | What it does |
|---|---|---|
| `extendRootPartition` | once | Grows `C:` to fill the actual EBS volume |
| `setAdminAccount` | once | Randomizes the local Administrator password |
| `startSsm` | once | Starts the Systems Manager agent |
| `activateWindows`, `setDnsSuffix`, `setWallpaper` | once | Licensing, DNS suffixes, wallpaper |

`once` means "only on the image's first-ever boot", tracked by a marker file at `C:\ProgramData\Amazon\EC2Launch\state\.run-once`. Packer snapshots the whole `C:` drive — **including that marker** — so every instance launched from a Packer-built Windows image starts with the `once` tasks already considered done.

Two consequences:

- User data still runs. It is not a `once` task, and ours is tagged `<persist>true</persist>` besides, so SSH-key injection is unaffected. A non-generalized image does not break Windows E2E outright.
- The `once` tasks are skipped, and `extendRootPartition` is the one that bites: a test launching a larger root volume than the image's would not get `C:` grown into it.

So a Windows image build should run `EC2Launch.exe reset -c` as its last provisioner, which clears the state directory so the `once` tasks run again on the next boot. That restores the behaviour of the plain AWS images the tests use today. Full `EC2Launch.exe sysprep` is heavier than needed: it also disables RDP, regenerates the machine SID, and forces shutdown semantics that complicate the snapshot step.

Linux has no equivalent problem because `cloud-init` keys its per-instance state on the *instance ID* (`/var/lib/cloud/instances/<instance-id>/`), so a cloned image re-runs everything on a new instance automatically.

### EC2Launch v1 vs v2

AWS publishes two families of 2016/2019 base images. The plain `Windows_Server-*` AMIs ship EC2Launch **v1**; `EC2LaunchV2-Windows_Server-*` ships **v2**, as do all 2022/2025 images.

| | v1 (plain 2016/2019) | v2 |
|---|---|---|
| Location | `C:\ProgramData\Amazon\EC2-Windows\Launch\` | `C:\Program Files\Amazon\EC2Launch\EC2Launch.exe` |
| Re-run init on next boot | `Scripts\InitializeInstance.ps1 -Schedule` | `EC2Launch.exe reset` |
| Generalize | `Scripts\SysprepInstance.ps1` | `EC2Launch.exe sysprep` |
| IMDS / KMS static routes | **persistent** | non-persistent |
| `<persist>true</persist>` | a Windows scheduled task | agent state |
| `setComputerName` | on by default | off by default |

Build custom images from the `EC2LaunchV2-*` bases. The persistent-route row is the reason: AWS documents that v1's routes to the metadata service are *"captured as part of the OS configuration and any new instances launched from the AMI will retain the same routes, regardless of subnet placement"*. An image built in one subnet and launched in another can therefore lose IMDS — which means no user data, no SSH key, and a connection failure that presents as a flake. Coverage is unaffected: the OS under test is still Windows Server 2016, and EC2Launch is AWS guest-agent tooling that no Agent test exercises.

One rule follows from the same table regardless of version: **a Packer bootstrap must never use `<persist>`**. On v1 it bakes a scheduled task into the image that re-runs the build-time bootstrap on every consumer instance, fighting the test's own `setup-ssh.ps1`. The consumer user data keeps `<persist>`; the build-time one must not have it.

### Sources

- [EC2Launch v2 tasks](https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/ec2launch-v2.html) and [settings](https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/ec2launch-v2-settings.html)
- [EC2Launch v1 tasks](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2launch.html)
- [Create an AMI using Windows Sysprep with EC2Launch](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2launch-sysprep.html)
