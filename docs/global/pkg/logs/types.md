# pkg/logs/types

## Purpose

Defines shared data types for the logs system that are imported by multiple packages without pulling in heavier dependencies. Currently the package contains the fingerprinting types used to resume file tailing after an agent restart.

## Key Elements

### Fingerprinting

File fingerprints allow the agent to identify a log file across restarts even if its path or inode changes, by hashing the first few lines or bytes of the file.

| Symbol | Description |
|---|---|
| `Fingerprint` | A computed fingerprint: `Value uint64` (checksum) and `Config *FingerprintConfig` (the config that produced it). `Equals(other)` compares values; `ValidFingerprint()` checks for non-zero value and non-nil config. |
| `FingerprintConfig` | Configuration for how a fingerprint is computed. Fields: `FingerprintStrategy`, `Count` (lines or bytes), `CountToSkip` (lines/bytes to skip before hashing), `MaxBytes` (line-mode cap), `Source`. |
| `FingerprintStrategy` | String enum: `"line_checksum"` (hash the first N lines), `"byte_checksum"` (hash the first N bytes), `"disabled"`. Validated with `Validate() error`. |
| `FingerprintConfigSource` | String enum indicating where the config came from: `"per-source"`, `"global"`, or `"default"`. |
| `InvalidFingerprintValue` | Constant `0` — the zero-value signals an invalid or uncomputed fingerprint. |
| `DefaultLinesCount` | `1` — default line count for `line_checksum`. |
| `DefaultBytesCount` | `1024` — default byte count for `byte_checksum`. |

## Usage

- **`pkg/logs/tailers/file/tailer.go`** — computes a `Fingerprint` when opening a file and stores it in the auditor so the correct offset can be restored after restart.
- **`comp/logs/auditor/impl/auditor.go`** — persists `Fingerprint` values alongside file offsets in the registry; looks them up on startup to resume tailing.
- **`comp/logs/agent/config`** — parses `FingerprintConfig` from log source YAML configuration.
- **`pkg/logs/message/origin.go`** and launcher/provider code — pass `FingerprintConfig` through the pipeline from source config to the tailer.
