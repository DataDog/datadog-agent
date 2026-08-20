from __future__ import annotations

import json
import os
import sys
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import TypedDict

from invoke import task

from tasks.libs.build.bazel import bazel
from tasks.libs.common.gomodules import AGENT_MODULE_PATH_PREFIX

_IMPORT_PREFIX = AGENT_MODULE_PATH_PREFIX.rstrip("/")


def _label_to_import_path(label: str) -> str:
    """Convert a Bazel label like '//pkg/util/kernel:kernel_test_iot' into the
    Go import path of the package the test lives in."""
    pkg_part = label.lstrip("/").split(":", 1)[0]
    return _IMPORT_PREFIX if not pkg_part else f"{_IMPORT_PREFIX}/{pkg_part}"


def _test_output_candidates(
    label: str,
    uri: str,
    cfg_id: str,
    local_exec_root: str | None,
    config_testlogs: dict[str, Path],
    output_name: str,
) -> list[Path]:
    """Candidate paths for a Bazel test output, in priority order.

    BEP URIs are file:// for local actions and bytestream:// for remote-cache
    hits; for the latter Bazel often still materializes outputs on disk at
    <localExecRoot>/<testlogs-dir>/<label>/<output_name>.
    """
    paths: list[Path] = []
    if uri.startswith("file://"):
        paths.append(Path(uri.removeprefix("file://")))
    testlogs_dir = config_testlogs.get(cfg_id)
    if local_exec_root and testlogs_dir:
        # Label "//pkg/foo:bar_test" -> "pkg/foo/bar_test/<output_name>".
        label_rel = label.lstrip("/").replace(":", "/")
        paths.append(Path(local_exec_root) / testlogs_dir / label_rel / output_name)
    return paths


class _BepContext:
    """Tracks the workspace/configuration state needed to resolve BEP test.xml paths.

    The convenience symlink `bazel-testlogs` doesn't exist on CI
    (--noexperimental_convenience_symlinks), so test.xml paths are
    reconstructed from `localExecRoot` and each configuration's BINDIR
    instead ("bazel-out/<config-mnemonic>/bin" -> ".../testlogs").
    """

    def __init__(self):
        self.local_exec_root: str | None = None
        self.config_testlogs: dict[str, Path] = {}

    def observe(self, eid: dict, event: dict) -> bool:
        """Update state from a workspace/configuration event. Returns True if handled."""
        match eid:
            case {"workspace": _}:
                self.local_exec_root = event.get("workspaceInfo", {}).get("localExecRoot")
                return True
            case {"configuration": {"id": cfg_id}}:
                bindir = event.get("configuration", {}).get("makeVariable", {}).get("BINDIR", "")
                bindir_path = Path(bindir)
                if bindir_path.name == "bin":
                    self.config_testlogs[cfg_id] = bindir_path.parent / "testlogs"
                return True
            case _:
                return False


class BepTestArtifacts(TypedDict):
    cached: bool
    xml_paths: list[Path]  # noqa: F841 - TypedDict field, not a local variable
    log_paths: list[Path]  # noqa: F841 - TypedDict field, not a local variable


def _resolve_test_output_path(
    label: str,
    uri: str,
    cfg_id: str,
    ctx: _BepContext,
    output_name: str,
) -> Path:
    candidates = _test_output_candidates(label, uri, cfg_id, ctx.local_exec_root, ctx.config_testlogs, output_name)
    for candidate in candidates:
        if candidate.is_file():
            return candidate
    raise RuntimeError(
        f"Could not resolve {output_name} for {label}. "
        f"uri={uri!r}, cfg_id={cfg_id!r}, candidates={[str(p) for p in candidates]}"
    )


def _parse_bep(bep_path: Path) -> dict[str, BepTestArtifacts]:
    """Parse a BEP JSON file in one pass.

    Returns a mapping keyed by Bazel test label. Each value contains the cache
    status and the test.xml/test.log files produced by this invocation.

    The label key preserves distinct dd_agent_go_test variants for the same Go
    package, such as //pkg/foo:foo_test and //pkg/foo:foo_test_containerd.
    """
    ctx = _BepContext()
    test_actions: list[tuple[str, str, str, str, bool]] = []

    with bep_path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            eid = event.get("id", {})
            if ctx.observe(eid, event):
                continue
            match eid:
                case {"testResult": {"label": label}} if label:
                    tr = event.get("testResult", {})
                    cfg_id = eid["testResult"].get("configuration", {}).get("id", "")
                    output_uris = {
                        output.get("name"): output.get("uri", "")
                        for output in tr.get("testActionOutput", [])
                        if output.get("name")
                    }
                    missing_outputs = [name for name in ("test.xml", "test.log") if name not in output_uris]
                    if missing_outputs:
                        raise RuntimeError(
                            f"BEP testResult for {label} did not include {missing_outputs}; "
                            f"available outputs: {sorted(output_uris)}"
                        )
                    cached = bool(tr.get("cachedLocally") or tr.get("executionInfo", {}).get("cachedRemotely"))
                    test_actions.append((label, cfg_id, output_uris["test.xml"], output_uris["test.log"], cached))

    results: dict[str, BepTestArtifacts] = {}
    for label, cfg_id, xml_uri, log_uri, cached in test_actions:
        artifacts = results.setdefault(
            label,
            {"cached": cached, "xml_paths": [], "log_paths": []},
        )
        artifacts["cached"] = cached
        artifacts["xml_paths"].append(_resolve_test_output_path(label, xml_uri, cfg_id, ctx, "test.xml"))
        artifacts["log_paths"].append(_resolve_test_output_path(label, log_uri, cfg_id, ctx, "test.log"))
    return results


