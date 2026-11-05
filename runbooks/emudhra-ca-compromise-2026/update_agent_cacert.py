#!/usr/bin/env python3
"""Replace Datadog Agent CA bundles with the latest Mozilla extract, then
strip roots that still anchor the compromised eMudhra intermediates.

The curl.se extract dated 2026-11-05 still ships emSign Root CA - G1.
Dropping that file in unchanged does not remove the cross-sign path.

Dry run writes nothing. Rollback restores the newest backup this script made.
"""

import argparse
import base64
import hashlib
import os
import re
import shutil
import sys
import tempfile
import time
import urllib.request

# SHA-256 of the DER. The six TLS intermediates are cross-signed by
# Sectigo's "AAA Certificate Services", so that root has to go too.
DISTRUST = {
    "D7A7A0FB5D7E2731D771E9484EBCDEF71D5F0C3E0A2948782BC83EE0EA699EF4": "AAA Certificate Services",
    "40F6AF0346A99AA1CD1D555A4E9CCE62C7F9634603EE406615833DC8C8D00367": "emSign Root CA - G1",
    "86A1ECBA089C4A8D3BBE2734C612BA341D813E043CF9E8A862CD5C57A36BBE6B": "emSign ECC Root CA - G3",
    "125609AA301DA0A249B97A8239CB6A34216F44DCAC9F3954B14292F2E8C8608F": "emSign Root CA - C1",
    "BC4D809B15189D78DB3E1D8CF4F9726A795DA1643CA5F1358E1DDB0EDC0D7EB3": "emSign ECC Root CA - C3",
}

# Datadog intake chains end at one of these once AAA is gone.
REQUIRED = {
    "E793C9B02FD8AA13E21C31228ACCB08119643B749C898964B1746D46C3D4CBD2": "USERTrust RSA Certification Authority",
    "CB3CCBB76031E5E0138F8DD39A23F9DE47FFC35E43C1144CEA27D46A5AB1CB5F": "DigiCert Global Root G2",
}

BUNDLE_NAMES = ("cacert.pem", "cert.pem", "ca-certificates.crt", "cacerts.txt")
BLOCK = re.compile(r"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----", re.S)
MIN_PARSED = 10
BACKUP_MARK = ".pre-cert-update-"
UA = "datadog-agent-cacert-update"


def fingerprint(block):
    body = "".join(line.strip() for line in block.splitlines()[1:-1])
    try:
        return hashlib.sha256(base64.b64decode(body)).hexdigest().upper()
    except Exception:
        return None


def die(msg, code=1):
    print(f"Error: {msg}", file=sys.stderr)
    sys.exit(code)


def fetch_latest():
    req = urllib.request.Request("https://curl.se/docs/caextract.html", headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=30) as resp:
        html = resp.read().decode("utf-8", "replace")
    match = re.search(r"cacert-(\d{4}-\d{2}-\d{2})\.pem", html)
    if not match:
        die("could not find a cacert date on curl.se")
    date = match.group(1)
    url = f"https://curl.se/ca/cacert-{date}.pem"
    pem = _get(url)
    sha_line = _get(url + ".sha256").decode("ascii", "replace").split()
    if not sha_line or hashlib.sha256(pem).hexdigest() != sha_line[0]:
        die(f"sha256 of {url} does not match the published sidecar")
    print(f"Fetched {url} ({len(pem)} bytes)")
    return pem.decode("latin-1"), date


def _get(url):
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return resp.read()


def clean_bundle(text):
    """Return PEM text with distrusted roots removed. Refuse a bundle that
    does not still contain both Datadog anchors."""
    matches = list(BLOCK.finditer(text))
    if len(matches) < MIN_PARSED:
        die(f"source bundle has only {len(matches)} certificates")
    removed = []
    cut = []
    kept = set()
    for m in matches:
        fp = fingerprint(m.group(0))
        if fp in DISTRUST:
            removed.append(DISTRUST[fp])
            cut.append(m.span())
        elif fp:
            kept.add(fp)
    out, prev = [], 0
    for start, end in cut:
        out.append(text[prev:start])
        prev = end
        while prev < len(text) and text[prev] == "\n":
            prev += 1
    out.append(text[prev:])
    cleaned = "".join(out)
    if cleaned and not cleaned.endswith("\n"):
        cleaned += "\n"
    fps = set()
    for block in BLOCK.findall(cleaned):
        fp = fingerprint(block)
        if fp:
            fps.add(fp)
    if fps & set(DISTRUST):
        die("a distrusted root survived the clean step")
    missing = [name for fp, name in REQUIRED.items() if fp not in fps]
    if missing:
        die("cleaned bundle is missing {}".format(", ".join(missing)))
    return cleaned, removed, len(matches), len(fps)


def iter_bundles(root):
    real_root = os.path.realpath(root)
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        # Never walk out of the install via a directory symlink.
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in sorted(filenames):
            if name not in BUNDLE_NAMES:
                continue
            path = os.path.join(dirpath, name)
            yield path, real_root


