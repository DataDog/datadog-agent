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

The stock Azure provisioner creates one AKS node and a VM-hosted Fakeintake;
the [multi-node](#multi-node) scenario asks it for more nodes, and the
[smb-windows](#a-windows-file-server-smb-windows) cell adds a Windows Server VM
on the same subnet. The suite provisions in two passes. The first pass creates the Azure Files shares
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

By default the writers rotate each minute by rename and vary completed file
sizes around 81-85 KB; [Writer options](#writer-options) change the rotation
mode, the period, the rate and the number of files for every cell of a run.
After four rotations of every writer stream, the test checks the first three
completed files of each stream against Fakeintake: every record the ledger
lists must arrive exactly once, except that a record its rotation put at risk
may be missing, and every log of the cell's service must carry `source:java`
and the `e2e_run_id` and `e2e_cell` tags of its log source.

`AZURE_FILES_E2E_CELLS` restricts a run to named cells, so a single cell can be
provisioned instead of the whole matrix. An unknown name fails before any cloud
resource is created.

The four cells above are the default matrix: what runs when
`AZURE_FILES_E2E_CELLS` is unset. The suite has more SMB cells, which only
run when `AZURE_FILES_E2E_CELLS` names them, and only with `E2E_SMB_AZURE=1`
like the `smb` cell; see [The opt-in SMB cells](#the-opt-in-smb-cells). The
[scenarios](#scenarios) disrupt the Agent while the `smb` cell runs.

| Cell | What it adds to `smb` |
|---|---|
| `smb-copytruncate`, `smb-delete-recreate`, `smb-gzip` | Always rotates by that mode, whatever the run's rotation mode |
| `smb-late-1000`, `smb-late-2000`, `smb-late-3000` | Probes the drain window with one early marker at that age |
| `smb-glob-load` | 8 services under one `*/app.log` source, 10s periods, 2 MB/s in all |
| `smb-windows` | The share is on a Windows Server VM that requires SMB signing, not on Azure Files; also needs `AZURE_FILES_E2E_WINDOWS=1` |

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
identity, so the test can tell exactly which append is missing. Each rotation
gets its own background job, so markers keep their ages when rotations come
faster than 45s apart or in several stream directories at once.

The markers need a renamed file that stays where it was renamed to, so they
depend on the [rotation mode](#writer-options):

| Mode | Markers |
|---|---|
| `rename` | early and 45s, as described here |
| `gzip` | early only. The rotated file is compressed and deleted 5s after the rename, so a 45s marker would find it gone |
| `copytruncate`, `delete-recreate` | none: no renamed file is ever read (the copy is not a path the sources match) or there is none. The appender idles |

The appender never appends to a rotated file that is gone: it journals that
marker as `skipped` instead of creating the file again.

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
against `smb-markers.jsonl` before relying on its calibration, and see
[Late appends](#late-appends-smb-late-1000-smb-late-2000-smb-late-3000) for
the cells that probe where it ends, one age each.

## Credentials

Each cell's storage account key lives only in the Pulumi state (as a secret)
and in Kubernetes Secrets. None of it is a stack output, a Helm value or a
conf.d value.

- `azure-files-storage-<cell>` in the `azure-files-e2e` namespace holds the
  account name and key1. The CSI driver reads it for the writer's CIFS mount.
- The storage pass copies it into a Secret of the same name in the Agent
  namespace (`datadog`). Both the Agent's CIFS mounts and the `smb` cell's
  Secret volume need that copy:
  - the CSI driver resolves an inline volume's Secret in the pod's own
    namespace and ignores `secretNamespace`;
  - a Secret volume can only reference a Secret of its pod's namespace.

  The copy holds key1 too, except in the
  [key-rotation](#key-rotation) scenario, where the `smb` cell's copy holds
  key2. The pass runs before the Agent is installed, so the copies exist when
  the Agent pod starts. The suite waits for the Agent to be ready before it
  checks any cell, and when it isn't ready, the failure gives the pod's
  waiting containers and Warning events.

The `smb-windows` cell has no storage account. Its Secrets, under the same
names and keys, hold the name and password of the Windows file server's local
reader account: the storage pass generates the password as a Pulumi secret
(the `random` provider's `RandomPassword`, through its generic resource token)
and never exports it. See
[A Windows file server](#a-windows-file-server-smb-windows) for how it
reaches the VM.

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

- writer ledgers, writer journals (`<cell>-periods.jsonl`), marker journals
  and pod manifests, each with the files of every stream one after the
  other;
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
  image and the checksum of its ConfigMap scripts, or the Java writer image,
  and the writer options with every timing they imply), profile, whether
  markers are calibrated, the scenario, cells (with each one's writer
  options, markers and whether its losses are accounted, and the SMB
  source's host, share, user, path pattern, password handle and poll
  interval), storage accounts, shares, and the gated cells that were skipped;
- `<cell>-losses.json` for `smb-delete-recreate` and `smb-gzip`: each file
  that lost records, the Agent's missed-bytes reports for the cell and which
  file each was attributed to;
- `<cell>-marker-outcomes.json` with `AZURE_FILES_E2E_CALIBRATE=1`;
- `<cell>-load.json` for every paced cell: throughput, lags and the
  estimated SMB opens per second;
- for a scenario, `<scenario>-status-timeline.json` and its timings
  (`agent-restart-restart.json`, `key-rotation-rotation.json`,
  `network-drop-drop.json`), and for `agent-restart` the stopped pod's log;
  for `multi-node`, `multi-node-agents.json`, each Agent's hostname and pod;
- for `smb-windows`, what the VM holds instead of a writer pod: the ledger,
  marker journal and writer journal, the console log of each scheduled task
  (`smb-windows-writer.log`, `-ledger.log`, `-appender.log`), the server's
  state (`smb-windows-windows-server.txt`: the OS, the SMB server's signing
  and encryption settings, the share and its access, the sessions and open
  files, the tasks and the share's files) and `smb-windows-sessions.json`.

The test prints the evidence directory and the run's resource names at the end.

To destroy a kept stack, run `dda inv new-e2e-tests.clean -s` and select the
stack the test printed (`azure-files-<hash>`, or the `AZURE_FILES_E2E_STACK`
name). Then check that nothing is left:

```sh
az resource list -g dd-agent-sandbox --tag username=$USER -o table
```

## The writer workload

Each cell's writer pod runs three containers on the cell's share: the
`writer`, which writes sequence-numbered records and rotates `app.log` (every
minute by rename unless [Writer options](#writer-options) say otherwise); the
`ledger`, which records every completed file in `ledger.jsonl`; and the
`appender` (see [Post-rename markers](#post-rename-markers)).

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

`logwriter.py` is a port of the Java writer in `workload/java`. It reads the
same `LOGWRITER_*` variables, and with the default writer options it keeps
everything the assertions read from the share:

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
- ledger: `ledger.sh` is the same script, with two sources. The Java image
  runs its default, `scan`: each rotated file is recorded from its content,
  with the image's `Crc64` class. The stock image sets
  `LOGWRITER_LEDGER_SOURCE=journal`: each file is recorded from the Python
  writer's journal (below), whose checksums `logwriter.py` computes; it also
  prints them as `logwriter.py crc64` (`LOGWRITER_CRC64_COMMAND`). For the
  default rename mode, both sources write the same ledger fields.

`workload_test.go` runs `logwriter.py` on a simulated clock, then `ledger.sh`
and `appender.sh` on its files, and checks them against that contract (see
[Checks that do not create cloud resources](#checks-that-do-not-create-cloud-resources)).
A change to what either writer writes with the default options has to be made
in the other one too.

### The writer journal and the ledger

Some rotation modes compress or delete a file before the ledger could read
it, and copytruncate destroys records on purpose. So the Python writer
journals each completed file itself: before it renames, copies, truncates,
compresses or deletes anything, it appends one JSON line for the file to
`periods.jsonl` next to `app.log`. `ledger.sh` copies every journal line into
`ledger.jsonl` once, adding what the share holds for that file when it looks:
`observed` is `file`, `archive` (only the `.gz` is left), `deleted` (by
design) or `missing`, with `observed_bytes`.

The journal has the Java ledger's fields, computed from what the writer
wrote, and these:

| Field | Meaning |
|---|---|
| `first_sequence`, `last_sequence`, `bytes`, `line_count` | The records and bytes the writer wrote while the file was active; for copytruncate, while its period was current |
| `unwritten_sequences` | `[first, last]` ranges the writer failed to write, for example while an open handle elsewhere keeps a deleted `app.log` from being created again. They are in no file, so the test does not expect them, and it logs how many there were |
| `at_risk_sequences` | copytruncate only: what the writer appended between the start of the copy and the truncation. A reader of `app.log` can lose these without being at fault |
| `disposition`, `archive` | What the writer did with the file: `renamed`, `copied`, `compressed` (into `archive`) or `deleted`; `missing`, `empty` or `copy-failed` when no rotation happened |
| `stream`, `rotation_mode`, `rotated_at` | The stream directory (empty for the share root), the mode and the time of the rotation |

From the first three entries of each stream, the test expects every sequence
except the unwritten ones. A record in an at-risk range of its stream may be
missing, but never duplicated; every other record must arrive exactly once.
The number of at-risk records that were lost is logged, not failed.

### Writer options

These apply to every cell of a run, like the profile, and need the stock
Python writer:

| Variable | Default | Values |
|---|---|---|
| `AZURE_FILES_E2E_ROTATION_MODE` | `rename` | `rename`, `copytruncate`, `delete-recreate`, `gzip` |
| `AZURE_FILES_E2E_PERIOD_MS` | `60000` | 5000 to 600000, in whole seconds |
| `AZURE_FILES_E2E_RATE_BYTES_PER_SEC` | `0` | 0 to 20000000: the writer pod's total, shared by its streams |
| `AZURE_FILES_E2E_STREAMS` | `0` | 0 to 32 |

Some opt-in cells have options of their own: the
[rotation mode cells](#rotation-modes-smb-copytruncate-smb-delete-recreate-smb-gzip)
and the [late-append cells](#late-appends-smb-late-1000-smb-late-2000-smb-late-3000)
always rotate by their mode, and refuse a run that sets another, and
[smb-glob-load](#many-services-under-one-glob-smb-glob-load) has its own
defaults for the options a run leaves unset. A [scenario](#scenarios) sets
the run's default period and rate the same way.

Other run options:

| Variable | Values |
|---|---|
| `AZURE_FILES_E2E_CALIBRATE` | `1` records each marker's outcome instead of asserting it |
| `AZURE_FILES_E2E_SCENARIO` | `agent-restart`, `key-rotation`, `network-drop`, `multi-node` |
| `AZURE_FILES_E2E_NETWORK_DROP_SECONDS` | 31 to 600, `network-drop` only; 90 by default |
| `AZURE_FILES_E2E_NODES` | 1 to 5 AKS nodes; above 1 only with `multi-node`, which defaults to 2 |
| `AZURE_FILES_E2E_WINDOWS` | `1` lets the `smb-windows` cell run, with `E2E_SMB_AZURE=1`, when `AZURE_FILES_E2E_CELLS` names it |

The rotation modes (the timings are constants in `provisioner.go`):

| Mode | At each period boundary | What a reader may lose without failing the cell |
|---|---|---|
| `rename` | The first record of the new period closes `app.log`, renames it to `app.log.<suffix>` and creates a new `app.log` (Log4j2's `RollingFile`) | Nothing |
| `copytruncate` | A rotator copies `app.log` to `app.log.<suffix>` while the writer keeps appending through its one `O_APPEND` descriptor, waits 3s (`copyTruncateHoldMs`), then truncates `app.log` in place. The writer's next append lands at offset 0 | The at-risk records, written between the start of the copy and the truncation. A record written before the copy stayed in `app.log` for at least 3s, which is more than two file scans or SMB polls |
| `delete-recreate` | The first record of the new period closes `app.log`, waits 1s (`deleteRecreatePauseMs`), deletes it and creates a new `app.log` | Nothing. A record the reader had not read when the file was deleted counts as lost |
| `gzip` | Like `rename`; 5s later (`gzipDelayMs`) the rotated file is compressed to `app.log.<suffix>.gz` and deleted | Nothing |

The copy and the compression run in 1 MiB steps between the writer's own
writes, so a paced writer keeps its rate during them. A copy therefore never
holds a torn line, which logrotate's copytruncate can produce.

The period: periods start at multiples of it since the epoch. A period that is
not a whole number of minutes names its records and files to the second
(`period=20260813T120010Z`, `app.log.13082026_120010`). The head pause and the
first-period runway shrink to a sixth of a shorter period, 1666ms at 10s. The
suite waits up to `max(6 minutes, 6 periods)` for four rotations.

The rate: `0` keeps the Java writer's schedule, which fills each period to its
target size right after the head pause, one write per record. Above `0`,
each stream writes its share of the rate from the end of the head pause to
the end of the period, so a period carries `rate * (period - head pause)`:
5 MB/s with 10s periods averages 4.2 MB/s. A paced writer also:

- batches records into writes of up to 64 KiB and gives them 1 KiB payloads
  (1,253-byte records), so 5 MB/s takes about 4,000 records and, with ten
  streams, 80 writes a second;
- does not echo records to its container log;
- keeps at least 6 rotated files and at least the last 2 minutes of them,
  and deletes older ones, so a kept stack stays within the 5 GiB share quota.
  The suite refuses a rate and period whose files would not fit in 80% of it:
  5 MB/s fits periods up to 107s;
- skips a backlog of more than 5s instead of writing it in one burst, for
  example after the share stalled.

Ten streams at 5,000,000 bytes/s on a laptop disk wrote 4.85 MB/s during
their fills in the rename, gzip and copytruncate modes; the tick cut off by
each boundary accounts for the rest. Azure Files latency has not been
measured with it yet.

Streams: `0` writes one `app.log` at the root of the share. `N` writes
`svc-1/app.log` to `svc-<N>/app.log`, one thread each, and each stream has its
own run ID (`<run ID>-<cell>-svc-<i>`), sequences, journal, ledger and marker
journal. One log source per cell still reads them all: a file cell matches
`/mnt/azure-files/<cell>/*/app.log` and excludes `*/app.log.*`, and the `smb`
cell matches `path: "*/app.log"`.

Mind the costs:

- the appender lists each stream directory 5 times a second and the ledger
  once a second, and Azure bills every listing as a transaction. That is why
  streams stop at 32;
- Fakeintake returns every log of a cell's service on each check. At 5 MB/s,
  the three asserted periods are about 100,000 records with 10s periods, and
  about 660,000 with 60s ones. Use short periods for high rates.

For example, ten services rotating by copytruncate every 10s at 5 MB/s,
read by one SMB source:

```sh
AZURE_FILES_E2E_RUN=1 \
E2E_SMB_AZURE=1 \
AZURE_FILES_E2E_CELLS=smb \
AZURE_FILES_E2E_ROTATION_MODE=copytruncate \
AZURE_FILES_E2E_PERIOD_MS=10000 \
AZURE_FILES_E2E_RATE_BYTES_PER_SEC=5000000 \
AZURE_FILES_E2E_STREAMS=10 \
E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run \
  --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' \
  --pipeline-id=<smb-tailer-pipeline-id>
```

### Use the Java writer image

Set `AZURE_FILES_E2E_WRITER_IMAGE` to run the Log4j2 writer instead. The pods
then run that image as they did before the stock workload existed: its
entrypoint runs the writer, its own `ledger.sh` and `appender.sh` run from
`/app`, and no ConfigMap is created.

The Java writer only has the default writer options: rename every minute,
the target-size schedule and one `app.log`. The suite refuses to run it with
any other [writer option](#writer-options), with a scenario, or with an
opt-in cell that has options of its own (the rotation mode cells and
`smb-glob-load`). Its ledger scans the rotated files, as it always did.

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
- `E2E_SMB_AZURE=1` enables the `smb` cell, the
  [opt-in SMB cells](#the-opt-in-smb-cells) and the [scenarios](#scenarios).
  Without it, an SMB cell is not provisioned at all (no storage account,
  writer, Secret or Agent source) and its subtest is reported as skipped. If
  only SMB cells were selected, the whole test skips before creating
  anything.

The [smb-windows](#a-windows-file-server-smb-windows) cell has a third gate,
`AZURE_FILES_E2E_WINDOWS=1`, for its Windows VM.

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

A stack keeps its node count. A run with more than one node
([multi-node](#multi-node)) therefore never reuses the named stack: it runs on
a stack of its own, the name followed by `-n<nodes>`
(`AZURE_FILES_E2E_STACK=azure-files-smb-dev` with two nodes runs on
`azure-files-smb-dev-n2`), with its own cluster, Fakeintake and storage
accounts. A one-node run on the same name is unaffected, and a name that
already ends in `-n<digits>` is refused. Destroy the multi-node stack on its
own when done.

The Windows VM of [smb-windows](#a-windows-file-server-smb-windows) is part of
the stack while runs select that cell: the first such run creates it (about 10
more minutes), later ones reuse it, Python included, and a run without the
cell removes it. Creating it is estimated at about 10 more minutes; that was
not measured.

### Destroy the stack

```sh
dda inv new-e2e-tests.clean -s
az resource list -g dd-agent-sandbox --tag username=$USER -o table
```

`clean -s` lists your local stacks and destroys the ones you select; pick
`<user>-azure-files-smb-dev` (or the `azure-files-<hash>` stack a run
printed). A final run of the iteration command without `--keep-stack`
destroys the stack too, once it passes.

## The opt-in SMB cells

Each of these cells is an SMB source on a share, writer and storage account of
its own, like `smb`; `smb-windows` has a Windows VM's share instead of a
storage account. They need `E2E_SMB_AZURE=1` and run only when
`AZURE_FILES_E2E_CELLS` names them; several can share one run, next to the
default cells or not. Every one of them gets `smb`'s checks: the source must
be `OK` before the writer is waited for, the key must not leak, and the
source's status and tailers go to the evidence.

The durations below are estimates for one run on a kept stack, provisioning
included; none was measured on Azure yet. Each cell adds a Standard_LRS
storage account, which the stack keeps, and a share per run. Its cost is the
transactions: every SMB open, directory listing, read and write is billed
(see [Costs](#costs)).

### Rotation modes: smb-copytruncate, smb-delete-recreate, smb-gzip

**What they prove.** That the SMB source follows each way a log is rotated
in practice, not only Log4j2's rename: the copy and in-place truncation of
logrotate's `copytruncate`, a writer that deletes its file and creates a new
one, and a rotated file that is compressed and deleted 5s later. Each cell
always rotates by its mode (see the modes in [Writer options](#writer-options)),
so one run covers the three; a run that sets another
`AZURE_FILES_E2E_ROTATION_MODE` is refused. The run's period, rate and streams
still apply: with a rate, the active file is still being written up to the
rotation, so the drain has something left to read when the file goes away.

**Pass rule.** Every record of the first three completed files of each
stream is collected exactly once, except:

- `smb-copytruncate`: the records the writer journalled as at risk, appended
  between the start of the copy and the truncation, may be missing (never
  duplicated). The source reports what it knew of and lost to the
  truncation, but a record appended after its last look is lost without a
  trace; that is copytruncate's own flaw.
- `smb-delete-recreate`, `smb-gzip`: a file may lose its last records when
  the Agent reported them missed. The test reads the Agent's missed-bytes
  warnings (below) and requires, per file: only its last records are
  missing; the Agent reported missed bytes for that file; and the bytes it
  reported are the bytes of the file that no collected record holds, within
  4 KiB (the padding, a record whose start was read, a marker). A missing
  record with no report is a silent loss and fails the cell, as does a gap
  before collected records.

With the default options, the writer fills each period within seconds of its
start, so the drain is long done when the file goes away and nothing is
expected to be missing; add a rate, for example
`AZURE_FILES_E2E_RATE_BYTES_PER_SEC=200000` with `AZURE_FILES_E2E_PERIOD_MS=10000`,
to make the loss accounting do work.

**How the test reads missed bytes.** `RecordMissedBytes` in
`pkg/logs/tailers/smb/tailer.go` adds a lost file's unread bytes to:

| Where | Per cell? |
|---|---|
| the logs-agent `BytesMissed` expvar (in a flare) and the `logs.bytes_missed` telemetry counter | No: Agent-wide, untagged |
| the per-(source, service) missed bytes tracker | Only through the health platform's `log_data_lost_after_rotation` issue, every 15 minutes, when the health platform is enabled |
| `agent status` | Not shown, per source or at all |
| a warning in the Agent log: `<reason>: <N> bytes of SMB file smb://<host>/<share>/<path> (last read as <rotated path>) were not read and are lost` | Yes: the host names the cell's storage account |

So the test parses the warnings of the core Agent container's log (with the
kubelet's timestamps) and attributes them to the ledger's files: by the
rotated name for gzip, and, for delete-recreate, whose deleted file is
reported under `app.log`, by time, between the file's rotation and the next.
A gzip file already gone when the source saw its rotation is reported under
`app.log` too, and attributed by time the same way.
Then, when every cell of the run is an SMB cell, it requires the Agent-wide
`BytesMissed` from a flare to be the sum of the warnings, read just before
and just after the flare: a warning the test could not read, for example
because the kubelet rotated the container log, fails the cell rather than
lets a loss pass as reported. With file cells in the run, which count missed
bytes too, that check is skipped and logged. The losses, the reports and the
attribution go to `<cell>-losses.json`.

A per-source missed bytes counter in `agent status` would replace the log
parsing; see [Product notes](#product-notes).

**Run alone.**

```sh
AZURE_FILES_E2E_RUN=1 E2E_SMB_AZURE=1 \
AZURE_FILES_E2E_CELLS=smb-copytruncate,smb-delete-recreate,smb-gzip \
AZURE_FILES_E2E_STACK=<kept-stack> E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' --keep-stack --pipeline-id=<smb-tailer-pipeline-id>
```

About 9 to 11 minutes with 60s periods (four rotations, plus a flare per
loss-accounted cell), and 5 to 7 with `AZURE_FILES_E2E_PERIOD_MS=10000`. The
three cells run in parallel, so one or three take about as long. Each is an
`smb` cell's cost.

### Late appends: smb-late-1000, smb-late-2000, smb-late-3000

**What they prove.** Where the SMB source's drain window ends. Each cell
appends one early marker to each rotated file at its age (1s, 2s, 3s after
the rename) and the 45s marker, which must still be lost. A collected marker
restarts the drain's idle polls and keeps it open, so a later marker in the
same cell would measure that rather than the window: each age has its own
cell. They always rotate by rename.

**Pass rule.** Records as for `smb`. Without calibration, each early marker
must have the outcome `lateMarkerProbes` in `provisioner.go` gives its age:

| Cell | Expected | Why (from `pkg/logs/launchers/smb/scanner.go`) |
|---|---|---|
| `smb-late-1000` | either, never twice | The drain ends at its second idle poll, between 1s and 2s after the rename depending on where the rename falls between two polls. A 1s marker lands before it unless the scan came within the appender's ~0.2s detection lag of the rename: about 9 rotations in 10 |
| `smb-late-2000` | lost | The drain has ended by 2s plus the scan's few milliseconds |
| `smb-late-3000` | lost | Same |

These are predictions from the code, marked TODO in `provisioner.go`, until
calibration runs confirm them. `AZURE_FILES_E2E_CALIBRATE=1` records every
marker's outcome instead of asserting it (records are still asserted),
prints one line per cell, for example
`smb-late-1000 calibration ...: 1000ms: collected 3/3, lost 0/3 (expected either); 45000ms: collected 0/3, lost 3/3 (expected lost)`,
and writes `<cell>-marker-outcomes.json`. It applies to every cell of the
run that has markers. Run the three cells a few times with it, then update
the table.

```sh
AZURE_FILES_E2E_RUN=1 E2E_SMB_AZURE=1 AZURE_FILES_E2E_CALIBRATE=1 \
AZURE_FILES_E2E_CELLS=smb-late-1000,smb-late-2000,smb-late-3000 \
AZURE_FILES_E2E_STACK=<kept-stack> E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' --keep-stack --pipeline-id=<smb-tailer-pipeline-id>
```

About 9 to 10 minutes: four rotations of 60s, the 45s markers of the last
asserted rotation, and the 30s settle. Each is an `smb` cell's cost.

### Many services under one glob: smb-glob-load

**What it proves.** That one SMB source with `path: "*/app.log"` keeps up with
8 services (`svc-1/app.log` to `svc-8/app.log`) rotating every 10s at 2 MB/s
in all, about 1,600 records a second, and drains each rotated file exactly
once. `AZURE_FILES_E2E_STREAMS`, `_PERIOD_MS` and `_RATE_BYTES_PER_SEC` change
the cell's defaults when set.

**Pass rule.** Every record of the first three files of every service is
collected exactly once, and every early marker once, the 45s markers lost.

**Evidence.** `<cell>-load.json`, and one line in the test log, for every
paced cell:

- throughput: what the writer wrote in its steady periods, and what
  Fakeintake received of the asserted records over their arrival window;
- pipeline lag: from each record's write (the writer's timestamp in the
  line) to its arrival at Fakeintake, split into the Agent's read lag (to the
  log's timestamp, set when the Agent read it) and the delivery lag; and
  each file's completion lag, from its rotation to its last record's arrival.
  The writer and the Agent share the node's clock; Fakeintake runs on its own
  VM, so the arrival-based lags include the clock offset between the two;
- SMB opens per second, estimated from the poll logic, not measured: per 1s
  scan, one listing of the share root and of each service's directory, and
  one open per 64 KiB new in each file; per rotation, about 5 opens of its
  drain. At the defaults that is about 45 opens a second (9 listings, 32
  reads, 4 for the drains). Azure Monitor's `Transactions` metric of the
  storage account measures them.

**Run alone.**

```sh
AZURE_FILES_E2E_RUN=1 E2E_SMB_AZURE=1 AZURE_FILES_E2E_CELLS=smb-glob-load \
AZURE_FILES_E2E_STACK=<kept-stack> E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' --keep-stack --pipeline-id=<smb-tailer-pipeline-id>
```

About 6 to 10 minutes, most of it Fakeintake returning the cell's logs on
each 10s check: about 40,000 asserted records, and everything written since
the Agent started. The costliest cell: about 45 SMB opens a second from the
Agent, plus the appender's and the ledger's listings of 8 directories (5 and 1
a second each), and 2 MB/s of writes. The writer keeps writing after the test
on a kept stack, until the next run's first pass removes it.

### A Windows file server: smb-windows

**What it proves.** That the SMB source reads a Windows Server share, not
only Azure Files: NTLMv2 with a local account of the server, a server that
refuses any session that is not signed, NTFS file IDs across the rename, and
Windows' share-mode checks between the source's opens and a writer that
renames and appends on the server itself.

**How it is set up.** The cell has no storage account; `windows.go` and
`windows_test.go` hold what follows.

- The storage pass creates a Windows Server VM (the framework's default
  Windows Server image, Windows Server 2025 Datacenter Azure Edition Core, and
  VM size, `Standard_D4s_v5`) in the same Azure environment as the AKS
  cluster, so it lands on the subnet of the AKS nodes and the Fakeintake VM,
  with a private address only. Windows Defender is disabled, as the
  framework's Windows provisioners do by default. The pass also generates the
  password of the reader account into the cell's Secrets (see
  [Credentials](#credentials)). The suite's environment is the AKS one plus a
  remote host for the VM: the framework's `azurekubernetes.AKSRunWithEnv` runs
  the AKS provisioner in an Azure environment the suite also creates the VM
  in.
- Before the Agent pass, the test copies `workload/fileserver.ps1`,
  `workload/logwriter.py`, `workload/sidecars.py` and one cmd.exe wrapper per
  scheduled task to `C:\azure-files-e2e` over SFTP, and the password to a
  file there. It then runs `fileserver.ps1 prepare` over SSH, which stops the
  previous run's writer and removes its share; requires signing on the SMB
  server (`Set-SmbServerConfiguration -RequireSecuritySignature $true`); opens
  TCP 445 in Windows Firewall; creates or resets the local user `ddlogreader`
  with the password, and deletes the file at once; creates the share
  `smbwin-<run>` on `C:\azure-files-e2e\shares\smbwin-<run>`, which only
  `ddlogreader` may read; installs Python once per VM; and registers the
  writer, the ledger and the appender as scheduled tasks that run as SYSTEM.
  No command line, log or output carries the password.
- The Agent pass gives the cell's `type: smb` source the VM's private address
  as `host`, `ddlogreader` as `username`, and the password through a mounted
  Secret and an `ENC[file@...]` handle, as for the Azure cells.
- Once the Agent is ready, the test starts the tasks. They do what a writer
  pod's containers do, on the share's directory, and the test reads their
  files with `fileserver.ps1 read`, which shares them with the processes that
  are still writing them.

**Why the stock Python writer rather than a PowerShell port.** The VM runs
`logwriter.py`, the pods' writer, on python.org's embeddable Python 3.12.10,
the last 3.12 release with Windows binaries, downloaded once per VM from
www.python.org; `fileserver.ps1` refuses a `python.exe` that python.org did
not sign (Authenticode, signer "Python Software Foundation"). The cell's
records, rotation and journal are therefore those of every other cell, which
`workload_test.go` checks. A PowerShell port would be a second writer to keep
in step with the first, and nothing could test it before an Azure run:
PowerShell is not installed where the unit tests run. ledger.sh and
appender.sh need a POSIX shell, which Windows does not have, so
`workload/sidecars.py` ports both, and `workload_test.go` holds the shell
scripts and the port to the same tests. What it costs: the VM needs outbound
HTTPS to www.python.org once, and Python 3.12.10 gets no more fixes. The
writer opens its files in binary mode, so it writes the same bytes on Windows.

**Pass rule.** As for `smb`: the source must be `OK` before the writer is
waited for; every record of the first three completed files is collected
exactly once, the 0.5s marker once and the 45s marker is lost; and the
password appears nowhere the Agent prints, logs, packs into a flare or ships.
And the server must require signing and hold an SMB session of
`ddlogreader`: the Agent read the share as that user, over sessions the
server only accepts signed. Each session's dialect is logged.

The cell runs the writer's default options only, rename every minute and one
`app.log`; a run that sets a rotation mode, period, rate or streams is refused.
No scenario runs with it.

**Run alone.**

```sh
AZURE_FILES_E2E_RUN=1 E2E_SMB_AZURE=1 AZURE_FILES_E2E_WINDOWS=1 \
AZURE_FILES_E2E_CELLS=smb-windows \
AZURE_FILES_E2E_STACK=<kept-stack> E2E_OUTPUT_DIR=$HOME/azure-files-e2e-runs \
dda inv new-e2e-tests.run --targets=./tests/agent-log-pipelines/azure-files \
  --run='^TestAzureFiles$' --keep-stack --pipeline-id=<smb-tailer-pipeline-id>
```

Add `smb` to `AZURE_FILES_E2E_CELLS` to compare it with Azure Files in the same
run. `AZURE_FILES_E2E_WINDOWS=1` alone selects nothing: the cell, like every
opt-in cell, runs only when named.

**Duration and cost.** Estimates, none measured: about 9 minutes on a stack
that already has the VM (four one-minute rotations, the 30s settle and the
leak checks), and about 10 more to create the VM and install Python the first
time. The VM, `Standard_D4s_v5` with its Windows Server licence and a 200 GB
StandardSSD OS disk, is billed for as long as the stack keeps it, which costs
more than any share; a run without the cell removes it, and so does destroying
the stack. Its SMB traffic stays inside the virtual network, and no Azure Files
transaction is billed for it.

**Not verified on Azure yet.** The first run will tell:

- that the subnet's network security group lets the AKS nodes reach the VM on
  port 445, as it lets them reach the Fakeintake VM on port 80;
- that the VM reaches www.python.org, as the Fakeintake VM reaches the Docker
  download site, and that `python.exe` passes the signature check there;
- that Windows accepts the source's NTLMv2 login for a local account with no
  `domain` set in the source;
- that `Get-SmbSession` reports each session's dialect.

## Scenarios

A scenario disrupts the Agent while the `smb` cell's writer runs, then checks
the cell as usual, with what the disruption allows; [multi-node](#multi-node)
runs several Agents instead. `AZURE_FILES_E2E_SCENARIO` selects one; its test
method runs and the others skip, and so does
`TestRotatedFilesAreCollectedExactlyOnce`. It needs `E2E_SMB_AZURE=1`, runs
on `smb` when `AZURE_FILES_E2E_CELLS` is unset, and refuses the cells it would
disturb: `agent-restart` and `key-rotation` touch every source of the Agent,
and `multi-node` reads every cell once per Agent, so they run with `smb`
alone; `network-drop` can run with the file cells (they keep their usual
checks) but no other SMB cell.

Each disruption hits the second period of the writer, the first full one,
and has to be over before that period ends: a file that rotates out while no
source can read the share is not read again (see
[Product notes](#product-notes)). So each disruption has its own default
period and a paced writer, so the active file keeps growing through the
disruption; the run's options override them as long as they hold the
disruption, and it fails, naming the option to raise, when the disruption
outlasted the period. The `smb` cell has no markers in a disruption: it
delays the drains past every marker age. Each disruption writes
`<scenario>-*.json` to the evidence, with its timings and a status timeline
of the source, polled every 5s.

| Scenario | Default writer | Disruption | Pass rule |
|---|---|---|---|
| `agent-restart` | 120s periods, 20 KB/s | The Agent pod is deleted 30s into the period | No loss; duplicates within what a restart can resend |
| `key-rotation` | 300s periods, 10 KB/s | key2, which the Agent reads with, is renewed 15s into the period | The source reports an authentication error, then `OK` again; no loss, no duplicate |
| `network-drop` | 180s periods, 10 KB/s | The Agent's SMB traffic is dropped for 90s around the end of the period | The source reports an error, then `OK` again; no loss, no duplicate |
| `multi-node` | The run's (default: rename every minute) | None: 2 AKS nodes, so 2 Agent pods with the `smb` source | Every record collected once by each Agent, by its hostname |

### agent-restart

**What it proves.** That the SMB source resumes each file where its registry
offset says after the Agent restarts, and loses nothing written meanwhile.

The test checks first that the registry outlives the pod: the agent
container's `/opt/datadog-agent/run`, where the registry lives
(`logs_config.run_path`), must be a hostPath volume. The chart the framework
installs (3.245.0) mounts its `pointerdir` volume, the node's
`/var/lib/datadog-agent/logs`, there whenever `datadog.logs.enabled` is set,
which the suite sets; with an `emptyDir` the test fails and says to mount a
hostPath there. It then waits for the source to have read part of the active
file and for the registry to hold its offset, deletes the Agent pod with the
default grace period while following its log, and waits for the DaemonSet's
replacement to be ready and the source `OK`, before the period ends.

**Pass rule.** No record is missing. Duplicates are bounded by what a restart
can resend, read from the auditor: it writes the registry every second
(`defaultFlushPeriod` in `comp/logs/auditor/impl/auditor.go`) with the offset
of the last record the destination acknowledged, and once more when it stops,
after the pipeline has flushed. So:

- when the stopped Agent's log shows `logs-agent stopped` without
  `Timed out when stopping logs-agent`, the stop was graceful and no record
  may be collected twice;
- otherwise, at most the records written in one flush period, one
  `logs_config.batch_wait` (5s) and a second of send latency, plus one
  64 KiB write: about 200 records at 20 KB/s.

Duplicates must also be one run of sequences per file (one resume point),
each collected twice at most. The stopped pod's log goes to
`<old-pod>-agent-shutdown.log`.

**Run alone.** Add `AZURE_FILES_E2E_SCENARIO=agent-restart` to the `smb`
command of [Keep the stack up and iterate](#keep-the-stack-up-and-iterate).
About 14 minutes (four 2-minute rotations). One `smb` cell's cost.

### key-rotation

**What it proves.** That the SMB source survives the rotation of the key it
authenticates with: it reports the failure, and once its Secret holds the new
key, the secret refresh gives it to the source, which resumes without losing
or repeating a record.

In this scenario the storage pass gives the Agent's copy of the `smb`
Secret key2, while the writer still mounts the share with key1, and the
Agent's Helm values enable the secret refresh, through the settings
`pkg/config/setup/config.go` reads: `DD_SECRET_REFRESH_INTERVAL: "15"`
(`secret_refresh_interval`) and `DD_SECRET_REFRESH_SCATTER: "false"`
(`secret_refresh_scatter`), on the core Agent only.

15s into the period, the test renews key2 with the Azure CLI from the test
process (`az storage account keys renew --key secondary --output none`, then
`az storage account keys list --query` for the new key2 alone), on the
subscription and resource group recorded in the
`azure-files-storage-accounts` ConfigMap, which maps each cell to its
storage account's ARM ID. Neither command's output is printed, and both keys
join the keys redacted from the evidence and looked for by the leak checks.
If Azure keeps the session that authenticated with the old key2, nothing
fails until the client authenticates again; when no authentication error
shows within 45s, the test drops the Agent's SMB traffic for 35s, longer than
an operation timeout, with the network helper of `network-drop`, so the
client has to log in again. Once the status shows the authentication error,
the test writes the new key2 into the Agent's Secret, waits for the kubelet to
update the mounted file (comparing digests, never reading the key), and
waits for the source to be `OK`.

**Pass rule.** An authentication error after the renewal, then `OK` before
the period ends, no record missing and none collected twice.
`<scenario>-rotation.json` records whether the reconnection had to be forced.

**Run alone.** Run `az login` first: the test calls `az`. Then add
`AZURE_FILES_E2E_SCENARIO=key-rotation` to the `smb` command. About 25
minutes (four 5-minute rotations). The renewed key2 stays renewed; the next
run's storage pass gives the Agent's Secret its key again from Azure.

### network-drop

**What it proves.** That the SMB source survives losing its connection to
the share for longer than an operation timeout (30s) and the client's
longest backoff (30s), across a rotation: it reports the error, reconnects,
drains the file that rotated while it could not read, and loses nothing.

`AZURE_FILES_E2E_NETWORK_DROP_SECONDS` sets the drop, 90 by default, 31 to
600. The period must hold the drop, the longest backoff after it and a 60s
margin; the default period grows with the drop. The drop is centred on the
end of the second period.

The drop is made by a short-lived helper pod on the Agent's node: privileged,
in the node's PID namespace, running the writers' stock image, which is
pinned by digest and already on the node. It finds a process of the agent
container through the container ID the pod status gives (the lowest PID
whose cgroup names it), and runs the node's own iptables, entering the
node's mount namespace and the Agent pod's network namespace with nsenter,
to add a rule that drops TCP to the storage endpoint's addresses on port 445.
Using the node's iptables avoids pinning a tools image and keeps the
netfilter backend the node uses. The test also checks that:

- the Agent's network namespace had an established connection to port 445
  before the drop, the rule is in that namespace and not in the node's, and
  the rule dropped packets;
- the writer kept writing: its CIFS mount was set up by the CSI node plugin,
  which runs in the node's network namespace, and the kernel keeps the
  mount's connection there, where the rule is not. The writer must have
  rotated during the drop with every record written.

The rule is removed after the drop and the helper deleted in a deferred
cleanup, also when the test fails.

**Pass rule.** An error status during the drop, `OK` again before the next
period ends, no record missing and none collected twice, and a rotation of
the writer during the drop.

**Run alone.** Add `AZURE_FILES_E2E_SCENARIO=network-drop` to the `smb`
command, and `AZURE_FILES_E2E_CELLS=smb,file-line` to watch a file cell next
to it. About 17 minutes (four 3-minute rotations). One `smb` cell's cost.

### multi-node

**What it proves.** What several Agents with the same `smb` source do today:
the SMB source has no single-reader election, so each Agent reads the whole
share, and every record is collected once per Agent. That is the limitation
this scenario documents and holds the source to, so that a change to it shows
up. Once the source elects a single reader, the pass rule becomes exactly once
in all, from any one Agent.

`AZURE_FILES_E2E_NODES` sets the node count, 2 by default and up to 5; the
AKS provisioner's `aks.WithNodeCount` option sizes the system node pool, so
the Agent DaemonSet runs one pod per node. No other run may have more than one
node: its cells would each be collected once per Agent. Writer options apply
as usual, but for a rotation mode other than rename; the markers stay, since
nothing disrupts the Agents.

The test requires one ready Agent pod per node, on distinct nodes, reads each
Agent's hostname with `agent hostname`, and records them in
`multi-node-agents.json`. Fakeintake keeps the hostname of the Agent that sent
each log, which attributes every record to its Agent.

**Pass rule.** For each Agent, every record of the first three completed
files is collected exactly once, its 0.5s markers once and its 45s markers
lost, and an expected record sent by a host that runs none of the Agents
fails the cell. So every record arrives once per Agent: twice with two nodes.
A failure names the cell and the Agent's hostname, as `smb@<hostname>`. The
source must be `OK` on every Agent, and the leak checks run on every Agent
pod.

**The stack.** A stack keeps its node count, so the scenario never runs on a
reused one-node stack: it gets a stack of its own, named after
`AZURE_FILES_E2E_STACK` with `-n<nodes>` (see
[Keep the stack up and iterate](#keep-the-stack-up-and-iterate)).

**Run alone.** Add `AZURE_FILES_E2E_SCENARIO=multi-node` to the `smb`
command. The first run creates the `-n2` stack: estimated at 15 to 20 minutes
for the cluster, the Fakeintake VM and the storage accounts, then about 9 for
the cell; none of this was measured. Cost: a second `Standard_D4s_v5` node,
and a second cluster, Fakeintake VM and storage account for as long as the
`-n2` stack is kept.

## Costs

Azure Files bills every SMB operation as a transaction. Per cell:

- the Agent's SMB source: one listing a second per directory of its pattern,
  one open per 64 KiB of new data, two opens every 10 seconds for a file
  with nothing new, and about 5 per rotation for the drain; see the estimate
  in `<cell>-load.json` for paced cells;
- the appender lists each stream directory 5 times a second, and the ledger
  once a second;
- the writer's writes, and its CIFS client's metadata requests (`actimeo=1`).

A storage account and its empty shares cost next to nothing at rest, so the
kept stack's accounts can stay. The paced cells' and scenarios' writers keep
writing on a kept stack until the next run's first pass removes them.

Two options add compute, billed by the hour while a stack keeps it:

- `smb-windows` adds a `Standard_D4s_v5` Windows Server VM with a 200 GB
  StandardSSD OS disk, which a run without the cell removes; its writer keeps
  writing until then;
- `multi-node` runs on a stack of its own, `<stack>-n<nodes>`, with a cluster
  of that many `Standard_D4s_v5` nodes, a Fakeintake VM and storage accounts
  of its own, until that stack is destroyed.

## Product notes

Gaps the suite works around rather than tests:

- **No single reader for a share.** Every Agent with an `smb` source reads the
  whole share and ships every line, so a DaemonSet with the source on N nodes
  collects every line N times. The [multi-node](#multi-node) scenario asserts
  exactly that; the release note of the SMB source does not say it.
- **Missed bytes per source.** The Agent counts missed bytes Agent-wide and
  per (source, service), but publishes the latter only through a 15-minute
  health platform issue. A `Bytes Missed` counter in each source's status,
  next to `Bytes Read`, would let the loss-accounted cells read their losses
  from `agent status --json` instead of parsing warnings.
- **A file that rotates while no source can read the share is not read
  again.** When the Agent starts, or when a source is replaced after a
  secret refresh, a path whose registry or predecessor offset names another
  FileId than the file now at that path is read from offset 0, and the
  previous file, still listed under its rotated name, is not drained from
  that offset: its unread end is lost, and no missed bytes are reported. The
  scenarios keep their disruptions inside one period because of it. With
  the same source running through a network outage, the first rotation is
  drained once it reconnects, but a file that rotated in and out during the
  outage was never tailed.

## Checks that do not create cloud resources

The suite's Bazel test target is tagged `manual`, so it does not run under
`//...`. Name it explicitly to run the unit tests; the E2E test itself skips:

```sh
bazel test //test/new-e2e/tests/agent-log-pipelines/azure-files:azure-files_test

dda inv linter.go \
  --module=test/new-e2e \
  --targets=./tests/agent-log-pipelines/azure-files

shellcheck -s sh workload/appender.sh workload/ledger.sh
ruff check workload/logwriter.py workload/sidecars.py
ruff format --check workload/logwriter.py workload/sidecars.py
```

The framework packages the suite changed have tests of their own:

```sh
bazel test //test/e2e-framework/scenarios/azure/aks:aks_test
```

Nothing checks `workload/fileserver.ps1` before an Azure run: PowerShell is
not installed where the unit tests run. `config_test.go` only checks the
commands that call it and a few of its lines.

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
  asserts on them, from the files and from the writer's journal alike. It
  also needs `sha256sum`, which macOS keeps in `/sbin`, outside the PATH Bazel
  gives tests; add
  `--test_env=PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin` to run it;
- `appender.sh`, with its ages scaled down to 50ms and 100ms, must journal and
  append both markers to each rotation the writer makes;
- every rotation mode, both on one `app.log` with the Java schedule and on two
  paced streams with 10s periods, must journal each period before its file is
  touched. Each rotated, copied or decompressed file must hold exactly the
  journalled sequences, bytes and head checksums, and every record written
  must be on the share once, unless its file was deleted by design or its
  copytruncate rotation put it at risk, in which case at most once;
- a paced writer must write its share of the rate from the end of the head
  pause to the end of each period, with the paced payload;
- the real writer, with three streams on one-second gzip periods, must rotate,
  compress and journal each stream on its own thread and exit with 143 on
  SIGTERM;
- `ledger.sh` must record the journal of the gzip, delete-recreate and
  copytruncate modes as it is, with what the share holds for each file, per
  stream directory;
- `appender.sh` must mark each stream's rotations in that stream's journal
  with the stream's run ID, never append to a `.gz` file or to a rotated file
  that is gone (it journals those markers as `skipped`), and idle with
  `LOGWRITER_APPEND_DELAYS_MS=none`;
- `sidecars.py`, the Windows file server's port of `ledger.sh` and
  `appender.sh`, must pass each of the ledger and appender tests above with
  the journal source, as a `python` subtest next to the `shell` one;
- the Windows file server's writer, ledger and appender, with the
  environment the test gives their scheduled tasks and the share moved to a
  local directory, must produce a ledger and a marker journal that the
  record and marker checks pass on.

`config_test.go` checks the writer options against the pod spec and the Agent
sources they produce: the validation, the Java writer refusing them, the env
of each container, the stream run IDs, the `*/app.log` sources, the marker
plan of each mode, the retention against the markers, and the at-risk and
unwritten handling of the record check. It also checks the opt-in cells and
the scenarios without Azure:

- the default matrix is still its four cells; every opt-in cell is gated by
  `E2E_SMB_AZURE`, has an account, share and service of its own, and can
  share a run with the others;
- the mode cells keep their mode against the run's, and only gzip and
  delete-recreate account for losses; the late cells probe one age each,
  with predictions consistent with the drain's poll logic; the glob-load
  cell's defaults, its one `*/app.log` source and its opens estimate;
- the marker expectations (collected, lost, either) and what calibration
  records instead;
- the missed-bytes warnings are parsed from the Agent log, attributed to the
  right file, and a loss is only accepted at the end of a file and when
  reported in about its size; the Agent-wide total brackets the warnings;
- the restart's duplicate bound and its checks, the registry hostPath check,
  and the shutdown log reading;
- the scenarios' validation (cells, writer options, drop duration), their
  timings within one period, the secret refresh Helm values of key-rotation
  only, the key index of the Agent's Secret, the Azure CLI arguments, the
  network helper's pod and iptables commands, and the status
  classification;
- the load report's throughput and lags;
- the multi-node scenario: its node count (2 by default, refused above 1 for
  any other run), its stack of its own (`-n<nodes>`, and a reused name that
  already ends so refused), its one `smb` cell with its markers, and the
  rule that each Agent's hostname collects every record and marker once, with
  what fails it: a record short or twice on one Agent, a host that runs no
  Agent, exactly once in all;
- the smb-windows cell: its two gates, its Helm source (the VM's address,
  `ddlogreader`, the mounted password), the writer options and scenarios it
  refuses, the cmd.exe wrapper of each scheduled task (the pod's environment
  on the VM's paths, and values cmd.exe would expand refused), the
  `fileserver.ps1` commands, which carry no secret, and the decoding of the
  server's signing setting and sessions.

You can also run the writer on its own:

```sh
LOGWRITER_LOG_DIR=$(mktemp -d) LOGWRITER_RUN_ID=local \
  python3 workload/logwriter.py selftest --start 2026-08-13T12:00:10Z --rotations 4
```

Any writer option works the same way, for example two paced copytruncate
streams on 10s periods:

```sh
LOGWRITER_LOG_DIR=$(mktemp -d) LOGWRITER_RUN_ID=local LOGWRITER_ROTATION_MODE=copytruncate \
  LOGWRITER_PERIOD_MS=10000 LOGWRITER_HEAD_PAUSE_MS=1666 LOGWRITER_INITIAL_FILL_RUNWAY_MS=1666 \
  LOGWRITER_RATE_BYTES_PER_SEC=40000 LOGWRITER_STREAMS=2 LOGWRITER_CONSOLE_RECORDS=false \
  python3 workload/logwriter.py selftest --start 2026-08-13T12:00:10Z --rotations 4
```

The E2E test skips before provisioning unless `AZURE_FILES_E2E_RUN=1` is set.
The `smb` cell also needs `E2E_SMB_AZURE=1`, and `smb-windows` also
`AZURE_FILES_E2E_WINDOWS=1`.
