from __future__ import annotations

import json
import sys
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path

from invoke import task

from tasks.flavor import AgentFlavor
from tasks.libs.common.color import color_message
from tasks.libs.common.gomodules import AGENT_MODULE_PATH_PREFIX

_IMPORT_PREFIX = AGENT_MODULE_PATH_PREFIX.rstrip("/")


def _label_to_import_path(label: str) -> str:
    pkg_part = label.lstrip("/").split(":", 1)[0]
    return _IMPORT_PREFIX if not pkg_part else f"{_IMPORT_PREFIX}/{pkg_part}"


def _parse_bep_cache_status(bep_path: Path) -> dict[str, bool]:
    """Return a map of import_path → was_cached parsed from a BEP JSON file."""
    status: dict[str, bool] = {}
    with bep_path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            tr = event.get("testResult")
            if not tr:
                continue
            label = event.get("id", {}).get("testResult", {}).get("label", "")
            if not label:
                continue
            import_path = _label_to_import_path(label)
            cached = bool(tr.get("cachedLocally") or tr.get("executionInfo", {}).get("cachedRemotely"))
            status[import_path] = cached
    return status


def _annotate_junit_cache_status(xml_path: Path, cache_status: dict[str, bool]) -> None:
    """Add a bazel.cached <property> to each <testsuite> whose import path is known.

    gotestsum emits one <testsuite> per test function with name "{import_path}.{TestFunc}",
    so the import path is recovered by stripping the final ".FunctionName" component.
    """
    if not cache_status:
        return
    tree = ET.parse(xml_path)
    root = tree.getroot()
    for ts in root.findall(".//testsuite"):
        ts_name = ts.get("name", "")
        cached = cache_status.get(ts_name)
        if cached is None:
            dot = ts_name.rfind(".")
            if dot > 0:
                cached = cache_status.get(ts_name[:dot])
        if cached is None:
            continue
        props = ts.find("properties")
        if props is None:
            props = ET.Element("properties")
            ts.insert(0, props)
        ET.SubElement(props, "property", name="bazel.cached", value=str(cached).lower())
    tree.write(str(xml_path))


def _get_testlogs_dir(ctx) -> Path:
    # `bazel info bazel-testlogs` does not account for configuration transitions,
    # so locate the directory from output_path instead.
    output_path = Path(ctx.run("bazel info output_path", hide=True).stdout.strip())
    candidates = sorted(output_path.glob("*/testlogs"), key=lambda p: p.stat().st_mtime, reverse=True)
    if not candidates:
        raise RuntimeError(f"no testlogs directory found under {output_path}")
    if len(candidates) > 1:
        others = [str(c) for c in candidates[1:]]
        print(
            color_message("warning", "yellow")
            + f": multiple testlogs directories found, using most recent: {candidates[0]}\n  others: {others}",
            file=sys.stderr,
        )
    return candidates[0]


@task
def testlogs_dir(ctx):
    """Print the absolute path to Bazel's testlogs directory."""
    print(_get_testlogs_dir(ctx))


@task(
    help={
        "flavor": f"Agent flavor ({', '.join(f.name for f in AgentFlavor)}). Embedded in each JUnit XML.",
        "output_tgz": "Destination path for the output tgz (e.g. junit-bazel-base.tgz).",
        "bep_file": "Path to a Bazel BEP JSON file; when provided, annotates each testsuite with bazel.cached.",
    },
)
def collect_junit(ctx, flavor, output_tgz, bep_file=None):
    """Collect Bazel test results and package them for junit_upload.

    Merges the test.xml files produced by the rules_go test runner (one per
    test target) into a single JUnit XML, then packages it into a tgz compatible
    with the existing junit_upload machinery (same format as --junit-tar from
    dda inv test).
    """
    from tasks.libs.common.junit_upload_core import enrich_junitxml, produce_junit_tar

    tl_dir = _get_testlogs_dir(ctx)

    xml_files = [p for p in tl_dir.rglob("test.xml") if p.is_file()]
    if not xml_files:
        print(f"error: no test.xml files found under {tl_dir}", file=sys.stderr)
        sys.exit(1)

    agent_flavor = AgentFlavor[flavor]

    with tempfile.TemporaryDirectory() as tmpdir:
        merged = ET.Element("testsuites")
        collected = 0
        for xml_path in xml_files:
            try:
                file_root = ET.parse(xml_path).getroot()
            except ET.ParseError:
                continue
            suites = (
                list(file_root)
                if file_root.tag == "testsuites"
                else [file_root]
                if file_root.tag == "testsuite"
                else []
            )
            for ts in suites:
                if int(ts.get("tests", "0")) == 0:
                    continue
                merged.append(ts)
                collected += 1

        if collected == 0:
            print(
                f"error: no test suites found (all {len(xml_files)} test.xml files had 0 tests)",
                file=sys.stderr,
            )
            sys.exit(1)

        merged_path = Path(tmpdir) / f"junit-bazel-{flavor}.xml"
        ET.ElementTree(merged).write(str(merged_path), encoding="unicode")

        enrich_junitxml(str(merged_path), agent_flavor)

        if bep_file:
            bep_path = Path(bep_file)
            if bep_path.is_file():
                _annotate_junit_cache_status(merged_path, _parse_bep_cache_status(bep_path))
            else:
                print(f"warning: BEP file not found: {bep_file}", file=sys.stderr)

        produce_junit_tar([str(merged_path)], output_tgz)

    print(f"Packaged {collected} test suites → {output_tgz}")
