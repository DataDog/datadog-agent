# Agent 7 — remove the compromised eMudhra roots from the embedded CA bundles (2026)

Removes the roots that still anchor the compromised eMudhra intermediates from
the CA bundles shipped inside a Datadog Agent 7 install. Everything else in
each bundle is kept, including certificates added on the host, such as a
corporate proxy CA. With `--latest`, each bundle is instead replaced by the
current Mozilla extract from curl.se, minus the same roots. Dry run and
rollback are built in.

This is not a substitute for upgrading to 7.84.6 or later. That release also
rejects the compromised intermediate keys inside embedded Python, and on
Windows it stops trusting them through the Windows certificate store. Editing
PEM files does not add those checks. The Go Agent on Linux reads the operating
system trust store unless `SSL_CERT_FILE` is set. The Go Agent on Windows reads
the Windows certificate store.

## What gets written

Only the bundles the Agent ships, relative to the install directory (default
`/opt/datadog-agent`, or `C:\Program Files\Datadog\Datadog Agent` on Windows):

| Linux | Windows |
|---|---|
| `embedded/ssl/certs/cacert.pem` | `embedded3\ssl\certs\cacert.pem` |
| `embedded/ssl/cert.pem` (a symlink, left alone) | `embedded3\ssl\cert.pem` |
| `embedded/lib/python3.*/site-packages/certifi/cacert.pem` | `embedded3\Lib\site-packages\certifi\cacert.pem` |
| the same under `pip/_vendor/certifi/` | the same under `pip\_vendor\certifi\` |
| the same under `botocore/` | the same under `botocore\` |

Other bundles under the install directory, such as those of integrations or
packages installed later, are listed as `not touched` and never written.
Files with fewer than 10 certificates and symlinks are skipped.

These roots are removed:

- AAA Certificate Services
- emSign Root CA - G1
- emSign ECC Root CA - G3
- emSign Root CA - C1
- emSign ECC Root CA - C3

AAA has to go because the six compromised intermediates are cross-signed by it.

`--latest` uses the newest `cacert-YYYY-MM-DD.pem` linked from
https://curl.se/docs/caextract.html, checked against the published `.sha256`.
As of 2026-11-05 that extract still contains emSign Root CA - G1, so it is
removed there too. The script refuses an extract whose result is missing
USERTrust RSA Certification Authority or DigiCert Global Root G2, or that
OpenSSL cannot load. A replaced bundle loses every certificate that is not in
the extract; the dry run lists each one as `drops (not in the extract)`, so
check that list for CAs you added yourself.

Each written file is copied to `<file>.pre-cert-update-<YYYYmmddHHMMSS>`
beside it. On Linux, mode and owner are kept. On Windows the new file inherits
the directory ACL like the original did, but its owner becomes the
Administrators group instead of SYSTEM.

## Usage

Download the script and run it with the Agent's own Python, as root on Linux.
The script needs nothing beyond the standard library, and the default mode
needs no network access. On hosts without access to GitHub, download it
elsewhere and copy it over.

```sh
curl -O https://raw.githubusercontent.com/DataDog/datadog-agent/main/runbooks/emudhra-ca-compromise-2026/update_agent_cacert.py
PY=/opt/datadog-agent/embedded/bin/python3
sudo "$PY" update_agent_cacert.py -n       # dry run
sudo "$PY" update_agent_cacert.py          # remove the distrusted roots
sudo "$PY" update_agent_cacert.py -r       # restore the newest backup of each file
```

`--latest` (`-l`) replaces with the Mozilla extract instead; add `-c <file>` to
use a PEM you already have instead of downloading. `-p <dir>` is the install
directory. `-n` with `-r` previews the rollback.

On Windows, from an elevated PowerShell:

```powershell
Invoke-WebRequest -UseBasicParsing -OutFile update_agent_cacert.py https://raw.githubusercontent.com/DataDog/datadog-agent/main/runbooks/emudhra-ca-compromise-2026/update_agent_cacert.py
$py = "C:\Program Files\Datadog\Datadog Agent\embedded3\python.exe"
& $py update_agent_cacert.py -n
& $py update_agent_cacert.py
& $py update_agent_cacert.py -r
```

Restart the Agent after a real run. On Linux that is
`sudo systemctl restart datadog-agent` when the host is booted with systemd.
On Windows it is `Restart-Service datadogagent -Force`. The script does not
restart anything, and it does not edit the OS or Windows certificate store.

## Check

A scrubbed bundle must reject the eMudhra test chain and still accept Datadog:

```sh
PY=/opt/datadog-agent/embedded/bin/python3
"$PY" - <<'PY'
import socket, ssl
def check(host, cafile):
    ctx = ssl.create_default_context(cafile=cafile)
    try:
        ctx.wrap_socket(socket.create_connection((host, 443), timeout=15), server_hostname=host)
        return "accepted"
    except ssl.SSLError:
        return "rejected"
