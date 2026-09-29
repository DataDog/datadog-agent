# Portable bundle fixtures

`macos/` and `windows/` contain small synthetic capture bundles for automated
tests. Their typed inputs are constructed in
[fixtures_test.go](../../integration/fixtures_test.go) and submitted through the
real Agent serializers, process submission path, and event-platform delivery
pipeline with an in-memory recording transport. No live device collection or
staging request is involved.

These fixtures exercise bundle compatibility, sanitization contracts, portable
mixed-platform replay, process/software overlays, Windows connection evidence,
wireless correlation, and delivery accounting. They do not prove native Windows
capture, staging device enrichment, monitor behavior, or Command Center/Bits
acceptance.

Both fixtures use the deliberate test-only Agent commit
`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` and version `7.85.0-fixture`.
A normal revision-stamped simulator binary rejects them as incompatible; use
real operator-managed captures for staging. Do not edit their manifests to
claim the revision of a real binary.

Each bundle includes:

- `manifest.json`, with profile inventories, relative sample offsets, stream
  cadences, schema/sanitizer versions, and file digests.
- `COMPLETE`, containing the manifest digest.
- Sanitized typed `sample-*.json` files. Metric envelopes retain Agent source
  identifiers and fractional relative timestamps.
- `sample-*-wire-*.json` references recorded after Agent serialization. The
  reference bodies are encoded as JSON byte strings and retain their normal
  compression and payload format; credentials are excluded.

The macOS fixture declares an arm64 profile with metrics, host metadata,
processes, and a software snapshot. The Windows fixture declares an amd64
profile with those streams plus connections. Both represent a synthetic 16-GiB
device profile. Fixture facts are chosen for the tests; they are not evidence of
hardware or applications observed on an operator's machine.

To generate the fixtures from the repository root, after ensuring neither
output directory exists:

```sh
EUDM_GENERATE_FIXTURES=1 dda inv test \
  --targets=./cmd/eudm-simulator/integration \
  --build-exclude=python \
  --extra-args='-run TestGenerateCaptureFixtures'
```

Generation is opt-in and the writer refuses existing bundle directories. Before
regeneration, review and move aside only these two synthetic fixture directories;
keep this README and real capture artifacts. Review regenerated typed samples,
decoded wire payloads, inventories, and digests together. Run the simulator tests
through `dda inv test --targets=./cmd/eudm-simulator/... --build-exclude=python`
after regeneration. Generator success alone is not a replay acceptance result.

Real Windows and macOS captures remain outside the repository as
operator-managed artifacts. Their run plans record their actual digests and
Agent revisions; an Agent revision change requires recapture. Follow the
[simulator runbook](../../../../doc/how-to/test/eudm-simulator.md) for native
capture and the deferred staging proofs.
