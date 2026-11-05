#!/usr/bin/env python3
"""Remove the roots that anchor the compromised eMudhra intermediates from the
CA bundles shipped with Datadog Agent 7.

By default only those roots are removed and everything else in each bundle,
including certificates added on the host, is kept. --latest instead replaces
each bundle with the current Mozilla extract from curl.se, minus the same roots.

Only bundles the Agent ships are written. Dry run writes nothing. Rollback
restores the newest backup this script made.
"""

import argparse
import base64
import glob
import hashlib
import os
import re
import shutil
import ssl
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

# Relative to the install directory: Linux layout, then Windows.
SHIPPED = (
    "embedded/ssl/certs/cacert.pem",
    "embedded/ssl/cert.pem",
    "embedded/lib/python3*/site-packages/certifi/cacert.pem",
    "embedded/lib/python3*/site-packages/pip/_vendor/certifi/cacert.pem",
    "embedded/lib/python3*/site-packages/botocore/cacert.pem",
    "embedded3/ssl/certs/cacert.pem",
    "embedded3/ssl/cert.pem",
    "embedded3/Lib/site-packages/certifi/cacert.pem",
    "embedded3/Lib/site-packages/pip/_vendor/certifi/cacert.pem",
    "embedded3/Lib/site-packages/botocore/cacert.pem",
)
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


def fingerprints(text):
    return {fp for fp in map(fingerprint, BLOCK.findall(text)) if fp}


def labels(text):
    """Map fingerprint to the comment line just above each certificate."""
    out, prev = {}, 0
    for m in BLOCK.finditer(text):
        lines = [s.strip().lstrip("#").strip() for s in text[prev : m.start()].splitlines()]
        lines = [s for s in lines if s and set(s) != {"="} and not s.startswith(("MD5", "SHA1", "SHA256"))]
        label = lines[-1] if lines else "(no label)"
        if label.startswith("Label:"):
            label = label[len("Label:") :].strip().strip('"')
        out[fingerprint(m.group(0))] = label
        prev = m.end()
    return out


def strip_distrusted(text):
    removed, out, prev = [], [], 0
    for m in BLOCK.finditer(text):
        fp = fingerprint(m.group(0))
        if fp not in DISTRUST:
            continue
        removed.append(DISTRUST[fp])
        out.append(text[prev : m.start()])
        prev = m.end()
        while prev < len(text) and text[prev] == "\n":
            prev += 1
    out.append(text[prev:])
    cleaned = "".join(out)
    if cleaned and not cleaned.endswith("\n"):
        cleaned += "\n"
    return cleaned, removed


def load_error(text):
    # Only the PEM blocks: cadata must be ASCII, and Mozilla's comments are not.
    try:
        ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT).load_verify_locations(cadata="\n".join(BLOCK.findall(text)))
    except (ssl.SSLError, ValueError) as exc:
        return str(exc)
    return None


def clean_source(text):
    """Strip the Mozilla extract used by --latest and refuse a result that
    lost a Datadog anchor or that OpenSSL cannot load."""
    before = len(BLOCK.findall(text))
    if before < MIN_PARSED:
        die(f"source bundle has only {before} certificates")
    cleaned, removed = strip_distrusted(text)
    fps = fingerprints(cleaned)
    if fps & set(DISTRUST):
        die("a distrusted root survived the clean step")
    missing = [name for fp, name in REQUIRED.items() if fp not in fps]
    if missing:
        die("cleaned bundle is missing {}".format(", ".join(missing)))
    problem = load_error(cleaned)
    if problem:
        die(f"cleaned bundle does not load in OpenSSL ({problem}); nothing written")
    return cleaned, removed, before, len(fps)


def shipped_bundles(root):
    paths = set()
    for pattern in SHIPPED:
        paths.update(glob.glob(os.path.join(root, *pattern.split("/"))))
    return sorted(paths)


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


def report_others(root, shipped):
    real_root = os.path.realpath(root)
    shipped = {os.path.normcase(os.path.abspath(p)) for p in shipped}
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in sorted(filenames):
            path = os.path.join(dirpath, name)
            if name not in BUNDLE_NAMES or os.path.normcase(os.path.abspath(path)) in shipped:
                continue
            text, _ = readable_bundle(path, real_root)
            if text is not None:
                print(f"  not touched (not shipped with the Agent) {path}")


