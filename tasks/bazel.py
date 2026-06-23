from __future__ import annotations

import json
import os
import platform
import re
import subprocess
import sys
import xml.etree.ElementTree as ET
from datetime import datetime
from pathlib import Path

from invoke import task

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.flavor import AgentFlavor
from tasks.libs.common.gomodules import AGENT_MODULE_PATH_PREFIX

REPO_ROOT = Path(__file__).parent.parent
# Top-level Test* declarations in Go source files — Go side of the parity comparison.
_TEST_FUNC_RE = re.compile(r'^func (Test\w+)\(', re.MULTILINE)
_IMPORT_PREFIX = AGENT_MODULE_PATH_PREFIX.rstrip("/")
# Tag the dd_agent_go_test macro stamps on every variant it emits
# (bazel/rules/go/dd_agent_go_test.bzl). Used to distinguish its generated
# go_test rules from other custom wrappers (rtloader_go_test, ...).
_DD_AGENT_GO_TEST_TAG = "dd_agent_go_test"


def _go_test_packages(tags: list[str], import_paths: set[str]) -> dict[str, set[str]]:
    """Return {import_path: {Test* func names}} for the given import paths
    compiled under the given build tags."""
    if not import_paths:
        return {}
    # shell=False is required: cmd.exe on Windows has an 8191-char command-line limit.
    result = subprocess.run(
        ["go", "list", "-json", "-e", f"-tags={','.join(sorted(tags))}", *sorted(import_paths)],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        encoding="utf-8",
    )
    pkgs: dict[str, set[str]] = {}
    decoder = json.JSONDecoder()
    text, pos = result.stdout, 0
    while pos < len(text):
        try:
            obj, end = decoder.raw_decode(text, pos)
        except json.JSONDecodeError:
            break
        pos = end
        while pos < len(text) and text[pos].isspace():
            pos += 1
        import_path = obj.get("ImportPath", "")
        if import_path not in import_paths:
            continue
        pkg_dir = Path(obj.get("Dir", ""))
        test_files = obj.get("TestGoFiles", []) + obj.get("XTestGoFiles", [])
        funcs: set[str] = set()
        for f in test_files:
            try:
                funcs.update(_TEST_FUNC_RE.findall((pkg_dir / f).read_text()))
            except OSError:
                continue
        # TestMain is Go's test harness entry point, never dispatched as a test case.
        funcs.discard("TestMain")
        if funcs:
            pkgs[import_path] = funcs
    return pkgs


def _label_to_import_path(label: str) -> str:
    """Convert a Bazel label like '//pkg/util/kernel:kernel_test_iot' into the
    Go import path of the package the test lives in."""
    pkg_part = label.lstrip("/").split(":", 1)[0]
    return _IMPORT_PREFIX if not pkg_part else f"{_IMPORT_PREFIX}/{pkg_part}"


def _test_xml_candidates(
    label: str,
    uri: str,
    cfg_id: str,
    local_exec_root: str | None,
    config_testlogs: dict[str, Path],
) -> list[Path]:
    """Candidate paths for test.xml, in priority order.

    BEP URIs are file:// for local actions and bytestream:// for remote-cache
    hits; for the latter Bazel still materializes test.xml on disk at
    <localExecRoot>/<testlogs-dir>/<label>/test.xml.
    """
    paths: list[Path] = []
    if uri.startswith("file://"):
        paths.append(Path(uri.removeprefix("file://")))
    testlogs_dir = config_testlogs.get(cfg_id)
    if local_exec_root and testlogs_dir:
        # Label "//pkg/foo:bar_test" -> "pkg/foo/bar_test/test.xml".
        label_rel = label.lstrip("/").replace(":", "/")
        paths.append(Path(local_exec_root) / testlogs_dir / label_rel / "test.xml")
    return paths


def _test_xml_funcs(paths: list[Path]) -> set[str]:
    """Return top-level Test* functions from the first readable test.xml.

    Subtests appear as name="TestFoo/Sub"; filtering to names without '/'
    gives top-level functions. Requires --test_arg=-test.v (or
    --test_env=GO_TEST_WRAP_TESTV=1) for a complete XML.
    """
    for path in paths:
        try:
            content = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        if not content.strip():
            continue
        try:
            root = ET.fromstring(content)
        except ET.ParseError:
            continue
        funcs = {
            tc.get("name", "")
            for tc in root.iter("testcase")
            if tc.get("name", "").startswith("Test") and "/" not in tc.get("name", "")
        }
        return funcs
    return set()


