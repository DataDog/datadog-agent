from __future__ import annotations

import sys
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path

from invoke import task

from tasks.flavor import AgentFlavor
from tasks.libs.common.color import color_message
from tasks.libs.common.gomodules import AGENT_MODULE_PATH_PREFIX

_IMPORT_PREFIX = AGENT_MODULE_PATH_PREFIX.rstrip("/")


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
    },
)
def collect_junit(ctx, flavor, output_tgz):
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
        produce_junit_tar([str(merged_path)], output_tgz)

    print(f"Packaged {collected} test suites → {output_tgz}")
