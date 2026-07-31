<!--
DRAFT — this file belongs in the ami-builder repository, at ami/images/e2e/README.md
(replacing the current one-line placeholder). It is drafted here only because
that is where the ACIX-1920 work was done; move it over as-is.

Two caveats before moving it:
  - The "Windows images" section describes PR A's design. Land it with PR A, not before.
  - Cross-repo links point at datadog-agent by URL, since the two repos are separate.
-->

# E2E AMIs

The machine images used by the Datadog Agent's E2E test suite
([`test/new-e2e`](https://github.com/DataDog/datadog-agent/tree/main/test/new-e2e)). The consumer
side — how a test picks an image, and how an ID gets bumped in `datadog-agent` — is documented at
[Custom AMIs](https://datadoghq.dev/datadog-agent/how-to/test/e2e/custom-amis/). This file covers
the build side.

Owner: **#agent-devx** (the repository root README describes only the KMT images, which are owned by
#ebpf-platform).

## Layout

| Path | Role |
|---|---|
| `config.yaml` | The image catalogue: which images exist and how each is built. Hand-edited. |
| `template.yaml` | Defaults merged into every entry (Packer template, `ssh_username`, volume size, tags, target accounts). |
| `amis.yaml` | SSM parameter paths for the upstream base images, used to refresh `config.yaml`'s `source_ami` values. |
| `generate.py` | Drives everything: pipeline generation, per-image config generation, source-AMI refresh, AMI-ID scraping. |
| `eks_nodes.yaml` | EKS node AMI versions (unrelated to the VM images). |
| `pipeline_stub.yaml` | Extra jobs appended to the generated pipeline. |
| `config/`, `pipeline.yml` | **Generated**, gitignored. |

The Packer templates and provisioner scripts live one level up, in `ami/packer/`.

## Copy or build

Each entry takes one of two paths, decided by `Image.__init__` in `ami/utils/image.py`:

```python
self.only_copy = (
    self.e2e_image
    and options['packer'].get('provisioner_script', None) is None
    and not options['packer'].get('disable_unattended_upgrades', False)
)
```

| Path | What happens | When |
|---|---|---|
| **Copy** | A plain `ec2:CopyImage` of the upstream base into `datadog-agent-qa`. No instance is launched, Packer is never invoked. | The entry sets neither `provisioner_script` nor `disable_unattended_upgrades`. |
| **Build** | Packer launches an instance from the base image, runs the provisioner scripts over it, and snapshots the result. | The entry sets either of those two keys. |

By convention the plain key (`ubuntu.x86_64."22-04"`) is a copy of the pristine upstream image and
the `-e2e` sibling (`"22-04-e2e"`) is the built, batteries-included variant. Both point at the same
`source_ami`; see [Refreshing base images](#refreshing-base-images) for why that matters.

/// note
A build does **not** require a runner matching the image's OS. Packer's `amazon-ebs` builder
provisions a real EC2 instance and talks to it over the network, so a Linux CI runner can build a
Windows image.
///

### The account flow

```
build path                                  copy path
──────────                                  ─────────
packer build                                assume role/deploy-ami in datadog-agent-qa
  ambient credentials, no role assumption      ec2:CopyImage in-account
  instance into build-stable (486234852809)    tag
    subnet-08b7a8aafa1ab2918                   share with datadog-agent-sandbox
    sg-0d6e77ad76292130c
    instance profile ci-packer-instance
  → intermediate AMI in build-stable
share AMI + snapshots with datadog-agent-qa
assume role/deploy-ami, CopyImage into agent-qa
tag, share the copy with datadog-agent-sandbox
deregister the build-stable intermediate
```

Only the build path touches `build-stable`, which is why the copy-only entries have never needed
credentials there.

## `config.yaml`

Three levels of nesting — `<os> → <arch> → <version> → entry` — with `arch` one of `x86_64` or
`arm64`. `template.yaml` is deep-merged underneath each entry, and the entry wins on conflict
(`AMIConfigGenerator.merge_configs`).

```yaml
ubuntu:
  x86_64:
    "22-04-e2e":
      packer:
        source_ami: ami-00de3875b03809ec5
        provisioner_script: provision-e2e-apt
        ssh_username: ubuntu
        disable_unattended_upgrades: true
```

Entry keys: `packer` (required) and `suffix` (optional — appended to the job name and the AMI name).
Inside `packer`:

| Key | Effect |
|---|---|
| `source_ami` | **Required.** The upstream base image. Also recorded in the `OriginAmi` tag. |
| `provisioner_script` | Filename stem under `ami/packer/provisioners/scripts/`. **Setting this forces a Packer build.** |
| `template` | Packer template stem under `ami/packer/`. Inherited as `e2e-tests-ami`; an entry may override it. |
| `ssh_username` | Overrides the inherited `ec2-user`. |
| `root_volume_size` | String. Overrides the inherited `"50"`. Must be ≥ the base image's volume. |
| `instance_type` | Defaults to `t3.medium` (x86) / `m6g.medium` (arm). |
| `disable_unattended_upgrades` | Bool. Passed to `disable-unattended-upgrades.sh`; **also forces a build.** |

Entries worth reading as precedent:

- `amazon-linux."2023"` — minimal copy-only entry.
- `redhat."9"` — `provisioner_script: noop`, forcing a build purely because Marketplace AMIs cannot
  be `CopyImage`'d cross-account.
- `centos."7-e2e"` — `root_volume_size: "200"`.
- `ubuntu."18-04-cuda-430"` — `suffix` plus a custom `instance_type`.

## Provisioner scripts

Under `ami/packer/provisioners/scripts/`:

| Kind | Files |
|---|---|
| Per-family orchestrators, named by `provisioner_script` | `provision-e2e-apt.sh`, `provision-e2e-amazon-linux.sh`, `provision-e2e-rhel-centos.sh`, `provision-e2e-suse.sh` |
| Shared helpers, uploaded to `/tmp/` for **every** build and invoked by the orchestrators | `setup_docker.sh`, `install_awscli.sh`, `precache_ansible.sh` |
| Do-nothing script for entries that must build but need no provisioning | `noop.sh` |

Every Linux `-e2e` image therefore ships Docker, `docker-compose` v2.27.0,
`amazon-ecr-credential-helper`, the AWS CLI v2, `jq`, `python3` with `pip`, `ansible` with the
`datadog.dd` collection pre-cached, and an `ab` client. Family-specific extras are gated inside the
orchestrators (Node.js 20, `php` and `stress` on Ubuntu; `fapolicyd` on RHEL).

/// note
The `e2e-tests-ami.json` template hardcodes both the `scripts/` directory and the `.sh` extension in
its payload provisioner, so a non-shell provisioner needs its own template file.
///

Provisioner paths in the templates are resolved relative to the **repository root**, so `ami-build`
must be run from there.

## Building one image

Nothing builds on merge. Every `build:<os>-<arch>-<version>` job is `when: manual`, which also means
a feature branch can publish a real AMI without merging anything.

1. Edit `config.yaml` (and add the provisioner script / template if needed), push the branch, open a PR.
1. Run `build:generate-e2e-pipeline`. Its log lists the jobs it generated — check yours is there.
1. `build:trigger-e2e-pipeline` creates the child pipeline, where `generate-e2e-config` runs
   automatically. Press ▶ on your `build:<os>-<arch>-<version>` job there.
1. A successful job ends with:
   ```
   Sharing new AMI ami-0123456789abcdef0 to account datadog-agent-sandbox
   ```
1. Open a `datadog-agent` PR putting that ID in `test/e2e-framework/resources/aws/platforms.json`.

/// warning
That log line is not just informational — `generate.py` recovers AMI IDs by grepping job logs for
`Sharing new AMI (ami-[a-f0-9]+) `. Changing the message, or removing an image's `share:` list, means
the ID is silently never propagated.
///

/// warning
The scheduled updater only rewrites `platforms.json` keys that **already exist**. A brand-new key —
a new OS version, a new `-e2e` variant — is built and shared, then dropped. The `datadog-agent` PR
that adds the key must come first (or in the same change that starts consuming it).
///

### Local builds are not possible

`packer build` launches into `build-stable`'s subnet and security group with the
`ci-packer-instance` profile. `iam:PassRole` on that profile and `ec2:RunInstances` in that account
are both denied to the most privileged role a non-CI-infra developer holds there, and the second
denial rules out overriding `-var instance_profile=` too.

So iteration is one manual CI job per attempt, with the job log as the only feedback channel. Write
provisioner scripts accordingly: log which branch every detection took, and keep them idempotent.

Debugging levers:

- `PACKER_LOG=1`.
- `-on-error=abort` leaves the instance running for inspection. Do **not** use `-on-error=ask`: it
  prompts on stdin and will hang the job until the 2h timeout.
- `aws ec2 get-console-output --instance-id … --latest`, readable from `build-stable` with the
  `developer` role. This is the only way to see whether the guest agent ran the user data at all,
  which is the failure mode when a build hangs before the first provisioner.

The expensive failure is a *hang*, not an error: a bootstrap that never opens the communicator burns
the whole connection timeout. Keep that timeout tight enough to iterate on.

## Refreshing base images

`amis.yaml` maps each base version to a rule that resolves the current upstream image, in one of two
forms:

```yaml
"2016":
  ssm: /aws/service/ami-windows-latest/Windows_Server-2016-English-Full-Base   # SSM parameter
"9":
  query: "RHEL-9.6*"                                                          # ec2 describe-images name filter
```

Use `ssm` where the vendor publishes a parameter. `query` is the fallback for vendors that don't
(RHEL, Fedora, the ECS-optimized Amazon Linux images) — note that `describe-images` results depend on
the calling account, so resolve it with credentials for the account the images are shared into.

`generate.py update-ami-config` reads those rules and rewrites `config.yaml`. Three ways to run it:

| How | When |
|---|---|
| The scheduled `update:update-e2e-config-latest-amis` job (`.gitlab/auto-update.yml`) | Normal path — apply its diff |
| `aws-vault exec sso-build-stable-developer -- python3 ami/images/e2e/generate.py update-ami-config` | Locally, when you need it now. May require Python 3.11. |
| The scheduled `update:launch-full-e2e-ami-update` job | Full chain, see below — not a validation tool |

The rewrite is a **textual `str.replace`** over `config.yaml`, chosen to preserve comments. Two
consequences:

- `-e2e` entries normally carry no `amis.yaml` key of their own. They stay current for free by
  sharing the exact `source_ami` *string* with their base sibling, so the replace hits both.
- If an `-e2e` entry needs a *different* base image than its sibling, that free ride is gone and it
  must get its own explicit `amis.yaml` key, or it silently goes stale.

/// note
The `count(old_ami) != 1` branch prints `WARNING: … Skipping <name> update`, but the replace happens
regardless — the warning is cosmetic. Shared `source_ami` values are safe and intended.
///

The scheduled `update:launch-full-e2e-ami-update` job chains the whole thing: PR a refreshed
`config.yaml`, wait for its pipeline, play every manual `build:*` job, scrape the AMI IDs, then PR
`platforms.json` in `datadog-agent`. It is not a validation tool — don't use it to test a change.

## Windows images

Windows entries were copy-only for a long time, which is why nothing in this repository mentioned
PowerShell, WinRM or EC2Launch. Building one needs more care than the Linux path; the background is
in
[Custom AMIs § Windows](https://datadoghq.dev/datadog-agent/how-to/test/e2e/custom-amis/#windows-what-differs).
The rules that apply here:

**Use the `EC2LaunchV2-Windows_Server-*` base images** for 2016 and 2019 (2022 and 2025 already ship
EC2Launch v2). v1 writes *persistent* static routes to the metadata service that are captured into a
custom AMI "regardless of subnet placement" — this repository builds in one subnet and the tests
launch in another, so a v1-based image risks unreachable IMDS, which surfaces as unexplained
connection flakes. Those two entries need their own `amis.yaml` keys, since their `source_ami` no
longer matches their base sibling's.

**The bootstrap user data must not use `<persist>`.** Persisted user data is baked into the image and
re-runs on every consumer instance, where it would fight the test's own `setup-ssh.ps1` over
`administrators_authorized_keys`.

**Run `EC2Launch.exe reset -c` as the last provisioner.** Packer snapshots the whole `C:` drive
including `%ProgramData%\Amazon\EC2Launch\state\.run-once`, so without a reset every instance
launched from the image skips EC2Launch's `once`-frequency tasks — including `extendRootPartition`.
Today's copy-only Windows images run those tasks on every VM; the reset preserves that. Full
`sysprep` is not warranted: it also disables RDP, regenerates the machine SID, and forces shutdown
semantics that complicate the snapshot step.

**Two `.ps1` idioms** the Linux scripts don't need: an explicit
`[Net.ServicePointManager]::SecurityProtocol = 'Tls12'` (Server 2016's .NET default does not
negotiate TLS 1.2) and `-UseBasicParsing` on `Invoke-WebRequest` (a Server image has no IE DOM). For
MSI installs, accept exit codes `0` and `3010` (success, reboot pending) and retry on `1618`
(another install in progress).

### What is baked, and what still happens at boot

`datadog-agent`'s `components/os/scripts/setup-ssh.ps1` runs as user data on every Windows E2E VM.
Its install block is *gated*, and the gate skips the whole block — not just the download. So
anything the block configures has to be baked, or it will simply be missing.

| Concern | Baked into the `-e2e` image | Still done at every boot |
|---|---|---|
| AWS CLI v2 at `C:\Program Files\Amazon\AWSCLIV2\aws.exe` | yes | — |
| Win32-OpenSSH Server MSI | yes | skipped when already present |
| `OpenSSH-Server-In-TCP` firewall rule | **yes** — skipped along with the install block | — |
| `HKLM:\SOFTWARE\OpenSSH\DefaultShell` → `powershell.exe` | **yes** — same reason | — |
| `sshd` StartupType `Automatic` | **yes** — same reason | — |
| `administrators_authorized_keys` | no, and must not be | rewritten with the test's key |
| `Start-Service sshd` | no | every boot |
| `SSH-Server-DD-Universal` firewall rule (WS2025) | no | every boot |
| `icacls C:\ /inheritance:d` (WS2025) | no | every boot |
| Edge auto-update disable | no | every boot |
| WinRM listener (build-time only) | no — should not be serving on a test VM | — |

Windows Server 2025 is the exception on the OpenSSH row: Microsoft preinstalls its own `sshd` under
`C:\Windows\System32\OpenSSH`, so the consumer script force-reinstalls the MSI build regardless of
what the image ships. Baking OpenSSH only helps there once `setup-ssh.ps1` discriminates on the MSI
path (`C:\Program Files\OpenSSH\sshd.exe`).

### Smoke-checking a produced image

Launch it and confirm:

- `& "C:\Program Files\Amazon\AWSCLIV2\aws.exe" --version` works.
- `Get-Service sshd` — StartupType `Automatic`, running; `DefaultShell` set; `OpenSSH-Server-In-TCP`
  enabled.
- `setup-ssh.ps1`'s log shows it skipped the install block.
- The root volume matches the requested size, confirming `reset` restored `extendRootPartition`.
- `winrm enumerate winrm/config/listener` — the build-time listener must not be serving.

## Out of scope

`ami/packer/final-cleanup.sh` is not referenced by `e2e-tests-ami.json` — it is used by `ebs.json`
and the two kernel-version-testing templates. It is dead on the e2e path only; do not delete it.

There is no PowerShell linting in this repository, and `ci-shellcheck` does not cover
`ami/packer/provisioners/scripts/`.
