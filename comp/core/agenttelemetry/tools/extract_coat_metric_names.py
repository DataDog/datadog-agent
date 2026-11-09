#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
"""
Extract the set of COAT (Cross Org Agent Telemetry) metric names from
defaultProfiles.yaml.

Older agents keep the defaultProfiles.yaml embedded in their binary, so a metric
removed from the current profiles is still reported by deployed agents. To build a
backend allowlist that does not drop anything currently in flight, we need the UNION
of every metric name that ever shipped, not just the names on the current branch.

This walks the full git history of every defaultProfiles.yaml blob and prints the
sorted union of metric names (the `name:` field under each profile's `metric.metrics`).
Event names (under `events:`) are intentionally excluded.

Usage (run from the datadog-agent repo root):
    python3 comp/core/agenttelemetry/tools/extract_coat_metric_names.py            # all history
    python3 comp/core/agenttelemetry/tools/extract_coat_metric_names.py --current  # current file only
"""

import argparse
import re
import subprocess
import sys

PROFILES_SUFFIX = "defaultProfiles.yaml"


def metric_names(text):
    """Return the set of metric names in a defaultProfiles.yaml document.

    Collects `- name: X` entries whose nearest enclosing mapping key is `metrics:`,
    so profile names and event names are not included.
    """
    names = set()
    keystack = []  # (indent, bare-key) for mapping keys with no inline value
    for raw in text.splitlines():
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip())
        s = raw.strip()
        keystack = [(i, k) for (i, k) in keystack if i < indent]
        m = re.match(r"- name:\s*(\S+)", s)
        if m:
            enclosing = keystack[-1][1] if keystack else None
            if enclosing == "metrics":
                names.add(m.group(1).strip().strip("\"'"))
            continue
        km = re.match(r"([A-Za-z0-9_]+):\s*$", s)
        if km:
            keystack.append((indent, km.group(1)))
    return names


def _git(args, check=True):
    return subprocess.run(["git", *args], capture_output=True, text=True, check=check)


def all_history_blobs():
    """Return the set of unique blob hashes for every defaultProfiles.yaml in history.

    Uses `--missing=allow-any` so the enumeration does not abort on a partial or
    blobless clone, where some historical trees/blobs are not present locally. Blobs
    that are genuinely missing are simply skipped by the reader, so a partial clone
    yields only the names it can reach; run against a full clone for the complete set.
    """
    r = _git(["rev-list", "--all", "--objects", "--missing=allow-any"], check=False)
    if r.returncode != 0:
        # Fall back to the file's own commit history, which touches far fewer objects.
        r = _git(["log", "--all", "--objects", "--format=", "--", f"*{PROFILES_SUFFIX}"], check=False)
    blobs = set()
    for line in r.stdout.splitlines():
        parts = line.split(" ", 1)
        if len(parts) == 2 and parts[1].endswith(PROFILES_SUFFIX):
            blobs.add(parts[0])
    return sorted(blobs)


def main():
    parser = argparse.ArgumentParser(description="Extract COAT metric names from defaultProfiles.yaml")
    parser.add_argument(
        "--current",
        action="store_true",
        help="only parse the working-tree file instead of the full git history",
    )
    parser.add_argument(
        "--path",
        default="comp/core/agenttelemetry/impl/defaultProfiles.yaml",
        help="path to defaultProfiles.yaml (used with --current)",
    )
    args = parser.parse_args()

    if args.current:
        with open(args.path, encoding="utf-8") as f:
            names = metric_names(f.read())
    else:
        names = set()
        blobs = all_history_blobs()
        scanned = 0
        for blob in blobs:
            r = _git(["cat-file", "blob", blob], check=False)
            if r.returncode != 0:
                continue  # object not present in a partial clone
            names |= metric_names(r.stdout)
            scanned += 1
        print(f"# scanned {scanned}/{len(blobs)} historical blobs", file=sys.stderr)

    print(f"# {len(names)} distinct metric names", file=sys.stderr)
    for name in sorted(names):
        print(name)


if __name__ == "__main__":
    main()
