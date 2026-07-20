"""
Utilities to manage build tags.

The canonical tag-set data lives in build_tags.bzl, which is written in the
common subset of Starlark and Python so it can be both `load()`ed by
//BUILD.bazel (for GAZELLE_BUILD_TAGS) and exec'd here. This module loads that
data, layers on the AgentFlavor-keyed `build_tags` mapping and the codegen
payload (which can't live in Starlark — no enums), and provides the @task entry
points and helpers that operate on the tags.
"""

from __future__ import annotations

import importlib.machinery
import importlib.util
import json
import os
import sys
from pathlib import Path
from typing import Any

from invoke import task

from tasks.flavor import AgentFlavor

# Load the shared Starlark/Python data file. It is valid Python (set([...])
# literals, set operators/methods), so we exec it and rebind the names below.
# An explicit SourceFileLoader is needed because importlib won't infer one from
# the .bzl extension. Typed as Any so the dynamic attribute access is mypy-clean.
_BZL_PATH = Path(__file__).with_name("build_tags.bzl")
_loader = importlib.machinery.SourceFileLoader("tasks._build_tags_data", str(_BZL_PATH))
_spec = importlib.util.spec_from_loader(_loader.name, _loader)
assert _spec is not None
_data: Any = importlib.util.module_from_spec(_spec)
_loader.exec_module(_data)

# Gazelle / "all tags" sets
COMMON_TAGS = _data.COMMON_TAGS
ALL_TAGS = _data.ALL_TAGS
GAZELLE_EXTRA_TAGS = _data.GAZELLE_EXTRA_TAGS
GAZELLE_OMIT_TAGS = _data.GAZELLE_OMIT_TAGS
GAZELLE_BUILD_TAGS = _data.GAZELLE_BUILD_TAGS

# Per-binary inclusion lists
AGENT_TAGS = _data.AGENT_TAGS
AGENT_HEROKU_TAGS = _data.AGENT_HEROKU_TAGS
FIPS_TAGS = _data.FIPS_TAGS
CLUSTER_AGENT_TAGS = _data.CLUSTER_AGENT_TAGS
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = _data.CLUSTER_AGENT_CLOUDFOUNDRY_TAGS
DOGSTATSD_TAGS = _data.DOGSTATSD_TAGS
IOT_AGENT_TAGS = _data.IOT_AGENT_TAGS
INSTALLER_TAGS = _data.INSTALLER_TAGS
PROCESS_AGENT_TAGS = _data.PROCESS_AGENT_TAGS
PROCESS_AGENT_HEROKU_TAGS = _data.PROCESS_AGENT_HEROKU_TAGS
SECURITY_AGENT_TAGS = _data.SECURITY_AGENT_TAGS
SBOMGEN_TAGS = _data.SBOMGEN_TAGS
SERVERLESS_TAGS = _data.SERVERLESS_TAGS
SYSTEM_PROBE_TAGS = _data.SYSTEM_PROBE_TAGS
TRACE_AGENT_TAGS = _data.TRACE_AGENT_TAGS
TRACE_AGENT_HEROKU_TAGS = _data.TRACE_AGENT_HEROKU_TAGS
CWS_INSTRUMENTATION_TAGS = _data.CWS_INSTRUMENTATION_TAGS
OTEL_AGENT_TAGS = _data.OTEL_AGENT_TAGS
LOADER_TAGS = _data.LOADER_TAGS
HOST_PROFILER_TAGS = _data.HOST_PROFILER_TAGS
PRIVATEACTIONRUNNER_TAGS = _data.PRIVATEACTIONRUNNER_TAGS
SECRET_GENERIC_CONNECTOR_TAGS = _data.SECRET_GENERIC_CONNECTOR_TAGS
AGENT_TEST_TAGS = _data.AGENT_TEST_TAGS

# Exclusion lists
LINUX_ONLY_TAGS = _data.LINUX_ONLY_TAGS
AIX_EXCLUDED_TAGS = _data.AIX_EXCLUDED_TAGS
WINDOWS_INCLUDED_TAGS = _data.WINDOWS_INCLUDED_TAGS
WINDOWS_EXCLUDED_TAGS = _data.WINDOWS_EXCLUDED_TAGS
DARWIN_EXCLUDED_TAGS = _data.DARWIN_EXCLUDED_TAGS
UNIT_TEST_TAGS = _data.UNIT_TEST_TAGS
UNIT_TEST_EXCLUDED_TAGS = _data.UNIT_TEST_EXCLUDED_TAGS

