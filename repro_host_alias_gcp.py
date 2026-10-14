#!/usr/bin/env python3
"""
AGENTRUN-1474 (GCP) — detect, and reproduce on demand, the "cloud resource alias
missing from the host v5 payload" failure on Google Compute Engine.

Run this ON the GCE VM (or in the Agent container), as root.

Prerequisites in /etc/datadog-agent/datadog.yaml:
    log_level: debug
    log_payloads: true

WHY A GCP-SPECIFIC SCRIPT
-------------------------
GCE publishes TWO host aliases, from different metadata endpoints, and they fail
independently:

    #1  <instance>.<zone>.c.<project>.internal   from /instance/hostname
    #2  <instance>.<project>                     from /instance/name + /project/project-id

Alias #2 is the "Cloud Resource Name" — the form the Datadog GCP integration
uses to name the host it discovers on its own, so it is the load-bearing one for
host matching. Alias #1 is warmed during early hostname resolution (the gce
hostname provider runs before the metadata runner starts), so it normally
survives an IMDS outage from a fresh process. Alias #2 is never warmed.

The predicted signature is therefore "#1 present, #2 missing", which is what the
reported incident shows. Checking the two together would hide exactly the
distinction that validates the model, so this script judges them separately and
names which one went missing.

MODES, least to most invasive
-----------------------------
    --scan                      parse the existing log, report every payload  (default, read-only)
    --restart                   restart the Agent, then evaluate the new payloads
    --restart --block N         also DROP all traffic to IMDS for N seconds
    --restart --block-project-id N
                                block ONLY requests for /project/project-id, leaving
                                /instance/hostname and /instance/name reachable. This
                                isolates the single point of failure in getInstanceAlias:
                                projectID has no fallback, so one failed fetch is
                                expected to discard the whole of alias #2.

All iptables rules are removed in a finally block, and none of them survive a
reboot, so a stuck rule self-clears. Only the IMDS address is ever affected,
never SSH.
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
IMDS_IP = "169.254.169.254"
PROJECT_ID_PATH = "/project/project-id"

G, R, Y, B, X = "\033[32m", "\033[31m", "\033[33m", "\033[1m", "\033[0m"


def dmi(field):
    try:
        with open(os.path.join(DMI, field)) as fh:
            return fh.read().strip()
    except OSError:
        return ""


def is_gce():
    """GCE reports Google in sys_vendor/board_vendor. Unlike AWS, SMBIOS carries
    neither the instance name nor the project, so DMI can detect the cloud but
    cannot reconstruct either alias."""
    return (dmi("sys_vendor") or dmi("board_vendor")).startswith("Google")


def split_internal_dns(fqdn):
    """Parse a GCE internal DNS name into (instance, zone, project).

    Two forms exist, both ending in ".c.<project>.internal":
        zonal (the default): <instance>.<zone>.c.<project>.internal
        global (legacy):     <instance>.c.<project>.internal

    The literal "c" label is the guard: it rejects unrelated names that merely
    end in ".internal". Returns (None, None, None) when the hostname is not a
    GCE internal DNS name, which includes instances given a custom hostname at
    creation. zone is None for the global form.
    """
    parts = fqdn.rstrip(".").split(".")
    if len(parts) < 4 or parts[-1] != "internal" or parts[-3] != "c":
        return None, None, None
    instance, project = parts[0], parts[-2]
    zone = parts[-4] if len(parts) >= 5 else None
    return instance, zone, project


def expected_from_local():
    """Derive what the payload must contain, from purely local sources.

    Returns (aliases, ccrid, note) where aliases is [(label, value), ...].
    This is deliberately the same derivation the proposed Agent-side fix would
    use, so a disagreement here is itself informative.
    """
    fqdn = socket.getfqdn().rstrip(".")
    instance, zone, project = split_internal_dns(fqdn)
    if not project:
        return [], "", f"hostname {fqdn!r} is not a GCE internal DNS name"

    aliases = [("#1 fqdn", fqdn), ("#2 resource-name", f"{instance}.{project}")]
    if zone:
        ccrid = f"//compute.googleapis.com/projects/{project}/zones/{zone}/instances/{instance}"
        return aliases, ccrid, ""
    # The global DNS form carries no zone, so the CCRID cannot be derived and is
    # only checked for being non-empty.
    return aliases, "", "global (zoneless) DNS form: CCRID value not derivable, only checked non-empty"


def classify(payload):
    """Several metadata payload types embed `meta`; only one is the host v5
    payload. Distinguish by top-level keys rather than by the presence of
    `meta`, which would over-match."""
    if "meta" not in payload:
        return None
    if "systemStats" in payload or "gohai" in payload:
        return "v5"
    if "agent_checks" in payload:
        # Carries a cached copy of meta, not a fresh collection: getMeta caches
        # under NoExpiration, so a bad first result leaks into this stream too.
        return "checks"
    return None


def parse_payloads(path, offset=0):
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


def verdict(payload, aliases, ccrid):
    """Judge one payload. Returns (problems, got_aliases, got_ccrid, missing_labels)."""
    meta = payload.get("meta", {})
    got_aliases = meta.get("host_aliases") or []
    got_ccrid = meta.get("ccrid", "")

    problems, missing = [], []
    for label, value in aliases:
        if value not in got_aliases:
            problems.append(f"missing {label} alias {value!r}")
            missing.append(label)

    if not got_ccrid:
        # collect_ccrid defaults to true, and ccrid has omitempty so a failed
        # fetch makes the key vanish entirely rather than appear as "".
        problems.append("ccrid absent")
    elif ccrid and got_ccrid != ccrid:
        problems.append(f"ccrid is {got_ccrid!r}, expected {ccrid!r}")

    return problems, got_aliases, got_ccrid, missing


def signature(missing, aliases):
    """Name the failure shape. The model predicts alias #1 survives while #2
    vanishes; anything else means the model is wrong somewhere."""
    if not missing:
        return "ok"
    if len(missing) == len(aliases):
        return "all-aliases-missing"
    labels = [label for label, _ in aliases]
    if len(aliases) == 2 and missing == [labels[1]]:
        return "resource-name-only"
    return "unexpected:" + ",".join(missing)


def report(rows, aliases, ccrid, judging):
    seen = {}
    bad_v5 = bad_checks = n_v5 = 0

    for ts, payload, kind in rows:
        tag = "host-v5" if kind == "v5" else "checks "
        if kind == "v5":
            n_v5 += 1

        if not judging:
            got = payload.get("meta", {}).get("host_aliases") or []
            print(f"{Y} -- {X} {ts}  {tag}  aliases={got}")
            print(f"            ccrid={payload.get('meta', {}).get('ccrid') or '(absent)'}")
            continue

        problems, got_aliases, got_ccrid, missing = verdict(payload, aliases, ccrid)
        if problems:
            if kind == "v5":
                bad_v5 += 1
                sig = signature(missing, aliases)
                seen[sig] = seen.get(sig, 0) + 1
            else:
                bad_checks += 1
            print(f"{R}FAIL{X} {ts}  {tag}  {problems[0]}")
            for extra in problems[1:]:
                print(f"            {R}also{X} {extra}")
            print(f"            got aliases={got_aliases}")
        else:
            print(f"{G} OK {X} {ts}  {tag}  aliases={got_aliases}")
        print(f"            ccrid={got_ccrid or '(absent)'}")

    if not rows:
        print(f"{Y}no metadata payloads found — are log_payloads:true and "
              f"log_level:debug set, and has a collection happened yet?{X}")
        return 1
    if not judging:
        print(f"\n{Y}'--' means reported but NOT checked (verdicts disabled).{X}")
        return 0

    extra = f" (+{bad_checks} in cached checks payloads)" if bad_checks else ""
    print(f"\n{n_v5} host-v5 payload(s) checked, {bad_v5} problem(s){extra}.")

    if seen:
        print(f"\n{B}failure shape:{X}")
        for sig, count in sorted(seen.items()):
            print(f"  {sig}  x{count}")
        if set(seen) == {"resource-name-only"}:
            print(f"\n{G}Matches the predicted signature{X}: alias #1 survived (warmed during "
                  f"early hostname resolution) while #2 was lost.")
        elif "all-aliases-missing" in seen:
            print(f"\n{Y}Both aliases vanished{X}, so the GCE hostname fetcher was never warm. "
                  f"Expected when IMDS was already unreachable at process start (early "
                  f"hostname resolution failed too), or when an earlier hostname provider "
                  f"(hostname: / hostname_file) short-circuits the chain before gceProvider.\n"
                  f"Note this is a DIFFERENT code path from the reported incident: with no "
                  f"GCE alias at all, cloudprovider stays empty and GetHostCCRID "
                  f"short-circuits at INFO. The reported incident keeps alias #1, so "
                  f"cloudprovider is \"GCP\" and the CCRID instead fails inside ccridFetcher, "
                  f"logged only at DEBUG.")
    return 1 if (bad_v5 or bad_checks) else 0


def iptables(action, args):
    rule = ["iptables", action, "OUTPUT", "-d", IMDS_IP] + args
    res = subprocess.run(rule, capture_output=True, text=True)
    if res.returncode != 0 and action == "-I":
        sys.exit(f"failed to insert iptables rule ({' '.join(rule)}):\n{res.stderr.strip()}")
    return res.returncode == 0


def block_all(enable):
    iptables("-I" if enable else "-D", ["-j", "DROP"])


def block_project_id(enable):
    """Drop only the requests whose payload contains /project/project-id.

    IMDS is plaintext HTTP, so a string match on the request works. This needs
    the xt_string kernel module; the insert is checked so a missing module fails
    loudly instead of silently reproducing nothing.
    """
    iptables("-I" if enable else "-D",
             ["-p", "tcp", "-m", "string", "--string", PROJECT_ID_PATH,
              "--algo", "bm", "-j", "DROP"])


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--log", default=AGENT_LOG)
    ap.add_argument("--scan", action="store_true", help="read-only scan of the existing log")
    ap.add_argument("--restart", action="store_true", help="restart the Agent and evaluate")
    ap.add_argument("--block", type=int, metavar="N",
                    help="with --restart: DROP all traffic to IMDS for N seconds")
    ap.add_argument("--block-project-id", type=int, metavar="N",
                    help="with --restart: block only /project/project-id for N seconds, "
                         "to test whether that one endpoint failing is enough to drop alias #2")
    ap.add_argument("--wait", type=int, default=25, help="seconds to wait for the first payload")
    ap.add_argument("--expect", metavar="ALIAS[,ALIAS...]",
                    help="expected alias(es), comma-separated, overriding local derivation. "
                         "Required when scanning a log copied off another host, since the "
                         "local hostname would describe the wrong machine")
    ap.add_argument("--expect-ccrid", metavar="CCRID",
                    help="expected CCRID value; without it the CCRID is only checked non-empty")
    args = ap.parse_args()

    if not os.path.exists(args.log):
        sys.exit(f"{args.log} not found")
    if args.block and args.block_project_id:
        sys.exit("--block and --block-project-id are mutually exclusive")
    if (args.block or args.block_project_id) and not args.restart:
        sys.exit("--block/--block-project-id only make sense with --restart")

    judging = True
    if args.expect:
        given = [a.strip() for a in args.expect.split(",") if a.strip()]
        aliases = [(f"#{i}", a) for i, a in enumerate(given, 1)]
        ccrid, note = args.expect_ccrid or "", "from --expect"
        print(f"{B}expecting:{X} [{note}]")
    else:
        if not is_gce():
            print(f"{Y}note:{X} DMI does not report Google, so this does not look like a GCE "
                  f"instance. Empty aliases would be correct here, so verdicts are disabled.\n"
                  f"      pass --expect <alias>[,<alias>] to check a log from a GCE host.\n")
            judging = False
        aliases, ccrid, note = expected_from_local()
        if judging and not aliases:
            print(f"{Y}note:{X} {note} — cannot derive the expected aliases locally, so "
                  f"verdicts are disabled.\n      pass --expect to supply them.\n")
            judging = False
        elif judging and note:
            print(f"{Y}note:{X} {note}")

    if judging:
        for label, value in aliases:
            print(f"{B}  expect {label}:{X} {value}")
        print(f"{B}  expect ccrid:{X} {ccrid or '(any non-empty)'}")

    # A local hostname can only describe the local machine; refuse to judge a log
    # that plainly came from elsewhere rather than reporting confident nonsense.
    print(f"{B}log:{X} {args.log}\n")

    if not args.restart:
        rows, _ = parse_payloads(args.log)
        if judging and not args.expect and rows:
            local = socket.gethostname().split(".")[0]
            # Compare against meta.socket-hostname (os.Hostname()), NOT
            # internalHostname: the latter is the Agent hostname and is often
            # overridden via hostname:/DD_HOSTNAME.
            hosts = {p.get("meta", {}).get("socket-hostname", "").split(".")[0]
                     for _, p, _ in rows}
            hosts.discard("")
            if hosts and local not in hosts:
                print(f"{Y}note:{X} log is from {sorted(hosts)}, this machine is {local!r} — "
                      f"verdicts disabled.\n      pass --expect to check a copied log.\n")
                judging = False
        sys.exit(report(rows, aliases, ccrid, judging))

    _, offset = parse_payloads(args.log)  # only look at what comes next
    unblock = None
    try:
        if args.block:
            print(f"{Y}blocking all traffic to {IMDS_IP} for {args.block}s{X}")
            block_all(True)
            unblock, hold = block_all, args.block
        elif args.block_project_id:
            print(f"{Y}blocking {PROJECT_ID_PATH} for {args.block_project_id}s "
                  f"(other endpoints stay reachable){X}")
            block_project_id(True)
            unblock, hold = block_project_id, args.block_project_id

        subprocess.run(["systemctl", "restart", "datadog-agent"], check=True)
        print("agent restarted; waiting for the first payload...")

        if unblock:
            time.sleep(hold)
            unblock(False)
            unblock = None
            print(f"{Y}unblocked{X}")

        deadline = time.time() + args.wait
        rows = []
        while time.time() < deadline:
            rows, _ = parse_payloads(args.log, offset)
            if rows:
                break
            time.sleep(1)
        print()
        sys.exit(report(rows, aliases, ccrid, judging))
    finally:
        if unblock:
            unblock(False)
            print(f"{Y}unblocked (cleanup){X}")


if __name__ == "__main__":
    main()