def _bazel_test_funcs_from_bep(bep_path: Path) -> dict[str, set[str]]:
    """Parse a Build Event Protocol JSON stream into {import_path: {Test* funcs}}.

    Selects dd_agent_go_test targets that had a testResult event and whose
    test.xml records at least one Test* function. Plain go_test rules (no
    `dd_agent_go_test` tag) are ignored.
    """
    dd_agent_labels: set[str] = set()
    # (uri, config_id) per label so we can recover test.xml even when Bazel
    # writes only a bytestream:// URI to the BEP. The convenience symlink
    # `bazel-testlogs` doesn't exist on CI (--noexperimental_convenience_symlinks),
    # so we reconstruct the absolute path from `localExecRoot` and the
    # configuration's BINDIR.
    test_action: dict[str, tuple[str, str]] = {}
    local_exec_root: str | None = None
    config_testlogs: dict[str, Path] = {}

    with open(bep_path) as fh:
        for line in fh:
            if not line.strip():
                continue
            event = json.loads(line)
            eid = event.get("id", {})
            if "workspace" in eid:
                local_exec_root = event.get("workspaceInfo", {}).get("localExecRoot")
            elif "configuration" in eid:
                cfg_id = eid["configuration"].get("id", "")
                bindir = event.get("configuration", {}).get("makeVariable", {}).get("BINDIR", "")
                # BINDIR is "bazel-out/<config-mnemonic>/bin"; testlogs lives
                # alongside as "bazel-out/<config-mnemonic>/testlogs".
                bindir_path = Path(bindir)
                if bindir_path.name == "bin":
                    config_testlogs[cfg_id] = bindir_path.parent / "testlogs"
            elif "targetConfigured" in eid:
                label = eid["targetConfigured"].get("label", "")
                cfg = event.get("configured", {})
                if cfg.get("targetKind") != "go_test rule":
                    continue
                tags = set(cfg.get("tag") or [])
                if _DD_AGENT_GO_TEST_TAG in tags:
                    dd_agent_labels.add(label)
            elif "testResult" in eid:
                label = eid["testResult"].get("label", "")
                cfg_id = eid["testResult"].get("configuration", {}).get("id", "")
                for out in event["testResult"].get("testActionOutput") or []:
                    if out.get("name") == "test.xml":
                        test_action[label] = (out.get("uri", ""), cfg_id)
                        break

    covered: dict[str, set[str]] = {}
    for label in dd_agent_labels:
        action = test_action.get(label)
        if action is None:
            continue
        uri, cfg_id = action
        funcs = _test_xml_funcs(_test_xml_candidates(label, uri, cfg_id, local_exec_root, config_testlogs))
        covered[_label_to_import_path(label)] = funcs

    return covered


def _emit_test_count_metric(flavor: str, count: int) -> None:
    """Send a datadog.agent.bazel_tests.executed gauge for one flavor job."""
    from tasks.libs.common.datadog_api import create_gauge
    from tasks.libs.common.datadog_api import send_metrics as _send_metrics

    if not os.environ.get("DD_API_KEY"):
        print("DD_API_KEY not set, skipping test count metric", file=sys.stderr)
        return

    tags = [
        f"flavor:{flavor}",
        f"platform:{platform.system().lower()}",
        f"pipeline_id:{os.environ.get('CI_PIPELINE_ID', 'unknown')}",
        "repository:datadog-agent",
    ]
    timestamp = int(datetime.now().timestamp())
    _send_metrics([create_gauge("datadog.agent.bazel_tests.executed", timestamp, count, tags)], warn=True)
    print(f"Sent metric: datadog.agent.bazel_tests.executed={count} (flavor={flavor})")


@task(
    help={
        "flavor_name": f"Agent flavor to check ({', '.join(f.name for f in AgentFlavor)}).",
        "bep": "Path to the build_event_json_file produced by the preceding "
        "'bazel test --config=<flavor> ...' invocation.",
        "verbose": "Print passing packages.",
        "emit_metrics": "Send a datadog.agent.bazel_tests.executed gauge to Datadog (requires DD_API_KEY).",
    },
)
def ensure_test_parity(ctx, bep, flavor_name, verbose=False, emit_metrics=False):
    """
    Verify every Go test visible to 'dda inv test --flavor=<f>' is also
    present and executed in the matching Bazel per-flavor run.

    Reads test execution from a Bazel Build Event Protocol JSON stream
    (--build_event_json_file). The BEP determines scope (dd_agent_go_test
    packages that produced test results); 'go list' is then queried for just
    those packages to obtain the expected Test* function set. A package
    compiled with wrong build tags (running a different set of functions) is
    also reported as a failure.

    Exits 1 if any gap is found.
    """
    bep_path = Path(bep)
    if not bep_path.is_file():
        print(f"error: BEP file not found: {bep}", file=sys.stderr)
        sys.exit(2)

    try:
        flavor = AgentFlavor[flavor_name]
    except KeyError:
        print(f"Unknown flavor '{flavor_name}'. Options: {[f.name for f in AgentFlavor]}", file=sys.stderr)
        sys.exit(1)

    tags = compute_build_tags_for_flavor("unit-tests", None, None, flavor)
    coverage = _bazel_test_funcs_from_bep(bep_path)
    test_pkgs = _go_test_packages(tags, set(coverage))

    go_pkgs = set(test_pkgs)
    extra_in_bazel = {p for p, funcs in coverage.items() if funcs} - go_pkgs

    failed = False
    test_count = 0
    for import_path in sorted(go_pkgs):
        go_funcs = test_pkgs[import_path]
        bazel_funcs = coverage[import_path]
        missing_funcs = go_funcs - bazel_funcs
        test_count += len(bazel_funcs)
        if missing_funcs:
            sample = ", ".join(sorted(missing_funcs)[:3]) + (", ..." if len(missing_funcs) > 3 else "")
            print(f"[FAIL] {import_path} [{flavor_name}] -- tests missing from Bazel: {sample}")
            failed = True
        elif verbose:
            print(f"[PASS] {import_path} [{flavor_name}] ({len(bazel_funcs)} tests)")
    for import_path in sorted(extra_in_bazel):
        print(f"[FAIL] {import_path} [{flavor_name}] -- Bazel target exists but not in dda inv test")
    if extra_in_bazel:
        failed = True

    if emit_metrics:
        _emit_test_count_metric(flavor_name, test_count)
    if failed:
        sys.exit(1)