def readable_bundle(path, real_root):
    if os.path.islink(path):
        return None, f"symlink -> {os.path.realpath(path)}"
    real = os.path.realpath(path)
    if not (real == real_root or real.startswith(real_root + os.sep)):
        return None, "resolves outside the Agent install"
    try:
        text = open(path, "rb").read().decode("latin-1")
    except OSError as exc:
        return None, f"not readable ({exc})"
    blocks = BLOCK.findall(text)
    if len(blocks) < MIN_PARSED:
        return None, f"only {len(blocks)} certificate(s); not treated as a trust store"
    return text, None


def copy_owner(path, st):
    # Windows has no os.chown; files there inherit the directory ACL.
    if not hasattr(os, "chown"):
        return
    try:
        os.chown(path, st.st_uid, st.st_gid)
    except OSError:
        pass


def apply_bundles(root, cleaned, dry_run):
    stamp = time.strftime("%Y%m%d%H%M%S")
    changed = skipped = 0
    seen = False
    for path, real_root in iter_bundles(root):
        seen = True
        text, reason = readable_bundle(path, real_root)
        if reason:
            skipped += 1
            print(f"  SKIPPED {path}\n    {reason}")
            continue
        if text == cleaned:
            print(f"  unchanged {path}")
            continue
        old_n = len(BLOCK.findall(text))
        new_n = len(BLOCK.findall(cleaned))
        if dry_run:
            changed += 1
            print(f"  would replace {path} ({old_n} -> {new_n} certs)")
            continue
        backup = f"{path}{BACKUP_MARK}{stamp}"
        if os.path.exists(backup):
            backup = f"{backup}-{os.getpid()}"
        tmp = None
        try:
            st = os.stat(path)
            shutil.copy2(path, backup)
            copy_owner(backup, st)
            fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), prefix=".cacert-", suffix=".tmp")
            with os.fdopen(fd, "wb") as handle:
                handle.write(cleaned.encode("latin-1"))
                handle.flush()
                os.fsync(handle.fileno())
            shutil.copymode(path, tmp)
            copy_owner(tmp, st)
            os.replace(tmp, path)
            tmp = None
        except OSError as exc:
            for leftover in (tmp, backup):
                if leftover and os.path.exists(leftover):
                    try:
                        os.unlink(leftover)
                    except OSError:
                        pass
            die(f"could not rewrite {path} ({exc}); left untouched")
        changed += 1
        print(f"  replaced {path} ({old_n} -> {new_n} certs)")
        print(f"    backup: {backup}")
    if not seen:
        die(f"no CA bundles found under {root}")
    return changed, skipped


def rollback(root, dry_run):
    restored = 0
    by_live = {}
    real_root = os.path.realpath(root)
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in filenames:
            if BACKUP_MARK not in name:
                continue
            backup = os.path.join(dirpath, name)
            live_name = name.split(BACKUP_MARK, 1)[0]
            if live_name not in BUNDLE_NAMES:
                continue
            live = os.path.join(dirpath, live_name)
            by_live.setdefault(live, []).append(backup)
    if not by_live:
        print(f"No backups found under {root}.")
        return 0
    for live, backups in sorted(by_live.items()):
        backup = sorted(backups)[-1]
        if os.path.islink(live) or os.path.islink(backup):
            print(f"  SKIPPED {live} (symlink)")
            continue
        if not os.path.realpath(live).startswith(real_root + os.sep):
            print(f"  SKIPPED {live} (outside the install)")
            continue
        if dry_run:
            print(f"  would restore {live} from {backup}")
            restored += 1
            continue
        st = os.stat(live) if os.path.exists(live) else None
        shutil.copy2(backup, live + ".rollback-tmp")
        os.replace(live + ".rollback-tmp", live)
        if st is not None:
            os.chmod(live, st.st_mode)
            copy_owner(live, st)
        print(f"  restored {live} from {backup}")
        restored += 1
    return restored


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    default_dir = r"C:\Program Files\Datadog\Datadog Agent" if os.name == "nt" else "/opt/datadog-agent"
    parser.add_argument("-p", "--install-dir", default=default_dir)
    parser.add_argument("-c", "--bundle", help="local cacert PEM instead of downloading")
    parser.add_argument("-n", "--dry-run", action="store_true")
    parser.add_argument("-r", "--rollback", action="store_true")
    args = parser.parse_args()
    root = args.install_dir
    if not os.path.isdir(root):
        die(f"install dir {root} does not exist")
    if args.rollback and args.bundle:
        die("--rollback does not take --bundle")
    if args.rollback:
        n = rollback(root, args.dry_run)
        verb = "would be restored" if args.dry_run else "restored"
        print(f"{n} bundle(s) {verb}.")
        return
    if args.bundle:
        try:
            raw = open(args.bundle, "rb").read().decode("latin-1")
        except OSError as exc:
            die(f"cannot read {args.bundle} ({exc})")
        source = args.bundle
    else:
        raw, source = fetch_latest()
    cleaned, removed, before, after = clean_bundle(raw)
    dropped = ", ".join(removed) if removed else "nothing"
    print(f"Source {source}: {before} certs, removed {dropped}, {after} remain")
    if args.dry_run:
        print("Dry run: no bundle will be written.")
    changed, skipped = apply_bundles(root, cleaned, args.dry_run)
    verb = "would be replaced" if args.dry_run else "replaced"
    print(f"{changed} bundle(s) {verb}, {skipped} skipped.")


if __name__ == "__main__":
    main()
