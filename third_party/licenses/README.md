# Standard license texts

Snapshot of the license texts that the omnibus build downloads at build time, so that
Bazel can use them without network access to S3.

- `text/<name>`: one file per object in `https://dd-agent-omnibus.s3.amazonaws.com/licenses/`,
  copied byte for byte on 2026-11-09.
- `standard_licenses.bzl`: `STANDARD_LICENSES` (license id -> file), the license-kind mapping and
  `standard_license(id)`, ported from `STANDARD_LICENSES` in omnibus-ruby
  `lib/omnibus/licensing.rb` (last changed in f1254809).

## Bazel targets

`BUILD.bazel` declares a `license` (from `@package_metadata//licenses/rules:license.bzl`) for each
text, named after its file. The kind is `@rules_license//licenses/spdx:<file name>`, except:

- `SPDX_KIND_OVERRIDES` (in `standard_licenses.bzl`) maps omnibus names that differ from the SPDX
  id, e.g. `UPL` -> `UPL-1.0` and `FPL-1.0.0` -> `0BSD`.
- `NON_SPDX_KINDS` covers texts with no SPDX id (Chef-MLSA, Oracle-Binary, Public-Domain); each gets a
  local `license_kind` with a `LicenseRef-*` identifier.

`standard_license(id)` returns the `license` target for a license id.

## How omnibus uses these

- `LICENSES/THIRD-PARTY-<name>`: one per distinct `License` value in the project's
  `third_party_licenses` CSV (`LICENSE-3rdparty.csv`, Go modules).
- `LICENSES/<software>-<name>`: for a software definition that declares a license id but no
  `license_file` (today only `python3`, `Python-2.0`).

## Known issues (not fixed here)

- 76 of the 85 S3 objects are opensource.org HTML pages rather than license text. Only these
  are plain text: Apache-2.0, Erlang-Public, GPL-2.0, GPL-3.0, LGPL-2.0, LGPL-3.0, MIT,
  Python-2.0 and Ruby.
- The MIT text is a template (`<YEAR>, <COPYRIGHT HOLDER>`), and it is not the SPDX wording.
- `AAL` maps to an object named `AALAAL`.
- Omnibus never fetches `LiLiQ-R+`, because it does not URL-encode the `+`. Fetching it as
  `LiLiQ-R%2B` works.

## Additions not in omnibus

These ids appear in `LICENSE-3rdparty.csv`, but omnibus could not resolve them, so it shipped
no text:

| Id | Text | Used by |
|---|---|---|
| `UPL-1.0` | `UPL` (S3) | oracle/oci-go-sdk |
| `Public Domain` | `Public-Domain` (S3) | xi2/xz |
| `BSD-2-Clause-Views` | SPDX license-list-data v3.27.0 | rcrowley/go-metrics |
