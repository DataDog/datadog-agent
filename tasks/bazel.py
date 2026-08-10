from __future__ import annotations

import json
import os
import platform
import re
import sys
import tempfile
import xml.etree.ElementTree as ET
from datetime import datetime
from pathlib import Path
from typing import TypedDict

from invoke import task

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.flavor import AgentFlavor
from tasks.libs.build.bazel import bazel
from tasks.libs.common.gomodules import AGENT_MODULE_PATH_PREFIX

# Top-level Test* declarations in Go source files — Go side of the parity comparison.
_TEST_FUNC_RE = re.compile(r'^func (Test\w*)\(', re.MULTILINE)
# Matches json.decoder.WHITESPACE, an undocumented/unstubbed cpython internal.
_JSON_WHITESPACE_RE = re.compile(r"[ \t\n\r]*")
_IMPORT_PREFIX = AGENT_MODULE_PATH_PREFIX.rstrip("/")
# Tag the dd_agent_go_test macro stamps on every variant it emits
# (bazel/rules/go/dd_agent_go_test.bzl). Used to distinguish its generated
# go_test rules from other custom wrappers (rtloader_go_test, ...).
_DD_AGENT_GO_TEST_TAG = "dd_agent_go_test"


# cmd.exe on Windows caps command lines at 8191 chars; stay safely under that
# per 'go list' invocation so the command line doesn't grow unbounded with
# the tracked package set.
_MAX_CMDLINE_CHARS = 6000


def _chunk_import_paths(import_paths: list[str]) -> list[list[str]]:
    """Split import paths into batches that keep each 'go list' command line
    under the Windows cmd.exe length limit."""
    chunks: list[list[str]] = []
    current: list[str] = []
    current_len = 0
    for path in import_paths:
        if current and current_len + len(path) + 1 > _MAX_CMDLINE_CHARS:
            chunks.append(current)
            current, current_len = [], 0
        current.append(path)
        current_len += len(path) + 1
    if current:
        chunks.append(current)
    return chunks


def _go_test_packages(ctx, tags: list[str], import_paths: set[str]) -> dict[str, set[str]]:
    """Return {import_path: {Test* func names}} for the given import paths
    compiled under the given build tags."""
    if not import_paths:
        return {}
    pkgs: dict[str, set[str]] = {}
    decoder = json.JSONDecoder()
    for chunk in _chunk_import_paths(sorted(import_paths)):
        text = bazel(
            ctx,
            "run",
            "--@rules_go//go/config:race",  # avoid thrashing the analysis cache right after `bazel test`
            "//:go",
            "--",
            "list",
            "-json",
            "-e",
            f"-tags={','.join(sorted(tags))}",
            *chunk,
            capture_output=True,
        )
        pos = 0
        while pos < len(text):
            pos = _JSON_WHITESPACE_RE.match(text, pos).end()
            if pos >= len(text):
                break
            obj, pos = decoder.raw_decode(text, pos)
            import_path = obj["ImportPath"]
            if import_path not in import_paths:
                continue
            pkg_dir = Path(obj["Dir"])
            test_files = obj.get("TestGoFiles", []) + obj.get("XTestGoFiles", [])
            funcs: set[str] = set()
            for f in test_files:
                funcs.update(_TEST_FUNC_RE.findall((pkg_dir / f).read_text()))
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
        root = ET.fromstring(content)
        return {
            tc.attrib["name"]
            for tc in root.iter("testcase")
            if tc.attrib["name"].startswith("Test") and "/" not in tc.attrib["name"]
        }
    raise FileNotFoundError(f"no readable test.xml found among candidates: {paths}")


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


def _bazel_test_funcs_from_bep(bep_path: Path) -> dict[str, set[str]]:
    """Parse a Build Event Protocol JSON stream into {import_path: {Test* funcs}}.

    Selects dd_agent_go_test targets that had a testResult event and whose
    test.xml records at least one Test* function. Plain go_test rules (no
    `dd_agent_go_test` tag) are ignored.
    """
    dd_agent_labels: set[str] = set()
    test_action: dict[str, tuple[str, str]] = {}
    ctx = _BepContext()

    with open(bep_path) as fh:
        for line in fh:
            if not line.strip():
                continue
            event = json.loads(line)
            eid = event.get("id", {})
            if ctx.observe(eid, event):
                continue
            if "targetConfigured" in eid:
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
        if label not in test_action:
            continue
        uri, cfg_id = test_action[label]
        funcs = _test_xml_funcs(
            _test_output_candidates(label, uri, cfg_id, ctx.local_exec_root, ctx.config_testlogs, "test.xml")
        )
        covered.setdefault(_label_to_import_path(label), set()).update(funcs)

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
    try:
        _send_metrics([create_gauge("datadog.agent.bazel_tests.executed", timestamp, count, tags)])
    except Exception as e:
        print(f"Failed to send test count metric: {e}", file=sys.stderr)
        return
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

    # bazel test runs systematically pass the "race" build tag, so files guarded
    # by `!race` must be excluded from the expected test set too.
    tags = [*compute_build_tags_for_flavor("unit-tests", None, None, flavor), "race"]
    coverage = _bazel_test_funcs_from_bep(bep_path)
    test_pkgs = _go_test_packages(ctx, tags, set(coverage))

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

    Returns a mapping keyed by Go import path. Each value contains the cache
    status and the test.xml/test.log files produced by this invocation.
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
        import_path = _label_to_import_path(label)
        artifacts = results.setdefault(import_path, {"cached": cached, "xml_paths": [], "log_paths": []})
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
    cache_status = {import_path: artifacts["cached"] for import_path, artifacts in test_artifacts.items()}
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
            for import_path in sorted(test_artifacts.keys()):
                manifest.writelines(
                    f'{import_path}\t{log_path}\n' for log_path in test_artifacts[import_path]["log_paths"]
                )

        bazel(
            ctx,
            "run",
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
