# scrappy-assets — Scrappy live-detector assets

Three assets are required to build the custom dd-agent image with the Scrappy live-detection
inference detector.  Two are committed here; one is fetched at build time.

## Committed assets

| File | Size | Description |
|------|------|-------------|
| `vocab.json` | 59,802 B | BPE vocabulary (3594 tokens) used by `scrappy_tokenizer.go` to tokenize metric/log surfaces before inference |
| `scrappy-infer` | 55,832 B | Native C inference engine binary (linux/amd64 ELF, AVX2/FMA/F16C acceleration). Runs inference against `model.scrappy` via stdin/stdout JSON protocol |

## Fetched at build time

| File | Size | Why not committed |
|------|------|-------------------|
| `model.scrappy` | 139,883,748 B (~140MB) | Exceeds git object limits; no LFS configured in this repo (largest existing blob ~500KB). Baked into the durable GAR image. |

### Fetch model.scrappy
```bash
# From inside q_branch/generators/observer-eval/scrappy-assets/:
./fetch-model.sh
# Or provide a custom image tag:
./fetch-model.sh us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev:scrappy-detect-20260605-sc5fix
```
The script pulls the GAR image, creates a temporary container, copies
`/opt/scrappy/model.scrappy` out, and removes the container.

## Vocab ↔ model match requirement

**CRITICAL**: `vocab.json` (3594 tokens) MUST match the model version.  
- This vocab corresponds to **v0.3 model** (`v0.3-run-001/epoch_005.pt`, converted to `model.scrappy`).
- The tokenizer expands to exactly 3594 vocabulary entries; the model embedding layer has
  dim 0 = 3594. A mismatch causes an immediate inference error.
- If you update the model checkpoint, regenerate `vocab.json` from the same scrappy-repo revision
  and update both files together.

## Provenance

- `model.scrappy` — converted from checkpoint `v0.3-run-001/epoch_005.pt` via scrappy repo
  conversion tooling (`github.com/DataDog/scrappy`). Also available at `/opt/scrappy/model.scrappy`
  in the durable GAR image.
- `scrappy-infer` — built from `github.com/DataDog/scrappy/native` (CMake, `-mavx2 -mfma -mf16c`,
  target `build-amd64/scrappy-infer`). Also available at `/opt/scrappy/scrappy-infer` in the image.
- `vocab.json` — from `github.com/DataDog/scrappy/native/vocab.json` (or `~/dd/scrappy/vocab.json`).
