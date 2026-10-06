# Azure Files log rotation E2E

This native `new-e2e` suite checks that log lines written to an Azure Files
share, across rotations, reach Fakeintake exactly once. It covers the two ways
the Agent can read such a share:

```text
writer pod --------> its own CIFS mount -> Azure Files share
                                               |
              +--------------------------------+---------------------------+
              |                                                            |
 file source: the Agent's own CIFS mount                 smb source: the Agent's SMB client,
 (independent of the writer's)                           no mount on the Agent side
              |                                                            |
              +-----------------------> Fakeintake <-----------------------+
```

The stock Azure provisioner creates one AKS node and a VM-hosted Fakeintake. The
suite provisions in two passes. The first pass creates the Azure Files shares
and their Kubernetes secrets without an Agent. The second pass updates that
same stack to install the selected Agent and then starts the writers.
This keeps setup time from looking like lost rotations. The Agent and the
writers use separate inline CSI volumes, so Linux creates independent CIFS
mounts to the same share.

Storage accounts and shares are created through the generic `azure-native`
resource tokens, so the suite needs no storage SDK module.

Fakeintake keeps payloads for 2 hours instead of its 15-minute default, so the
last cell of a full matrix still finds its first rotation, and it does not
forward anything to a Datadog org.

## Cells

A cell is one entry in `cells` in `provisioner.go`. Each cell gets its own
storage account, share, writer Deployment, Agent log source and service name
(`azure-files-<cell>`), so cells are independent measurements inside one run.

| Cell | Agent reader | Fingerprint | Mount options | What it proves |
|---|---|---|---|---|
| `file-line` | `type: file` over the Agent's CIFS mount | `line_checksum`: one line, at most 4096 bytes | `actimeo=1` | A file source detects each rotation and drains the rotated file |
| `file-byte` | `type: file` over the Agent's CIFS mount | `byte_checksum`: 2048 bytes | `actimeo=1` | The same with byte fingerprints, which see the head of the file rather than its first line |
| `file-line-actimeo30` | `type: file` over the Agent's CIFS mount | `line_checksum` | `actimeo=30` | Same as `file-line` with only the attribute cache lifetime changed |
| `smb` | `type: smb`, the native SMB source; runs only with `E2E_SMB_AZURE=1` | none | `actimeo=1` on the writer only | The SMB source follows a rotated file by its server FileId and drains it, without any mount on the Agent side |

`actimeo=1` is the value every measurement so far has used. `actimeo=30` is the
low end of what Microsoft recommends for Azure Files on Linux, and their
documentation warns that lower values cost performance, so real deployments
are likely to run 30 or more. Every mount also uses `cache=strict`,
`nosharesock`, `serverino`, `closetimeo=0` and `persistenthandles`.

The writers rotate each minute and vary completed file sizes around 81-85 KB.
After four rotations, the test checks the first three completed files against
Fakeintake: every writer sequence must arrive exactly once, and every log of
the cell's service must carry `source:java` and the `e2e_run_id` and
`e2e_cell` tags of its log source.

`AZURE_FILES_E2E_CELLS` restricts a run to named cells, so a single cell can be
provisioned instead of the whole matrix. An unknown name fails before any cloud
resource is created.

Because every cell has its own share, the SMB source's opens never touch the
files the CIFS cells read, and the reverse.

## Profiles

`logs_config.unreliable_mount.enabled` applies to every file source on an
Agent, and the suite runs a single Agent. `AZURE_FILES_E2E_PROFILE` therefore
selects it for the whole run, and comparing the two settings takes one run
each:

| Profile | `unreliable_mount.enabled` | How a file source drains a rotated file |
|---|---|---|
| `default` (when unset) | `false` | Keeps reading it next to the replacement for `close_timeout`, then closes it |
| `unreliable-mount` | `true` | Uses O_DIRECT fingerprint reads and hands the path over: the replacement waits until the rotated file has gone 30s without new reads, or until `rotation_drain_timeout` (60s) |

The profile changes nothing else: the cells, their fingerprint settings and
mount options are identical in both. The `smb` cell is not affected by
`unreliable_mount`, so it measures the same reader in both runs.

