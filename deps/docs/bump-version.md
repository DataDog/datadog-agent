# Bump the MODULE.bazel pin

Manual-mode counterpart of the http_archive bump Renovate would otherwise do. Goal: get a trusted sha256 for the new version and write it (plus the new URL / strip_prefix) into the right MODULE.bazel block.

## 1. Resolve the target version

If not given as `$2`, ask the user.

## 2. Acquire the new sha256 from a trusted upstream source

Try in order, stop at the first hit:

1. A SHA-256 sidecar published alongside the artefact: `<url>.sha256`, `<url>.sha256sum`, or a `SHA256SUMS` line for this file. Pin the value verbatim. **Do not** treat `SHA512SUMS` or any non-SHA-256 sidecar as a direct sha256 source — those carry a different digest. SHA-512 / BLAKE2 sidecars can still anchor integrity in the verify-then-hash flow (item 3 below).
2. A structured release manifest from the project (GitHub release assets JSON when the URL is `github.com/...`, a project-specific manifest like `swdb.lst` for GnuPG, etc.).
3. A detached GPG signature against a well-known maintainer key (`<url>.sig` / `<url>.asc`). If present, do the verify-then-hash dance — the signature is the integrity anchor, the local hash is just transcription:
   ```
   # 1. Identify the signing key
   gpg --list-packets <file>.sig | grep -E 'issuer fpr|keyid'

   # 2. Cross-check the fingerprint against the project's HTTPS-published
   #    signing key page (e.g. gnupg.org/signature_key.html). Fingerprint
   #    must match what upstream publishes, byte-for-byte.

   # 3. Import the published key into an isolated GNUPGHOME
   mkdir -p /tmp/<dep>-gpg && chmod 700 /tmp/<dep>-gpg
   curl -sLO <url to published .asc key file>
   GNUPGHOME=/tmp/<dep>-gpg gpg --import <key.asc>

   # 4. Verify the signature
   GNUPGHOME=/tmp/<dep>-gpg gpg --verify <file>.sig <file>

   # 5. Strong sanity check: if a previous version is currently pinned,
   #    verify and hash IT too. The hash you compute must match the one
   #    already in MODULE.bazel — that proves the trust chain end-to-end.

   # 6. Now sha256sum the verified new file and pin that value.
   sha256sum <file>
   ```
   The `WARNING: This key is not certified with a trusted signature` notice from gpg is fine — the cryptographic check itself passed, and the fingerprint cross-check is what anchors trust.
4. **Ask the user to paste the sha256.** Print the exact URL you intend to pin and request the hash explicitly. Do not proceed without one.

**No pin from an unverified download.** Without an out-of-band anchor, local hashing just hashes whatever the network handed you. Bazel's `http_archive` will then re-verify against itself — meaningless.

## 3. Apply the edits

- For `deps/<dep-name>/<dep-name>.MODULE.bazel` or the entry in `deps/repos.MODULE.bazel`: update `version` (if present), `strip_prefix` (typically `<dep-name>-<version>`), `sha256`, and rewrite the URL with the new version. Deps fetch upstream through ADMS/Depot's pull-through mirror (the `rewrite` rules in `.adms/bazel/adms.mirror.cfg`), so there is no separate mirror URL to maintain.
- For `release.json`-driven deps: update `<NAME>_VERSION` and `<NAME>_SHA256`.

Return to the orchestrator's Step 3.