def _is_gotestsum_shaped(suite: ET.Element) -> bool:
    """True if every testcase in this testsuite has a classname attribute.

    Bazel synthesizes a minimal single-testcase XML (no classname) for test
    rules that don't emit their own JUnit report (diff_test, sh_test, rust
    tests, ...); downstream JUnit processing assumes gotestsum's schema,
    where classname is always present.
    """
    return all("classname" in tc.attrib for tc in suite.iter("testcase"))


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


def _collect_junit(test_artifacts, output_tgz):
    """Collect Bazel test results and package them for junit_upload.

    Merges the test.xml files produced by the rules_go test runner (one per
    test target) into a single JUnit XML, then packages it into a tgz compatible
    with the existing junit_upload machinery (same format as --junit-tar from
    dda inv test).
    """
    from tasks.libs.common.junit_upload_core import produce_junit_tar

    xml_files = [p for artifacts in test_artifacts.values() for p in artifacts["xml_paths"]]
    cache_status = {_label_to_import_path(label): artifacts["cached"] for label, artifacts in test_artifacts.items()}
    if not xml_files:
        print("error: no test.xml files found in BEP output", file=sys.stderr)
        sys.exit(1)

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
                if not _is_gotestsum_shaped(ts):
                    continue
                merged.append(ts)
                collected += 1

        if collected == 0:
            print(
                f"error: no test suites found (all {len(xml_files)} test.xml files had 0 tests)",
                file=sys.stderr,
            )
            sys.exit(1)

        merged_path = Path(tmpdir) / "junit-bazel.xml"
        ET.ElementTree(merged).write(str(merged_path), encoding="unicode")

        if cache_status:
            _annotate_junit_cache_status(merged_path, cache_status)

        produce_junit_tar([str(merged_path)], output_tgz)

    print(f"Packaged {collected} test suites → {output_tgz}")


def _collect_test2json(ctx, test_artifacts, output_path):
    with tempfile.TemporaryDirectory() as tmpdir:
        manifest_path = os.path.join(tmpdir, "manifest.tsv")
        with open(manifest_path, "w") as manifest:
            for label in sorted(test_artifacts.keys()):
                # Use the Bazel label, not the Go import path, as the test2json package
                # identity. dd_agent_go_test emits multiple configured go_test targets
                # for different build-tag sets in the same Go package; collapsing them
                # to one import path would make UTOF report those distinct runs as
                # retries.
                manifest.writelines(f'{label}\t{log_path}\n' for log_path in test_artifacts[label]["log_paths"])

        bazel(
            "run",
            "--config=gorace",  # to use same analysis cache across test & run commands
            "//bazel/tools/testlogs_to_json",
            "--",
            "-manifest",
            manifest_path,
            "-output",
            os.path.abspath(output_path),
        )


@task(
    help={
        "bep_file": "Path to a Bazel BEP JSON file (--build_event_json_file) used to gather all necessary data.",
        "result_json": "Path to write test2json JSONL output.",
        "junit_tar": "Path to write the JUnit tgz.",
    },
)
def process_test_results(ctx, bep_file, result_json="test_output.json", junit_tar=""):
    """Collect results from Bazel-run tests and produce various artifacts.

    This task:
    - Produces a tgz JUnit XML file compatible with our existing upload machinery.
    - Produces a test2json file with test results and a UTOF json file created from it.
    - Displays test results in a human-friendly way (based on UTOF).
    """
    # BEP is the authoritative source: it lists exactly the test.xml and test.log files
    # produced by this invocation, avoiding stale results from previous runs
    # with a different Bazel configuration.
    test_artifacts = _parse_bep(Path(bep_file))

    # Produce the test2json result file
    _collect_test2json(ctx, test_artifacts, result_json)

    # Produce UTOF and associated terminal output
    from tasks.libs.testing.utof.go.generate import generate_unified_output

    generate_unified_output(ctx, result_json, "bazel", "")

    # Produce the junit tar
    if junit_tar:
        _collect_junit(test_artifacts, junit_tar)