The suite pins `file_scan_period: 1` and lowers `close_timeout` to 5s for both
profiles, so that a single late marker (below) lands past every reader's drain
window and still before the next rotation. These settings and
`unreliable_mount.enabled` are set through `agents.containers.agent.envDict`.
A `datadog.env` list would replace the framework's own list, which points the
Agent at Fakeintake, because Helm replaces lists instead of merging them.

This replaces the per-source `rotation_handoff_mode: sequential`,
`sequential_rotation_*` and `fingerprint_config.open_flags` settings of the
earlier version of this suite. Those keys no longer exist; their behaviour is
now the `unreliable-mount` profile.

## Post-rename markers

The writer rotates like a Log4j2 `RollingFile` appender: it closes the active
file, renames it, and only then reopens the active path, so it never appends to
a rotated file. That makes it unable to produce the loss this suite cares
about: a writer that keeps flushing to the old file after the rename, past the
point where the draining reader decides the rotated file is finished.

The `appender` sidecar supplies exactly those late appends. It shares the writer
pod, so it appends through the same CIFS mount that performed the rename. On
each new rotated file it appends one marker line to the rotated path shortly
after the rename (1.5s for the file cells, 0.5s for the `smb` cell), and
another at 45s. Every attempt is journalled to `markers.jsonl` on the share,
and each marker carries a `marker_id=<run-id>-<cell>-r<rotation>-m<age-ms>`
identity, so the test can tell exactly which append is missing.

The marker ages straddle each reader's drain window on purpose. See the marker
constants at the top of `provisioner.go`:

| Reader | Drain window after it sees the rotation | Early marker |
|---|---|---|
| file source, `default` | `close_timeout`: 5s | 1.5s |
| file source, `unreliable-mount` | 30s without new reads, at most 60s | 1.5s |
| SMB source | until two 1s polls in a row find no new data, at most `close_timeout` (5s) | 0.5s |

- The early marker must be collected exactly once, because it lands while the
  cell's reader is still reading the rotated file.
- The 45s marker is expected to be lost. That is the calibration point: it is
  what proves the suite can observe this loss at all.

The SMB source counts the poll made in the scan that detects the rotation as
the first of its two idle polls. A rotated file that stopped growing, which is
what the writer leaves behind, is therefore dropped one poll interval after the
source sees the rotation, and it can see it right after the rename. A 1.5s
marker would be lost on about half of the rotations, depending on where the
rename falls between two polls. The 0.5s marker lands before the second poll
even when the appender notices the rename one of its 200ms polls late. Every
read of new data restarts the idle count, so the early marker also keeps the
drain open; that is why a cell cannot probe a later age with a third marker.

A run where the 45s marker survives is suspicious, not a pass. Investigate the
harness before the product: it usually means the marker no longer lands past
the drain window. One known case: a drain window opens when the Agent sees the
rotation, and with `actimeo=30` a file source can see it up to 30s late. In the
`unreliable-mount` profile the window of `file-line-actimeo30` can then still
be open at 45s.

The SMB source's window comes from its code (poll interval, two idle polls,
`close_timeout`) and has not been measured yet. Check the first `smb` results
against `smb-markers.jsonl` before relying on its calibration.

## Credentials

Each cell's storage account key lives only in the Pulumi state (as a secret)
and in Kubernetes Secrets. None of it is a stack output, a Helm value or a
conf.d value.

- `azure-files-storage-<cell>` in the `azure-files-e2e` namespace holds the
  account name and key. The CSI driver reads it for the CIFS mounts, the
  writer's included.
- For the `smb` cell, the storage pass also copies the key into a Secret of
  the same name in the Agent namespace (`datadog`). A Secret volume can only
  reference a Secret of its pod's namespace. The pass runs before the Agent is
  installed, so the Secret exists when the Agent pod starts.

The Agent pod mounts that copy at `/etc/azure-files-secrets/smb/` (mode 0400),
and the SMB source names the file through the secret backend:

```yaml
password: "ENC[file@/etc/azure-files-secrets/smb/azurestorageaccountkey]"
```

