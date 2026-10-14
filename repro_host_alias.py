#!/usr/bin/env python3
"""
AGENTRUN-1474 — detect the "cloud resource alias missing from host v5 payload"
failure signature, optionally reproducing it by blocking IMDS across a restart.

Run this ON the cloud VM (or in the Agent container), as root.

Prerequisites in /etc/datadog-agent/datadog.yaml:
    log_level: debug
    log_payloads: true

The local failure signature is:
    - the expected cloud alias is absent from meta.host_aliases
    - meta.ccrid is empty
(Server-side, sobotka additionally shows cloud_provider = unknown_cloud; the
Agent does not put the cloud name in the v5 payload, so we can't check it here.)

Modes, least to most invasive:
    --scan              parse the existing log, report every v5 payload   (default, read-only)
    --restart           restart the Agent, then evaluate the new payloads
    --restart --block N  also DROP traffic to IMDS for N seconds around the restart
"""

import argparse
import json
import os
import socket
import subprocess
import sys
import time

AGENT_LOG = "/var/log/datadog/agent.log"
MARK = "Sending metadata payload, content: "
DMI = "/sys/devices/virtual/dmi/id"
AZURE_CHASSIS_TAG = "7783-7084-3265-9085-8269-3286-77"
IMDS_IP = "169.254.169.254"

G, R, Y, B, X = "\033[32m", "\033[31m", "\033[33m", "\033[1m", "\033[0m"


def dmi(field):
    try:
        with open(os.path.join(DMI, field)) as fh:
            return fh.read().strip()
    except OSError:
        return ""


def project_id_from_hostname(fqdn):
    """GCE internal DNS: <instance>.<zone>.c.<project>.internal (or without zone)."""
    parts = fqdn.rstrip(".").split(".")
    if len(parts) < 4 or parts[-1] != "internal" or parts[-3] != "c":
        return ""
    return parts[-2]


def detect_expected():
    """Work out, from purely local sources, which alias the payload must contain."""
    if dmi("chassis_asset_tag") == AZURE_CHASSIS_TAG:
        uuid = dmi("product_uuid").lower()
        if not uuid:
            return "Azure", "", "product_uuid unreadable — run as root"
        return "Azure", uuid, ""

    vendor = dmi("sys_vendor") or dmi("board_vendor")
    if vendor.startswith("Google"):
        fqdn = socket.getfqdn()
        project = project_id_from_hostname(fqdn)
        if not project:
            return "GCP", "", f"hostname {fqdn!r} is not a GCE internal name"
        return "GCP", f"{parts_first(fqdn)}.{project}", ""

    if vendor == "Amazon EC2":
        return "AWS", dmi("board_asset_tag"), ""

    return "", "", "no cloud detected from DMI"


def parts_first(fqdn):
    return fqdn.split(".")[0]


def classify(payload):
    """Several metadata payloads embed `meta`; only one is the host v5 payload."""
    if "meta" not in payload:
        return None
    if "systemStats" in payload or "gohai" in payload:
        return "v5"
    if "agent_checks" in payload:
        return "checks"  # carries a cached copy of meta, not a fresh collection
    return None


def parse_v5(path, offset=0):
    """Return ([(timestamp, payload, kind)], byte_offset_after)."""
    out = []
    size = os.path.getsize(path)
    if offset > size:  # log rotated under us
        offset = 0
    with open(path, errors="replace") as fh:
        fh.seek(offset)
        for line in fh:
            i = line.find(MARK)
            if i < 0:
                continue
            try:
                payload = json.loads(line[i + len(MARK):].strip())
            except ValueError:
                continue
            kind = classify(payload)
            if kind:
                out.append((line[:19], payload, kind))
        end = fh.tell()
    return out, end


def verdict(payload, cloud, expected):
    """Only meaningful on a cloud host: off-cloud, empty aliases/ccrid are correct."""
    meta = payload.get("meta", {})
    aliases = meta.get("host_aliases") or []
    ccrid = meta.get("ccrid", "")
    problems = []
    if cloud:
        if expected and expected not in aliases:
            problems.append(f"missing {cloud} alias {expected}")
        elif not expected and not aliases:
            problems.append("host_aliases empty")
        if not ccrid:
            # collect_ccrid defaults to true, so this is a real signal
            problems.append("ccrid empty")
    return problems, aliases, ccrid


