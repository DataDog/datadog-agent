"""
Utilities for discovering and running Rust tests via Bazel
"""

from __future__ import annotations

import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path

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


def discover_rust_tests(target_paths: list[str]) -> dict[str, str]:
    """
    Discover Rust tests in the given target paths by scanning for BUILD.bazel files.

    Args:
        target_paths: List of paths to scan for Rust tests (e.g., ["./pkg/collector/..."])

    Returns:
        Dictionary mapping test_name -> source_path
        Example: {"rusthello_test": "pkg/collector/corechecks/servicediscovery/module/rusthello"}
    """
    rust_tests = {}

    for target in target_paths:
        # Remove leading "./" and trailing "/..."
        clean_target = target.lstrip('./').rstrip('/...')

        # Find all BUILD.bazel files in the target directory
        target_dir = Path(clean_target)
        if not target_dir.exists():
            continue

        for build_file in target_dir.rglob('BUILD.bazel'):
            # Read the BUILD.bazel file
            try:
                content = build_file.read_text()

                # Look for rust_test() definitions
                # Pattern: rust_test(\n    name = "test_name",
                test_pattern = r'rust_test\s*\(\s*name\s*=\s*"([^"]+)"'
                matches = re.findall(test_pattern, content)

                for test_name in matches:
                    # Get the source path relative to repo root
                    source_path = str(build_file.parent)
                    rust_tests[test_name] = source_path

            except Exception as e:
                print(color_message(f"Warning: Failed to read {build_file}: {e}", "yellow"))
                continue

    return rust_tests


def run_rust_tests(
    ctx: Context,
    rust_tests: dict[str, str],
    arch: Arch,
    junit_base_name: str | None = None,
    flavor=None,
) -> RustTestResult:
    """
    Run Rust tests using Bazel with CI visibility support.

    Args:
        ctx: Invoke context
        rust_tests: Dictionary mapping test_name -> source_path
        arch: Architecture to run tests for
        junit_base_name: Base name for JUnit XML files (e.g., "junit-rust-base")
        flavor: Agent flavor for JUnit XML enrichment

    Returns:
        RustTestResult with success status, failures, and JUnit file paths
    """
    import shutil
    from datetime import datetime, timezone

    # Skip on Windows
    if sys.platform == 'win32':
        print(color_message("Rust tests are not supported on Windows, skipping", "yellow"))
        return RustTestResult(success=True, failures=[], test_count=0, junit_files=[])

    # Check if bazelisk is available
    check_bazel = ctx.run("which bazelisk", warn=True, hide=True)
    if check_bazel.exited != 0:
        print(
            color_message(
                "Warning: bazelisk not found, skipping Rust tests. Install with: brew install bazelisk", "yellow"
            )
        )
        return RustTestResult(success=True, failures=[], test_count=0, junit_files=[])

    if not rust_tests:
        return RustTestResult(success=True, failures=[], test_count=0, junit_files=[])

    # Platform mapping for Bazel
    platform_map = {
        "x86_64": "//bazel/platforms:linux_x86_64",
        "arm64": "//bazel/platforms:linux_arm64",
    }

    platform_flag = ""
    if arch.kmt_arch in platform_map:
        platform_flag = f"--platforms={platform_map[arch.kmt_arch]}"

    test_results = []
    test_count = len(rust_tests)

    for test_name, source_path in rust_tests.items():
        print(f"Running Rust test: {test_name} ({source_path})")
        start_time = datetime.now(timezone.utc)

        # Run Bazel test - it automatically generates test.xml in bazel-testlogs
        result = ctx.run(
            f"bazelisk test {platform_flag} --test_output=errors -- @//{source_path}:{test_name}", warn=True
        )

        end_time = datetime.now(timezone.utc)

        # Bazel creates test.xml in bazel-testlogs/{source_path}/{test_name}/test.xml
        bazel_xml_path = f"bazel-testlogs/{source_path}/{test_name}/test.xml"

        test_results.append(
            {
                'test_name': test_name,
                'source_path': source_path,
                'start_time': start_time,
                'end_time': end_time,
                'xml_path': bazel_xml_path if os.path.exists(bazel_xml_path) else None,
                'exit_code': result.exited if result.exited is not None else 1,
            }
        )

    # Collect JUnit XML files
    junit_files = []
    if junit_base_name and flavor:

        from tasks.libs.common.junit_upload_core import enrich_junitxml

        for result in test_results:
            if result['xml_path'] and os.path.exists(result['xml_path']):
                # Copy Bazel's test.xml to our output location with consistent naming
                output_xml = f"{junit_base_name}-{result['test_name']}.xml"
                shutil.copy2(result['xml_path'], output_xml)

                # Enhance error message with full test output from system-out
                _enhance_junit_error_message(output_xml, result['source_path'], result['test_name'])

                # Enrich JUnit XML with flavor info (same as Go tests)
                enrich_junitxml(output_xml, flavor)

                junit_files.append(output_xml)

    # Determine success/failure
    failed_tests = [f"{r['source_path']}:{r['test_name']}" for r in test_results if r['exit_code'] != 0]

    return RustTestResult(
        success=len(failed_tests) == 0, failures=failed_tests, test_count=test_count, junit_files=junit_files
    )


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