def copy_owner(path, st):
    # Windows has no os.chown; files there inherit the directory ACL.
    if not hasattr(os, "chown"):
        return
    try:
        os.chown(path, st.st_uid, st.st_gid)
    except OSError:
        pass


def write_bundle(path, new, stamp):
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
            handle.write(new.encode("latin-1"))
            handle.flush()
            os.fsync(handle.fileno())
        shutil.copymode(path, tmp)
        copy_owner(tmp, st)
        os.replace(tmp, path)
    except OSError as exc:
        for leftover in (tmp, backup):
            if leftover and os.path.exists(leftover):
                try:
                    os.unlink(leftover)
                except OSError:
                    pass
        die(f"could not rewrite {path} ({exc}); left untouched")
    return backup


def apply_bundles(root, latest, dry_run):
    """Scrub each shipped bundle, or replace it with latest when given."""
    stamp = time.strftime("%Y%m%d%H%M%S")
    real_root = os.path.realpath(root)
    changed = skipped = 0
    paths = shipped_bundles(root)
    if not paths:
        die(f"no Agent CA bundles found under {root}")
    latest_fps = fingerprints(latest) if latest is not None else None
    for path in paths:
        text, reason = readable_bundle(path, real_root)
        if reason:
            skipped += 1
            print(f"  SKIPPED {path}\n    {reason}")
            continue
        if latest is None:
            new, removed = strip_distrusted(text)
        else:
            new = latest
        if new == text:
            print(f"  unchanged {path}")
            continue
        problem = load_error(new)
        if problem:
            skipped += 1
            print(f"  SKIPPED {path}\n    result would not load in OpenSSL ({problem})")
            continue
        old_n, new_n = len(BLOCK.findall(text)), len(BLOCK.findall(new))
        verb = "would replace" if dry_run else "replaced"
        print(f"  {verb} {path} ({old_n} -> {new_n} certs)")
        if latest is None:
            print("    removes: {}".format(", ".join(dict.fromkeys(removed))))
        else:
            names = labels(text)
            for fp in sorted(fingerprints(text) - latest_fps):
                kind = "distrusted" if fp in DISTRUST else "not in the extract"
                print(f"    drops ({kind}): {names.get(fp, '(no label)')} {fp[:16]}")
        if not dry_run:
            print(f"    backup: {write_bundle(path, new, stamp)}")
        changed += 1
    report_others(root, paths)
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
    parser.add_argument("-l", "--latest", action="store_true", help="replace with the Mozilla extract")
    parser.add_argument("-c", "--bundle", help="with --latest: local cacert PEM instead of downloading")
    parser.add_argument("-n", "--dry-run", action="store_true")
    parser.add_argument("-r", "--rollback", action="store_true")
    args = parser.parse_args()
    root = args.install_dir
    if not os.path.isdir(root):
        die(f"install dir {root} does not exist")
    if args.rollback and (args.bundle or args.latest):
        die("--rollback does not take --latest or --bundle")
    if args.bundle and not args.latest:
        die("--bundle needs --latest")
    if args.rollback:
        n = rollback(root, args.dry_run)
        verb = "would be restored" if args.dry_run else "restored"
        print(f"{n} bundle(s) {verb}.")
        return
    latest = None
    if args.latest:
        if args.bundle:
            try:
                raw = open(args.bundle, "rb").read().decode("latin-1")
            except OSError as exc:
                die(f"cannot read {args.bundle} ({exc})")
            source = args.bundle
        else:
            raw, source = fetch_latest()
        latest, removed, before, after = clean_source(raw)
        dropped = ", ".join(removed) if removed else "nothing"
        print(f"Source {source}: {before} certs, removed {dropped}, {after} remain")
    if args.dry_run:
        print("Dry run: no bundle will be written.")
    changed, skipped = apply_bundles(root, latest, args.dry_run)
    verb = "would be replaced" if args.dry_run else "replaced"
    print(f"{changed} bundle(s) {verb}, {skipped} skipped.")


if __name__ == "__main__":
    main()