The Helm values set `secretBackend.command: /readsecret_multiple_providers.sh`
and `enableGlobalPermissions: false`. Reading a mounted file needs no
Kubernetes API access, so the Agent gets no permission on Secrets; the chart
default would let it read every Secret in the cluster. The chart mounts
`agents.volumeMounts` into every container of the Agent pod, so the key file
is visible to all of them. Only the core Agent resolves the handle.

## Retention and evidence

A passing run destroys its stack. A failing run keeps the AKS cluster,
Fakeintake VM, storage accounts, shares and pods for investigation
(`e2e.WithSkipDeleteOnFailure`). Set `E2E_DEV_MODE=true` to keep them after a
pass too. Each run has its own stack and storage accounts, named after a run ID,
unless `AZURE_FILES_E2E_STACK` names a stack to reuse (see
[Keep the stack up and iterate](#keep-the-stack-up-and-iterate)).

Evidence is written under the framework session output directory, not `/tmp`.
Every evidence file has the `smb` account key replaced by
`[redacted storage account key]`. If the test cannot read the Secret, it logs
that it cannot redact the key and writes no Agent output or Fakeintake logs to
the evidence. The evidence contains:

- writer ledgers, marker journals and pod manifests;
- writer, ledger, appender and Agent container logs;
- the kernel version and effective mount options on both sides;
- active-path metadata and the Agent's file descriptors;
- `agent status --verbose` and `agent status --json --verbose`, which show
  SMB connection and authentication errors and every tailer;
- `smb-<agent-pod>-source.json`: the SMB source's status, inputs, messages,
  `Bytes Read`, its tailers, whether `agent secret` lists the password handle
  as resolved, and the Agent-wide `BytesMissed` counter (see below);
- the Agent registry snapshot;
- `azure-files-run.json`: the run ID, stack, writer workload (the stock
  image and the checksum of its ConfigMap scripts, or the Java writer image),
  profile, cells (with the SMB source's host, share, user, password handle and
  poll interval), storage accounts, shares, and the gated cells that were
  skipped.

The test prints the evidence directory and the run's resource names at the end.

To destroy a kept stack, run `dda inv new-e2e-tests.clean -s` and select the
stack the test printed (`azure-files-<hash>`, or the `AZURE_FILES_E2E_STACK`
name). Then check that nothing is left:

```sh
az resource list -g dd-agent-sandbox --tag username=$USER -o table
```

## The writer workload

Each cell's writer pod runs three containers on the cell's share: the
`writer`, which writes sequence-numbered records and rotates `app.log` every
minute; the `ledger`, which records every rotated file in `ledger.jsonl`; and
the `appender` (see [Post-rename markers](#post-rename-markers)).

By default the pods run a stock public image, so a run needs no image build
and no registry push:

- the image is `python:3.12.15-slim-bookworm`, pinned by the digest of its
  multi-arch index (`stockWorkloadImage` in `provisioner.go`). It comes
  through the environment's Docker Hub mirror, which on Azure is Docker Hub
  itself;
- the `azure-files-workload` ConfigMap holds `workload/logwriter.py`,
  `workload/ledger.sh` and `workload/appender.sh`, embedded in the test
  binary, and is mounted read-only at `/app`, where the Java image keeps its
  own copies. The pod template carries a checksum of the scripts, so a reused
  stack restarts the writers when a script changes;
- the containers run as UID 1000; the Java image runs as a non-root user too.

`logwriter.py` is a port of the Java writer in `workload/src`. It reads the
same `LOGWRITER_*` variables, and it keeps everything the assertions read from
the share:

- record lines: the log4j2 `LOG_PATTERN` of `log4j2.xml`
  (`%d{yyyy-MM-dd HH:mm:ss.SSS}  %-5level %pid --- [%20t] %-40c{1.} : %msg%n`,
  in UTC), the same `run_id=... period=... sequence=... record=... phase=...
  target_bytes=... host=... payload=...` message, the same level for each
  sequence, and one write per line;
- rotation: the first record of a new UTC minute closes `app.log`, renames it
  to `app.log.<ddMMyyyy_HHmm>` after the minute the file covers, and only then
  opens a new `app.log`, like the `TimeBasedTriggeringPolicy` of `log4j2.xml`;
- file sizes: each period is filled to the next value of
  `LOGWRITER_TARGET_BYTES_SEQUENCE` and ends in raw `p` padding without a
  newline, so the appender's first marker extends that line;
- schedule: a 250ms fixed-delay tick, a head record, a 5s head pause, and no
  first period with 15s or less left in its minute;
- ledger: `ledger.sh` is the same script. It computes its CRC64 values with
  `logwriter.py crc64` instead of the Java image's `Crc64` class
  (`LOGWRITER_CRC64_COMMAND`); both print Go's `hash/crc64` ISO checksum.

`workload_test.go` runs `logwriter.py` on a simulated clock, then `ledger.sh`
and `appender.sh` on its files, and checks them against that contract (see
[Checks that do not create cloud resources](#checks-that-do-not-create-cloud-resources)).
A change to what either writer writes has to be made in the other one too.

### Use the Java writer image

Set `AZURE_FILES_E2E_WRITER_IMAGE` to run the Log4j2 writer instead. The pods
then run that image as they did before the stock workload existed: its
entrypoint runs the writer, its own `ledger.sh` and `appender.sh` run from
`/app`, and no ConfigMap is created.

The image carries the writer, the ledger, and the appender, so it has to be
rebuilt when any of them changes. Build and push the fixture to a registry that
the AKS node can pull from, and use an immutable tag or digest:

```sh
docker build \
  --platform linux/amd64 \
  -t <approved-registry>/azure-files-log-writer:<immutable-tag> \
  test/new-e2e/tests/agent-log-pipelines/azure-files/workload
docker push <approved-registry>/azure-files-log-writer:<immutable-tag>
```

Then add it to any of the commands below:

```sh
AZURE_FILES_E2E_WRITER_IMAGE=<approved-registry>/azure-files-log-writer@sha256:<digest>
```

## Run locally against agent-sandbox

Run on your host, not in `dda env dev`, after the one-time Azure setup:

```sh
az login
dda inv e2e.setup --team=agent-log-pipelines --with-azure
```

The E2E framework installs the Agent image built by the selected GitLab
pipeline. Do not build an Agent image separately and do not pass
`--agent-image` for the normal workflow. Select a successful pipeline for the
commit under test that has completed its `qa_agent_linux` and `qa_dca` jobs.
The `smb` cell is left out unless `E2E_SMB_AZURE=1` is set; see
[The smb cell](#the-smb-cell) for the Agent build it needs.

```sh
AZURE_FILES_E2E_RUN=1 \
E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run \
  --targets=./tests/agent-log-pipelines/azure-files \
  --pipeline-id=<pipeline-id>
```

The runner reads the commit SHA from that pipeline and selects
`agent-qa:<pipeline-id>-<short-commit-sha>` from the internal registry. If the
checked-out commit already has a successful pipeline, the runner can detect it
automatically and `--pipeline-id` may be omitted. Passing it is recommended
because it records exactly which build was tested.

To test a development Agent image that was built and pushed separately, replace
`--pipeline-id` with `--agent-image=<approved-registry>/agent@sha256:<digest>`.
Only the Agent image changes; the cluster, shares, writer, mount options,
fingerprint configuration and assertions stay the same.

`E2E_OUTPUT_DIR` is the durable root for every timestamped framework session.
Do not point it at `/tmp` for investigation runs.

### Compare the profiles

Run the suite once per profile against the same Agent build, changing only
`AZURE_FILES_E2E_PROFILE`:

```sh
for profile in default unreliable-mount; do
  AZURE_FILES_E2E_RUN=1 \
  AZURE_FILES_E2E_PROFILE=$profile \
  E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
  dda inv new-e2e-tests.run \
    --targets=./tests/agent-log-pipelines/azure-files \
    --pipeline-id=<pipeline-id>
done
```

Every run creates its own stack, so the runs do not share cloud state. To
compare Agent builds instead, keep the profile and change only `--pipeline-id`
(or `--agent-image`). In both cases compare `<cell>-markers.jsonl` in the
evidence directory against the collected logs: the early markers say whether
appends to a rotated file are still delivered, and the 45s markers say whether
the suite is still calibrated.

To run only the cells that differ in mount options, add
`AZURE_FILES_E2E_CELLS=file-line,file-line-actimeo30`.

The suite does not pin the AKS node image. Record `uname -a` from the evidence
before interpreting a result. The original failure was reproduced on Linux
5.15, and the Azure provisioner cannot select a specific AKS node image yet.

## The smb cell

The `smb` cell needs an Agent built with the native SMB log source
(`yoon/smb-tailer`). An Agent without it accepts `type: smb` but never starts
the source, which stays `Pending`. The cell is therefore gated twice:

- `AZURE_FILES_E2E_RUN=1` enables the suite, as for every cell;
- `E2E_SMB_AZURE=1` enables the `smb` cell. Without it, the cell is not
  provisioned at all (no storage account, writer, Secret or Agent source) and
  its subtest is reported as skipped. If it was the only cell selected, the
  whole test skips before creating anything.

Once the Agent is installed, the cell:

1. requires `agent status --json` to list a `type: smb` source for
   `azure-files-smb` with status `OK` within 2 minutes, before it waits for
   the writer's rotations. A build without the source fails with that
   message rather than with every sequence missing, and an authentication or
   connection error fails with the source's status, with the key redacted;
2. once the writer has completed its rotations, applies the same
   exactly-once ledger and marker assertions as the CIFS cells, with its own
   early marker age (see [Post-rename markers](#post-rename-markers));
3. checks that the account key appears in none of `agent status --verbose`,
   `agent status --json --verbose` (which the CLI does not scrub),
   `agent configcheck`, `agent secret`, any entry of a flare built in the pod
   (`agent flare` without `--send`, unzipped by the test), any container log
   of the Agent pod, or any log Fakeintake collected. Failure messages say
   where the key was found and never print it;
4. records the source's status, inputs and `Bytes Read`, and each of its
   tailers (`smb://<host>/<share>/app.log`, with its own `Bytes Read`, FileId,
   and `Draining Since` for a rotated file still being read) from
   `agent status --json --verbose`; whether `agent secret` lists the password
   handle as resolved; and the logs-agent `BytesMissed` expvar from the flare.
   They go to the test log and to `smb-<agent-pod>-source.json`.
   `BytesMissed` is Agent-wide, so it also counts the file cells when they run
   in the same run. A rotated file whose tail is appended after the drain
   ended is invisible to the Agent, so the lost 45s markers do not show up in
   it.

### Use an Agent built from the feature branch

The suite installs whatever Agent image the run resolves; the test code can
stay on this branch while the Agent comes from another one.

**From a GitLab pipeline (recommended).** Push the branch that carries the
SMB source and wait for the `qa_agent_linux` and `qa_dca` jobs of its
pipeline:

```sh
git push origin yoon/smb-tailer
```

Then pass that pipeline to the run with `--pipeline-id=<pipeline-id>`. The
runner reads the pipeline's commit and the Helm installation pulls
`agent-qa:<pipeline-id>-<short-sha>-linux` and
`cluster-agent-qa:<pipeline-id>-<short-sha>` from the internal registry; it
fails early if either is missing. Once the feature and this suite are on one
branch, the pipeline of that branch serves both.

**From an image you built.** `--agent-image=<registry>/<repository>:<tag>`
sets `ddagent:fullImagePath`, which takes precedence over the pipeline image
for the node Agent. The cluster Agent still comes from the pipeline (passed or
auto-detected for your checkout) or the default release image. The AKS node
must be able to pull the image: for images in the agent-sandbox ECR
(`376334461865.dkr.ecr.us-east-1.amazonaws.com`) the runner logs in through
`aws-vault exec sso-agent-sandbox-account-admin-8h` and configures the pull
secret itself. One way to build such an image from a `yoon/smb-tailer`
checkout is `dda inv agent.hacky-dev-image-build --target-image=<image>
--push`. It compiles the Agent for the machine it runs on, and the AKS node is
`Standard_D4s_v5` (amd64), so run it on an amd64 Linux machine; on Apple
silicon use the pipeline instead.

### Run only the smb cell

```sh
AZURE_FILES_E2E_RUN=1 \
E2E_SMB_AZURE=1 \
AZURE_FILES_E2E_CELLS=smb \
E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run \
  --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' \
  --pipeline-id=<smb-tailer-pipeline-id>
```

Add `AZURE_FILES_E2E_CELLS=file-line,smb` (or leave it unset) to compare the
SMB source with the CIFS cells in the same run.

### Keep the stack up and iterate

By default every run creates a stack named after its run ID, and a passing run
destroys it. To iterate on one stack, give it a stable name and keep it:

```sh
AZURE_FILES_E2E_RUN=1 \
E2E_SMB_AZURE=1 \
AZURE_FILES_E2E_CELLS=smb \
AZURE_FILES_E2E_STACK=azure-files-smb-dev \
E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run \
  --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' \
  --keep-stack \
  --pipeline-id=<smb-tailer-pipeline-id>
```

`--keep-stack` sets `E2E_DEV_MODE=true`, so the stack stays up after a pass;
a failing run keeps it in any case. Run the same command again, for example
with the `--pipeline-id` of a newer feature build, to reuse the AKS cluster,
the Fakeintake VM and the storage accounts:

- the storage accounts are named after the stack, so they are kept;
- the shares are named after the run, so every run starts from an empty share
  and ledger. The SMB source keys its registry entries by host, share and
  path, so an offset from an earlier run cannot match; the file cells reuse
  their paths and rely on their fingerprints instead;
- the first pass removes the previous run's Agent and writers, and the test
  flushes Fakeintake before installing the new Agent.

`AZURE_FILES_E2E_STACK` takes 1 to 40 lowercase letters, digits or inner
hyphens. The framework prefixes it with your user name.

### Destroy the stack

```sh
dda inv new-e2e-tests.clean -s
az resource list -g dd-agent-sandbox --tag username=$USER -o table
```

`clean -s` lists your local stacks and destroys the ones you select; pick
`<user>-azure-files-smb-dev` (or the `azure-files-<hash>` stack a run
printed). A final run of the iteration command without `--keep-stack`
destroys the stack too, once it passes.

## Checks that do not create cloud resources

The suite's Bazel test target is tagged `manual`, so it does not run under
`//...`. Name it explicitly to run the unit tests; the E2E test itself skips:

```sh
bazel test //test/new-e2e/tests/agent-log-pipelines/azure-files:azure-files_test

dda inv linter.go \
  --module=test/new-e2e \
  --targets=./tests/agent-log-pipelines/azure-files

shellcheck -s sh workload/appender.sh workload/ledger.sh
ruff check workload/logwriter.py && ruff format --check workload/logwriter.py
```

The unit tests include the stock workload's self-tests in `workload_test.go`,
which need `python3` (3.8 or later) and skip without it:

- the writer runs on a simulated clock from 12:00:10, with the configuration
  of a writer pod, until it has rotated four files. Every line of every
  rotated file must be a record of the Java writer's format, each file must be
  named after its minute and end at its target size in padding, sequences
  must run on across files, and the first record must match the Java writer's
  byte for byte;
- a second run starts four seconds before a minute, which the Java writer
  skips;
- `logwriter.py crc64` must print what Go's `hash/crc64` computes;
- `ledger.sh` must record the rotated files exactly as the suite decodes and
  asserts on them. It also needs `sha256sum`, which macOS keeps in `/sbin`,
  outside the PATH Bazel gives tests; add
  `--test_env=PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin` to run it;
- `appender.sh`, with its ages scaled down to 50ms and 100ms, must journal and
  append both markers to each rotation the writer makes.

You can also run the writer on its own:

```sh
LOGWRITER_LOG_DIR=$(mktemp -d) LOGWRITER_RUN_ID=local \
  python3 workload/logwriter.py selftest --start 2026-08-13T12:00:10Z --rotations 4
```

The E2E test skips before provisioning unless `AZURE_FILES_E2E_RUN=1` is set.
The `smb` cell also needs `E2E_SMB_AZURE=1`.