# Build type: maps flavor to build tags map
build_tags = {
    AgentFlavor.base: {
        # Build setups
        "agent": AGENT_TAGS,
        "cluster-agent": CLUSTER_AGENT_TAGS,
        "cluster-agent-cloudfoundry": CLUSTER_AGENT_CLOUDFOUNDRY_TAGS,
        "dogstatsd": DOGSTATSD_TAGS,
        "installer": INSTALLER_TAGS,
        "process-agent": PROCESS_AGENT_TAGS,
        "security-agent": SECURITY_AGENT_TAGS,
        "serverless": SERVERLESS_TAGS,
        "system-probe": SYSTEM_PROBE_TAGS,
        "system-probe-unit-tests": SYSTEM_PROBE_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
        "trace-agent": TRACE_AGENT_TAGS,
        "cws-instrumentation": CWS_INSTRUMENTATION_TAGS,
        "sbomgen": SBOMGEN_TAGS,
        "otel-agent": OTEL_AGENT_TAGS,
        "loader": LOADER_TAGS,
        "host-profiler": HOST_PROFILER_TAGS,
        "privateactionrunner": PRIVATEACTIONRUNNER_TAGS,
        "secret-generic-connector": SECRET_GENERIC_CONNECTOR_TAGS,
        # Test setups
        "test": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS)
        .union(CLUSTER_AGENT_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDED_TAGS),
        "lint": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS)
        .union(CLUSTER_AGENT_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDED_TAGS),
        "unit-tests": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS)
        .union(CLUSTER_AGENT_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDED_TAGS),
    },
    AgentFlavor.fips: {
        "agent": AGENT_TAGS.union(FIPS_TAGS),
        "dogstatsd": DOGSTATSD_TAGS.union(FIPS_TAGS),
        "process-agent": PROCESS_AGENT_TAGS.union(FIPS_TAGS),
        "security-agent": SECURITY_AGENT_TAGS.union(FIPS_TAGS),
        "serverless": SERVERLESS_TAGS.union(FIPS_TAGS),
        "system-probe": SYSTEM_PROBE_TAGS.union(FIPS_TAGS),
        "system-probe-unit-tests": SYSTEM_PROBE_TAGS.union(FIPS_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDED_TAGS),
        "trace-agent": TRACE_AGENT_TAGS.union(FIPS_TAGS),
        "cws-instrumentation": CWS_INSTRUMENTATION_TAGS.union(FIPS_TAGS),
        "sbomgen": SBOMGEN_TAGS.union(FIPS_TAGS),
        "installer": INSTALLER_TAGS.union(FIPS_TAGS),
        "privateactionrunner": PRIVATEACTIONRUNNER_TAGS.union(FIPS_TAGS),
        "secret-generic-connector": SECRET_GENERIC_CONNECTOR_TAGS.union(FIPS_TAGS),
        # Test setups
        "lint": AGENT_TAGS.union(FIPS_TAGS).union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
        "unit-tests": AGENT_TAGS.union(FIPS_TAGS).union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
        "otel-agent": OTEL_AGENT_TAGS.union(FIPS_TAGS),
    },
    AgentFlavor.heroku: {
        "agent": AGENT_HEROKU_TAGS,
        "process-agent": PROCESS_AGENT_HEROKU_TAGS,
        "trace-agent": TRACE_AGENT_HEROKU_TAGS,
        "lint": AGENT_HEROKU_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
        "unit-tests": AGENT_HEROKU_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
    },
    AgentFlavor.iot: {
        "agent": IOT_AGENT_TAGS,
        "lint": IOT_AGENT_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
        "unit-tests": IOT_AGENT_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
    },
    AgentFlavor.dogstatsd: {
        "dogstatsd": DOGSTATSD_TAGS,
        "lint": DOGSTATSD_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
        "unit-tests": DOGSTATSD_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDED_TAGS),
    },
}


def build_tags_codegen_payload() -> dict[str, object]:
    """Structured view of the tag data consumed by the codegen.

    All list values are sorted and deduplicated so the generated JSON / .go
    files are byte-stable.
    """
    return {
        "common_tags": sorted(COMMON_TAGS),
        "unit_test_tags": sorted(UNIT_TEST_TAGS),
        "linux_only_tags": sorted(LINUX_ONLY_TAGS),
        "windows_included_tags": sorted(WINDOWS_INCLUDED_TAGS),
        "windows_excluded_tags": sorted(WINDOWS_EXCLUDED_TAGS),
        "darwin_excluded_tags": sorted(DARWIN_EXCLUDED_TAGS),
        "flavor_specific_tags": {
            flavor.name: sorted(build_tags[flavor]["unit-tests"] - COMMON_TAGS - UNIT_TEST_TAGS)
            for flavor in AgentFlavor
            if "unit-tests" in build_tags.get(flavor, {})
        },
        "gazelle_build_tags": sorted(GAZELLE_BUILD_TAGS),
    }


_GOOS_TO_SYS_PLATFORM = {
    "windows": "win32",
}