ca = "/opt/datadog-agent/embedded/ssl/certs/cacert.pem"
print(check("dv-g3b-aaa-emudhra.we-have-a-problem.com", ca), "compromised test chain")
print(check("app.datadoghq.com", ca), "app.datadoghq.com")
PY
```

Expected: `rejected` for the test chain, `accepted` for `app.datadoghq.com`.

`tests/tls_check.py <embedded dir>` runs the same check against every
`cacert.pem`/`cert.pem` under that directory and against Python's default
context.

## Tests

`tests/test_update.sh` runs the script against fake Linux and Windows installs
built from a Mozilla extract at `/tmp/cacert-latest.pem` (override with
`CACERT_SRC`) plus the fixtures in `tests/fixtures/`: the real emSign Root CA -
G1 and a self-signed stand-in for a CA added on the host. It covers the
refusals (no Agent bundles, `-c` without `--latest`, a source OpenSSL cannot
load), scrub keeping the added CA, a bundle outside the shipped list left
alone, mode and symlink preservation, a no-op second run, `--latest` listing
what it drops, and rollback after both modes.

## Checked on Agent 7.84.5

These runs replaced each bundle with the Mozilla extract, which is what
`--latest` does now. The default mode removes the same roots, including AAA
from botocore's bundle, which is the only root the test chain anchors to there;
it is covered by `tests/test_update.sh` but was not rerun on these hosts.

### Linux

Ubuntu 24.04 arm64 container without systemd, so the package was installed
with `DD_INSTALL_ONLY=true` and the service was not started.

Shipped bundles, before the script:

| File | Certs | Distrusted roots still present | Test chain |
|---|---|---|---|
| `embedded/ssl/certs/cacert.pem` (Mozilla 2026-10-20) | 118 | emSign Root CA - G1 | rejected |
| `certifi/cacert.pem` and pip's copy | 120 | emSign G1, C1, ECC C3 | rejected |
| `botocore/cacert.pem` | 127 | AAA Certificate Services | accepted |

`embedded/ssl/cert.pem` is a symlink to `certs/cacert.pem` on Linux, so the
script lists it as skipped; its target is updated.

The published test chain anchors through AAA, so only the botocore bundle
accepted it. Dry run, apply, rollback, and a second apply all worked. After
apply, all four bundles had 117 certificates, rejected the test chain, and
accepted `app.datadoghq.com` and `app.datadoghq.eu`.

The host OS store (`/etc/ssl/certs/ca-certificates.crt`, 238 certs) still has
emSign G1, C1, and ECC C3. It does not have AAA, and Python's default context
rejected the test chain. The Go Agent reads that store, not the embedded
bundles. This script does not edit it.

### Windows

EC2 Windows Server 2025 from `dda inv aws.create-vm --os-family=windows
--agent-version=7.84.5`, Agent 7.84.5 (Python 3.13.15, OpenSSL 3.5.9).

| File under `embedded3\` | Certs | Test chain before | After |
|---|---|---|---|
| `ssl\cert.pem` and `ssl\certs\cacert.pem` (separate files, not links) | 118 | rejected | rejected |
| `Lib\site-packages\certifi\cacert.pem` and pip's copy | 120 | rejected | rejected |
| `Lib\site-packages\botocore\cacert.pem` | 127 | accepted | rejected |

Python's compiled-in default CA path is a build-machine path that does not
exist, so `ssl.create_default_context()` uses the Windows store. On this image
the Windows store has no AAA or emSign roots in `Root`, `CA`, or `AuthRoot`, and
both Python's default context and `Invoke-WebRequest` (SChannel, the same trust
the Go Agent uses) rejected the test chain. A host whose store has picked up AAA
through Windows root auto-update would accept it; this script does not change
that.

Dry run, apply, a second apply (no-op), rollback dry run, rollback, and
re-apply all worked. After re-apply and `Restart-Service datadogagent`,
`agent health` was `PASS`.
