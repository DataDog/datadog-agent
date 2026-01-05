"""
Utilities for running Rust tests via Bazel
"""

from __future__ import annotations

import os
import sys
from dataclasses import dataclass

from invoke.context import Context

from tasks.libs.common.color import color_message
from tasks.libs.types.arch import Arch


@dataclass
class RustTestResult:
    """Result of running Rust tests"""

    success: bool
    failures: list[str]
    test_count: int
    junit_files: list[str]  # Paths to generated JUnit XML files


def run_rust_tests(
    ctx: Context,
    target_paths: list[str],
    arch: Arch,
    junit_base_name: str | None = None,
    flavor=None,
) -> RustTestResult:
    """
    Run Rust tests using Bazel with CI visibility support.

    Args:
        ctx: Invoke context
        target_paths: List of paths to scan for Rust tests (e.g., ["./pkg", "./cmd"])
        arch: Architecture to run tests for
        junit_base_name: Base name for JUnit XML files (e.g., "junit-rust-base")
        flavor: Agent flavor for JUnit XML enrichment

    Returns:
        RustTestResult with success status, failures, and JUnit file paths
    """
    import shutil
    import subprocess
    import xml.etree.ElementTree as ET

    # Skip on Windows and macOS
    if sys.platform == 'win32' or sys.platform == 'darwin':
        print(color_message("Rust tests are only supported on Linux, skipping", "yellow"))
        return RustTestResult(success=True, failures=[], test_count=0, junit_files=[])

    # Platform mapping for Bazel
    platform_map = {
        "x86_64": "//bazel/platforms:linux_x86_64",
        "arm64": "//bazel/platforms:linux_arm64",
    }

    platform_flag = ""
    if arch.kmt_arch in platform_map:
        platform_flag = f"--platforms={platform_map[arch.kmt_arch]}"

    all_targets = []
    patterns = []
    for path in target_paths:
        # Normalize path: "./pkg/" -> "//pkg/..."
        clean_path = os.path.normpath(path)
        clean_path = clean_path.lstrip('./').rstrip('/').rstrip('.')
        bazel_pattern = f"//{clean_path}/..."
        patterns.append(bazel_pattern)

    try:
        # --keep_going is needed since otherwise the command fails if any of the
        # paths do not have any test targets.
        cmd = (
            [
                "bazelisk",
                "query",
                f"tests({' + '.join(patterns)})",
                "--output=label",
                "--keep_going",
                "--noshow_progress",
            ],
        )
        query_result = subprocess.run(
            *cmd,
            capture_output=True,
            text=True,
            check=False,
        )

        # We don't check the return code since it returns an error if any of the
        # paths do not have any test targets.

        # Parse targets (one per line)
        for line in query_result.stdout.strip().split('\n'):
            if line:
                all_targets.append(line)
    except Exception as e:
        print(color_message(f"Warning: Failed to query Bazel for {patterns}: {e}", "yellow"))

    if not all_targets:
        # No Rust tests found, don't run "bazelisk test" on the paths since it will error out.
        return RustTestResult(success=True, failures=[], test_count=0, junit_files=[])

    print(color_message(f"Found {len(all_targets)} Rust test(s) in target paths", "blue"))

    result = ctx.run(f"bazelisk test {platform_flag} --test_output=errors -- " + " ".join(all_targets), warn=True)

    junit_files = []
    failed_tests = []

    for target in all_targets:
        # Parse "//pkg/test:my_test" -> package_path="pkg/test", test_name="my_test"
        if ':' not in target:
            continue
        package_path = target.split(':')[0].lstrip('//')
        test_name = target.split(':')[1]

        xml_path = f"bazel-testlogs/{package_path}/{test_name}/test.xml"

        if os.path.exists(xml_path):
            if junit_base_name and flavor:
                # Copy and enhance JUnit XML
                output_xml = f"{junit_base_name}-{package_path.replace('/', '_')}-{test_name}.xml"
                shutil.copy2(xml_path, output_xml)
                _enhance_junit_error_message(output_xml, package_path, test_name)

                from tasks.libs.common.junit_upload_core import enrich_junitxml

                enrich_junitxml(output_xml, flavor)

                junit_files.append(output_xml)

            # Check if test failed by parsing XML
            try:
                tree = ET.parse(xml_path)
                root = tree.getroot()
                testsuite = root.find('.//testsuite')
                if testsuite is not None:
                    failures = int(testsuite.get('failures', 0))
                    errors = int(testsuite.get('errors', 0))
                    if failures > 0 or errors > 0:
                        failed_tests.append(f"{package_path}:{test_name}")
            except Exception as e:
                print(color_message(f"Warning: Failed to parse XML for {test_name}: {e}", "yellow"))

    # 4. Determine overall success
    success = result.exited == 0 and len(failed_tests) == 0

    if success:
        print(color_message(f"All {len(all_targets)} Rust tests passed", "green"))
    else:
        if failed_tests:
            print(color_message(f"Rust tests failed: {', '.join(failed_tests)}", "red"))
        else:
            print(color_message("Rust tests failed", "red"))

    return RustTestResult(success=success, failures=failed_tests, test_count=len(all_targets), junit_files=junit_files)


def _enhance_junit_error_message(xml_path: str, source_path: str, test_name: str):
    """
    Enhance Bazel's JUnit XML by replacing the generic error message with full test output.

    Bazel generates a generic "exited with error code 101" message, but the actual test
    output is in <system-out>. This function copies that output into the error message
    so it's visible in Datadog Test Visibility.

    Also restructures the names so that:
    - Test suite name = source_path (e.g., "pkg/collector/.../rusthello")
    - Test case name = test_name (e.g., "rusthello_test")

    Args:
        xml_path: Path to the JUnit XML file to enhance
        source_path: Source path for the test (used as test suite name and classname)
        test_name: Name of the test (used as test case name)
    """
    import xml.etree.ElementTree as ET

    tree = ET.parse(xml_path)
    root = tree.getroot()

    for testsuite in root.findall('.//testsuite'):
        # Set testsuite name to just the source path
        testsuite.set('name', source_path)

        # Get the testcase, error element, and system-out
        testcase = testsuite.find('testcase')
        system_out = testsuite.find('system-out')

        if testcase is None:
            continue

        # Set testcase name to just the test name (not the full path)
        testcase.set('name', test_name)

        # Set classname to the source path
        testcase.set('classname', source_path)

        # If there's an error element and system-out, replace error message with full output
        error = testcase.find('error')
        if error is not None and system_out is not None and system_out.text:
            # Replace the generic error message with the full test output
            error.set('message', 'Rust test suite failed')
            error.text = system_out.text.strip()

    # Write back the enhanced XML
    tree.write(xml_path, encoding='UTF-8', xml_declaration=True)
