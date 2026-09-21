#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Diagnostics for the CI 'link: open ... packages.txt: no such file' failures.

Two subcommands, used by the bazel:test:linux-amd64-no-cache CI job:

  graph-check  — parse a `bazel aquery --output=text` dump and report, for the
                 SDK's packages.txt artifacts, which actions GENERATE them vs
                 which CONSUME them. Flags any consumed path that has no
                 generating action ("DEAD REFERENCE"): that is the analysis-
                 time signature of the missing-file failure (a consumer holding
                 a File whose producing action never materialized in that
                 configuration).

  execlog      — parse a `--execution_log_json_file` dump (one JSON object per
                 line) and classify the actions that mention packages.txt as
                 producers or consumers, plus dump one raw sample entry so the
                 exact schema can be inspected even if Bazel changes it.
"""

from __future__ import annotations

import argparse
import json
import re
import sys

PKGS_RE = re.compile(r"\S*go_work_sdk/packages\.txt")
MNEMONIC_RE = re.compile(r"^\s*Mnemonic:\s+(\S+)")


def _action_blocks(text: str):
    for block in re.split(r"(?=Mnemonic: )", text):
        m = MNEMONIC_RE.match(block)
        if not m:
            continue
        yield m.group(1), block


def graph_check(path: str, label: str) -> int:
    text = open(path, encoding="utf-8", errors="replace").read()
    generated = {}
    consumed = {}
    for mnemonic, block in _action_blocks(text):
        for o in re.findall(r"Outputs: \[([^\]]*)\]", block, re.S):
            for p in PKGS_RE.findall(o):
                generated.setdefault(p, set()).add(mnemonic)
        for i in re.findall(r"Inputs: \[([^\]]*)\]", block, re.S):
            for p in PKGS_RE.findall(i):
                consumed.setdefault(p, set()).add(mnemonic)

    print(f"== {label}")
    for p, m in generated.items():
        print(f"   GENERATED at {p} by {sorted(m)}")
    for p, m in consumed.items():
        print(f"   CONSUMED at {p} by {sorted(m)}")

    dead = [p for p in consumed if p not in generated]
    for p in dead:
        print(f"   *** DEAD REFERENCE (consumed but never generated): {p} ***")
    if not dead and generated:
        print("   (all consumed paths have generators)")
    # Also report generators nobody consumes (split-brain in the other direction).
    for p in generated:
        if p not in consumed:
            print(f"   (orphan: generated at {p} but never consumed)")
    return 1 if dead else 0


def _classify(entry: dict) -> str:
    """Best-effort producer/consumer classification across schema variants."""
    for key in ("outputFiles", "output_paths", "outputs"):
        v = entry.get(key)
        if v and "go_work_sdk/packages.txt" in json.dumps(v):
            return "PRODUCER"
    # Fallback: if the path appears anywhere besides input-ish fields, unknown.
    return "CONSUMER?"


def execlog(path: str) -> int:
    try:
        fh = open(path, encoding="utf-8", errors="replace")
    except FileNotFoundError:
        print("execution log not found")
        return 2
    n = 0
    producers = []
    consumers = []
    sample_printed = False
    for line in fh:
        if "go_work_sdk/packages.txt" not in line:
            continue
        n += 1
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        argv = entry.get("commandArgs") or entry.get("arguments") or []
        head = " ".join(argv[:3]) if isinstance(argv, list) else str(argv)[:100]
        role = _classify(entry)
        (producers if role == "PRODUCER" else consumers).append(head)
        if not sample_printed:
            print("sample raw entry (schema reference):")
            print(line[:2000])
            sample_printed = True
    print(f"execution log entries mentioning packages.txt: {n}")
    print("PRODUCERS:")
    for p in producers[:10]:
        print(f"    {p}")
    print("CONSUMERS:")
    for c in consumers[:10]:
        print(f"    {c}")
    if n and not producers:
        print("*** NO PRODUCER ran for packages.txt in this invocation ***")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    g = sub.add_parser("graph-check")
    g.add_argument("aquery_file")
    g.add_argument("label")
    e = sub.add_parser("execlog")
    e.add_argument("execution_log")
    args = ap.parse_args()
    if args.cmd == "graph-check":
        return graph_check(args.aquery_file, args.label)
    return execlog(args.execution_log)


if __name__ == "__main__":
    sys.exit(main())