def _resolve_platform(platform=None):
    """Return the effective target platform as a sys.platform-style string.

    If platform is explicitly provided, normalize it from GOOS format to
    sys.platform format (e.g. "windows" -> "win32"). Otherwise fall back to
    the GOOS env var, then sys.platform.
    """
    if platform is None:
        platform = os.getenv("GOOS") or sys.platform
    return _GOOS_TO_SYS_PLATFORM.get(platform, platform)


def compute_build_tags_for_flavor(
    build: str,
    build_include: str | None,
    build_exclude: str | None,
    flavor: AgentFlavor = AgentFlavor.base,
    platform: str | None = None,
):
    """
    Given a flavor, an architecture, a list of tags to include and exclude, get the final list
    of tags that should be applied.
    If the list of build tags to include is empty, take the default list of build tags for
    the flavor or arch. Otherwise, use the list of build tags to include, minus incompatible tags
    for the given architecture.

    Then, remove from these the provided list of tags to exclude.
    """
    platform = _resolve_platform(platform)

    build_include = (
        get_default_build_tags(build=build, flavor=flavor, platform=platform)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), platform=platform)
    )

    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    list = get_build_tags(build_include, build_exclude)

    return list


@task
def print_default_build_tags(_, build="agent", flavor=AgentFlavor.base.name, platform: str | None = None):
    """
    Build the default list of tags based on the build type and platform.
    Prints as comma separated list suitable for go tooling (eg, gopls, govulncheck)

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """

    try:
        flavor = AgentFlavor[flavor]
    except KeyError:
        flavorOptions = [flavor.name for flavor in AgentFlavor]
        print(f"'{flavor}' does not correspond to an agent flavor. Options: {flavorOptions}")
        exit(1)

    print(",".join(sorted(get_default_build_tags(build=build, flavor=flavor, platform=platform))))


def get_default_build_tags(build="agent", flavor=AgentFlavor.base, platform: str | None = None):
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    platform = _resolve_platform(platform)
    include = build_tags[flavor].get(build)
    if include is None:
        print("Warning: unrecognized build type, no build tags included.", file=sys.stderr)
        include = set()

    include = include.union(COMMON_TAGS)
    return sorted(filter_incompatible_tags(include, platform=platform))


def filter_incompatible_tags(include, platform=None):
    """
    Filter out tags incompatible with the platform.
    include can be a list or a set.
    """
    platform = _resolve_platform(platform)
    exclude = set()
    if not platform.startswith("linux"):
        exclude = exclude.union(LINUX_ONLY_TAGS)

    if platform == "win32":
        include = include.union(WINDOWS_INCLUDED_TAGS)
        exclude = exclude.union(WINDOWS_EXCLUDED_TAGS)

    if platform == "darwin":
        exclude = exclude.union(DARWIN_EXCLUDED_TAGS)

    if platform == "aix":
        exclude = exclude.union(AIX_EXCLUDED_TAGS)

    return get_build_tags(include, exclude)


def get_build_tags(include, exclude):
    """
    Build the list of tags based on inclusions and exclusions passed through
    the command line
    include and exclude can be lists or sets.
    """
    # Convert parameters to sets
    include = set(include)
    exclude = set(exclude)

    # filter out unrecognised tags
    known_include = ALL_TAGS.intersection(include)
    unknown_include = include - known_include
    for tag in unknown_include:
        print(f"Warning: unknown build tag '{tag}' was filtered out from included tags list.", file=sys.stderr)

    known_exclude = ALL_TAGS.intersection(exclude)
    unknown_exclude = exclude - known_exclude
    for tag in unknown_exclude:
        print(f"Warning: unknown build tag '{tag}' was filtered out from excluded tags list.", file=sys.stderr)

    return list(known_include - known_exclude)


def compute_config_build_tags(
    targets="all", build_include=None, build_exclude=None, flavor=AgentFlavor.base.name, platform=None
):
    flavor = AgentFlavor[flavor]

    if targets == "all":
        targets = build_tags[flavor].keys()
    else:
        targets = targets.split(",")
        if not set(targets).issubset(build_tags[flavor]):
            print("Must choose valid targets. Valid targets are:")
            print(f'{", ".join(build_tags[flavor].keys())}')
            exit(1)

    if build_include is None:
        build_include = []
        for target in targets:
            build_include.extend(get_default_build_tags(build=target, flavor=flavor, platform=platform))
    else:
        build_include = filter_incompatible_tags(build_include.split(","), platform=platform)

    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)
    return use_tags


@task
def codegen_to_json(_, output=""):
    """Emit build-tag data as JSON.

    Writes to --output= path if provided (so callers can sidestep stdout noise
    from dda/rich's console init on Windows), otherwise prints to stdout.
    """
    text = json.dumps(build_tags_codegen_payload(), indent=2, sort_keys=True)
    if output:
        with open(output, "w") as f:
            f.write(text)
    else:
        print(text)
