# Portable bundle fixtures

`macos/` and `windows/` contain small synthetic capture bundles for automated
tests. Their typed inputs are constructed in
[fixtures_test.go](../../integration/fixtures_test.go) and written directly as typed samples. Replay tests submit their contents
through real Agent serializers, process submission, and event-platform delivery
with an in-memory recording transport. No live collection or staging request
is involved.

These fixtures exercise bundle compatibility, native telemetry fidelity and credential exclusion, portable
replay from one baseline per run, process/software overlays, macOS and Windows
connection evidence, wireless correlation, and delivery accounting. They do not prove live
installed-Agent capture on either platform, direct Windows connection capture,
staging device enrichment, monitor behavior, or Command Center/Bits acceptance.

Both schema-7 fixtures use the deliberate test-only `capture_tool.commit`
`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` and version `7.85.0-fixture`. Their
synthetic producers use commit `bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`,
protocol version 4, and the same fixture version. Producer process identities,
activation/stop acknowledgements, the fully armed recording window, sequences and cycles are constructed
test facts; no service was contacted to obtain them.
Their test identities are provenance, not a compatibility barrier. Use real
operator-managed captures for staging acceptance; these fixtures do not
represent observed devices. Do not edit their manifests to claim the revision
of a real binary.

Each bundle includes:

- `manifest.json`, with the capture-tool build, participating producers,
  acknowledged boundaries and final sequences, profile inventories, explicit
  producer/cycle/chunk identities, relative sample offsets, stream and metric-family cadences,
  schema version and file digests.
- `COMPLETE`, containing the manifest digest.
- Captured typed `sample-*.json` files. Metric envelopes retain Agent source
  identifiers and fractional relative timestamps.


The macOS fixture declares an arm64 profile and the Windows fixture declares an
amd64 profile. Both include metrics, legacy host metadata, Agent inventory, host
inventory, optional host system information, processes, connections, and a software snapshot. Connection evidence
belongs to the synthetic Process Agent on macOS and system-probe on Windows.
Live macOS capture includes connections only when an installed producer advertises
that capability. Both fixtures represent a synthetic 16-GiB
device profile. Fixture facts are chosen for the tests; they are not evidence of
hardware or applications observed on an operator's machine. All inventory
envelopes use the normal `/api/v1/metadata` serializer route. Their synthetic
Agent mode is `end_user_device`, timestamps are relative to capture start, and
the declared Agent/host inventory cadence is ten minutes. Host system information
has an hourly cadence and one sample at five seconds, with a 5.25-second envelope
timestamp, shared synthetic hardware models, and a synthetic native device serial.

Each fixture covers 301 seconds and contains 89 logical samples: 21 metric
cycles, 31 process groups, 31 connection groups, two legacy host-metadata samples,
and one sample each of Agent inventory, host inventory, host system information,
and software. CPU, memory, WLAN,
and network-throughput metrics appear every 15 seconds. Battery metrics appear
only at 0 and 300 seconds, including when they share a serializer flush with the
faster families. Network-throughput metrics retain their native rate type. This
mixed schedule verifies that replay preserves recorded offsets without repeating
cycles or filling every flush with slow battery samples. Longer scenario tests
generate a temporary 65-minute fixture; that expanded recording is not checked
in. Scenarios longer than their bundle are rejected before delivery.

The fixtures include Acme application/process names, a publisher, a prerelease
version, stable product codes, an installation timestamp, native-shaped paths,
process I/O counters, and `api.acme.example` DNS associations. Replay assertions
verify these values survive encoding while device identities change.

Generate into a separate output directory, then review the artifacts before
updating these two checked-in synthetic directories. For example, from the
repository root:

```sh
bazel test //cmd/eudm-simulator/integration:integration_test_zlib \
  --nostamp --workspace_status_command=/usr/bin/true --lockfile_mode=error \
  --test_env=EUDM_GENERATE_FIXTURES=1 --nocache_test_results \
  --test_arg=-test.run=TestGenerateCaptureFixtures
```

The generator writes `eudm-bundles/macos` and `eudm-bundles/windows` under
`TEST_UNDECLARED_OUTPUTS_DIR`; Bazel preserves these in the test's undeclared
output artifacts. An absolute `EUDM_FIXTURE_OUTPUT` overrides that location for
local generation. The writer always refuses existing platform directories.
The generator writes only typed samples. The same 1-GiB total sample, 64-MiB file,
and 4-MiB manifest limits apply. Serialization coverage belongs to
the portable replay tests; no duplicate request files are stored in bundles.

Review regenerated typed samples, inventories, and digests together. Replace only the synthetic `macos/` and `windows/` files;
keep this README and real capture artifacts. Run the simulator suite afterward:

```sh
dda inv test-new --module=. --targets=./cmd/eudm-simulator \
  --bazel-args='--nostamp --workspace_status_command=/usr/bin/true --lockfile_mode=error'
```

Generator success alone is not a replay acceptance result.

Real Windows and macOS captures remain outside the repository as
operator-managed artifacts. Their run reports record their actual digests,
capture-tool revision, and producing builds. Replay accepts the current bundle
schema and producer protocol across capture-tool revisions. Earlier bundle
schemas require recapture with compatible producers.
The loader does not migrate or relabel historical captures. Follow the
[simulator runbook](../../../../doc/how-to/test/eudm-simulator.md) for authenticated
live capture and the deferred staging proofs.
