# deps/cacerts

Vendors the Mozilla CA certificate bundle, converted to PEM by curl.se, for use
as our trusted root store. Source: https://curl.se/docs/caextract.html

## Files

- `cacerts.MODULE.bazel` — pulls `cacert.pem` and its license via `http_file`.
  Pinned by `version` (a `YYYY-MM-DD` date) and `sha` (its sha256).
- `BUILD.bazel` — exposes the downloaded files as targets.

## Updating the pinned version

1. Check the newest date on https://curl.se/docs/caextract.html and compare
   it to `version` in `cacerts.MODULE.bazel`. If it's not newer, stop — no
   update needed.
2. Fetch the sha256 for that date from
   `https://curl.se/ca/cacert-<version>.pem.sha256`.
3. Update `version` and `sha` in `cacerts.MODULE.bazel` to match.
4. Run `bazel build //deps/cacerts/...` to confirm the new file downloads and
   the sha256 verifies.

`update_cacerts.py` in this directory automates steps 1-3 (dry-run by
default; pass `--write` to edit the file).

There is a cron job that watches curl.se for new dates and alerts
`#team-agent-build` in Slack
(https://app.datadoghq.com/synthetics/details/pya-ptn-xnv) — that alert is
the usual trigger for this update. There's no need to rush a release: new
root certs are phased in gradually, so the old bundle keeps working for a
long time before its replacement is actually needed.