def guard_foreign_log(rows, cloud, expected, explicit):
    """Local DMI only describes the local machine. If the log came from elsewhere,
    disable verdicts rather than reporting confident nonsense."""
    if explicit or not rows:
        return cloud, expected
    # Compare against meta.socket-hostname (os.Hostname()), NOT internalHostname:
    # the latter is the Agent hostname and is often overridden via `hostname:`/DD_HOSTNAME.
    local = socket.gethostname().split(".")[0]
    hosts = {p.get("meta", {}).get("socket-hostname", "").split(".")[0] for _, p, _ in rows}
    hosts.discard("")
    if hosts and local not in hosts:
        print(f"{Y}note:{X} log is from {sorted(hosts)}, this machine is {local!r} — "
              f"local DMI does not describe it, so alias/ccrid verdicts are disabled.\n"
              f"      pass --expect <alias> to check a log copied from another host.\n")
        return "", ""
    return cloud, expected


def report(rows, cloud, expected):
    bad = 0
    judging = bool(cloud)
    for ts, payload, kind in rows:
        problems, aliases, ccrid = verdict(payload, cloud, expected)
        tag = "host-v5" if kind == "v5" else "checks "
        if problems:
            bad += 1
            print(f"{R}FAIL{X} {ts}  {tag}  {'; '.join(problems)}")
        else:
            mark = f"{G} OK {X}" if judging else f"{Y} -- {X}"
            print(f"{mark} {ts}  {tag}  aliases={aliases}")
        print(f"            ccrid={ccrid or '(empty)'}")
    if not rows:
        print(f"{Y}no metadata payloads found — is log_payloads:true and log_level:debug set?{X}")
    elif not judging:
        print(f"\n{Y}'--' means reported but NOT checked (verdicts disabled).{X}")
    else:
        n5 = sum(1 for _, _, k in rows if k == "v5")
        bad5 = sum(1 for ts, p, k in rows if k == "v5" and verdict(p, cloud, expected)[0])
        extra = f" (+{bad - bad5} in cached checks payloads)" if bad > bad5 else ""
        print(f"\n{n5} host-v5 payload(s) checked, {bad5} problem(s){extra}.")
    return bad


def imds_block(enable):
    rule = ["-d", IMDS_IP, "-j", "DROP"]
    action = "-I" if enable else "-D"
    subprocess.run(["iptables", action, "OUTPUT"] + rule, check=not enable)


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--log", default=AGENT_LOG)
    ap.add_argument("--scan", action="store_true", help="read-only scan of the existing log")
    ap.add_argument("--restart", action="store_true", help="restart the Agent and evaluate")
    ap.add_argument("--block", type=int, metavar="N",
                    help="with --restart: DROP traffic to IMDS for N seconds")
    ap.add_argument("--wait", type=int, default=25, help="seconds to wait for the first payload")
    ap.add_argument("--expect", metavar="ALIAS",
                    help="expected cloud alias; required when scanning a log copied "
                         "from another host, since local DMI would describe the wrong machine")
    args = ap.parse_args()

    if not os.path.exists(args.log):
        sys.exit(f"{args.log} not found")

    if args.expect:
        cloud, expected, note = "override", args.expect, "from --expect"
    else:
        cloud, expected, note = detect_expected()
    print(f"{B}cloud (from DMI):{X} {cloud or '(none)'}"
          + (f"   expected alias: {expected}" if expected else "")
          + (f"   [{note}]" if note else ""))
    print(f"{B}log:{X} {args.log}\n")

    if not args.restart:
        rows, _ = parse_v5(args.log)
        cloud, expected = guard_foreign_log(rows, cloud, expected, args.expect)
        sys.exit(1 if report(rows, cloud, expected) else 0)

    _, offset = parse_v5(args.log)  # only look at what comes next
    blocked = False
    try:
        if args.block:
            print(f"{Y}blocking {IMDS_IP} for {args.block}s{X}")
            imds_block(True)
            blocked = True

        subprocess.run(["systemctl", "restart", "datadog-agent"], check=True)
        print("agent restarted; waiting for the first v5 payload...")

        if args.block:
            time.sleep(args.block)
            imds_block(False)
            blocked = False
            print(f"{Y}unblocked {IMDS_IP}{X}")

        deadline = time.time() + args.wait
        rows = []
        while time.time() < deadline:
            rows, _ = parse_v5(args.log, offset)
            if rows:
                break
            time.sleep(1)
        print()
        sys.exit(1 if report(rows, cloud, expected) else 0)
    finally:
        if blocked:
            imds_block(False)
            print(f"{Y}unblocked {IMDS_IP} (cleanup){X}")


if __name__ == "__main__":
    main()
