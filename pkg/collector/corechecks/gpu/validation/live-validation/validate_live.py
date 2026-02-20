#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "datadog-api-client>=2.20.0",
#     "pyyaml>=6.0",
# ]
# ///
"""
GPU Metrics Live Validation Script

Queries the Datadog API for active GPU metrics and validates them against
the YAML specification. Discovers hosts by (architecture, device_mode),
samples hosts from each group, and validates metric presence and tag
compliance per group.

Usage:
    # With uv (recommended - auto-installs dependencies):
    DD_SITE=datadoghq.com DD_API_KEY=<key> DD_APP_KEY=<key> \
        uv run validate_live.py --spec ../spec/gpu_metrics.yaml

    # Dry run (no API keys needed):
    uv run validate_live.py --spec ../spec/gpu_metrics.yaml --dry-run

    # Filter by architecture:
    uv run validate_live.py --spec ../spec/gpu_metrics.yaml --arch turing
"""

import argparse
import os
import sys
import time
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

from datadog_api_client import ApiClient, Configuration
from datadog_api_client.v1.api.metrics_api import MetricsApi
import yaml


# Architecture generation order for filtering
ARCH_ORDER = [
    "fermi", "kepler", "maxwell", "pascal",
    "volta", "turing", "ampere", "hopper",
]

# Maximum hosts to sample per (architecture, device_mode) group
MAX_HOSTS_PER_GROUP = 2

# Tag spot-check definitions: (representative_metric, tag_keys_to_check, category_label)
TAG_SPOT_CHECKS = [
    {
        "label": "device tagset",
        "metric": "gpu.temperature",
        "tags": ["gpu_uuid", "gpu_device", "gpu_vendor", "gpu_driver_version"],
    },
    {
        "label": "process tagset",
        "metric": "gpu.process.memory.usage",
        "tags": ["pid"],
    },
    {
        "label": "custom_tags (memory_location)",
        "metric": "gpu.errors.ecc.corrected.total",
        "tags": ["memory_location"],
    },
    {
        "label": "custom_tags (type, origin)",
        "metric": "gpu.errors.xid.total",
        "tags": ["type", "origin"],
    },
]


def load_spec(spec_path: str) -> dict:
    """Load and parse the YAML spec file."""
    with open(spec_path) as f:
        return yaml.safe_load(f)


def normalize_support_value(value) -> bool | None:
    """
    Normalize support values from the YAML spec.

    Returns:
    - True for explicit support ("true", true)
    - False for explicit non-support ("false", false)
    - None for unknown/missing/other values
    """
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered == "true":
            return True
        if lowered == "false":
            return False
    return None


def get_expected_metrics(spec: dict, arch_filter: str | None = None) -> dict[str, list[dict]]:
    """
    Extract expected metric names from the spec.
    Returns a dict of metric_name -> list of spec entries.

    If arch_filter is provided, only include metrics that are not explicitly
    unsupported on that architecture.
    """
    arch_name = None
    if arch_filter:
        arch_filter = arch_filter.lower()
        if arch_filter in ARCH_ORDER:
            arch_name = arch_filter

    namespace = spec.get("metric_prefix", "gpu")
    expected = {}

    for metric_name, metric in spec.get("metrics", {}).items():
        if metric.get("deprecated", False):
            continue

        support = metric.get("support", {})
        unsupported_archs = support.get("unsupported_architectures", [])

        # Architecture filtering
        if arch_name is not None and arch_name in unsupported_archs:
            continue

        full_name = f"{namespace}.{metric_name}"
        if full_name not in expected:
            expected[full_name] = []
        expected[full_name].append({
            "type": metric.get("type", ""),
            "tagsets": metric.get("tagsets", []),
            "custom_tags": metric.get("custom_tags", []),
            "memory_locations": metric.get("memory_locations", []),
            "support": support,
            "unsupported_architectures": unsupported_archs,
            "device_features": support.get("device_features", {}),
            "process_data": support.get("process_data", False),
        })

    return expected


def get_expected_metrics_for_host(
    spec: dict,
    host_arch: str,
    device_mode: str,
) -> dict[str, list[dict]]:
    """
    Get metrics expected for a specific host based on its architecture and
    device mode (physical/mig/vgpu).

    Filters by:
    - unsupported_architectures: host arch must not be listed
    - device_features: host mode must not be explicitly "false"
      (true or unknown both count as expected)
    """
    namespace = spec.get("metric_prefix", "gpu")
    expected = {}

    for metric_name, metric in spec.get("metrics", {}).items():
        if metric.get("deprecated", False):
            continue

        support = metric.get("support", {})
        unsupported_archs = support.get("unsupported_architectures", [])
        if host_arch.lower() in unsupported_archs:
            continue

        # Device mode filtering: only explicit false is unsupported.
        device_features = support.get("device_features", {})
        support_value = normalize_support_value(device_features.get(device_mode))
        if support_value is False:
            continue

        full_name = f"{namespace}.{metric_name}"
        if full_name not in expected:
            expected[full_name] = []
        expected[full_name].append({
            "type": metric.get("type", ""),
            "tagsets": metric.get("tagsets", []),
            "custom_tags": metric.get("custom_tags", []),
            "memory_locations": metric.get("memory_locations", []),
            "support": support,
            "unsupported_architectures": unsupported_archs,
            "device_features": device_features,
            "process_data": support.get("process_data", False),
        })

    return expected


def get_deprecated_metrics(spec: dict) -> set[str]:
    """Get set of deprecated metric names."""
    namespace = spec.get("metric_prefix", "gpu")
    deprecated = set()
    for metric_name, metric in spec.get("metrics", {}).items():
        if metric.get("deprecated", False):
            deprecated.add(f"{namespace}.{metric_name}")
    return deprecated


def get_all_spec_device_groups(spec: dict) -> set[tuple[str, str]]:
    """
    Derive all (architecture, device_mode) combinations from the spec
    that have at least one metric with support != false.
    """
    device_modes = ["physical", "mig", "vgpu"]
    groups = set()

    for arch in ARCH_ORDER:
        for mode in device_modes:
            metrics = get_expected_metrics_for_host(spec, arch, mode)
            if metrics:
                groups.add((arch, mode))

    return groups


def make_api_client(site: str | None = None):
    """Create a configured Datadog API client."""
    config = Configuration()
    if site:
        config.server_variables["site"] = site
    return config


def query_live_metrics(site: str | None = None) -> set[str]:
    """Query the Datadog API for all active gpu.* metrics."""
    config = make_api_client(site)

    with ApiClient(config) as api_client:
        api = MetricsApi(api_client)
        response = api.list_metrics(q="metrics:gpu.")

    metrics = set()
    if hasattr(response, "results") and response.results:
        if hasattr(response.results, "metrics"):
            metrics = set(response.results.metrics)
        elif isinstance(response.results, list):
            metrics = set(response.results)

    return {m for m in metrics if m.startswith("gpu.")}


def discover_hosts(site: str | None = None) -> dict[tuple[str, str], list[str]]:
    """
    Discover hosts by querying gpu.device.total grouped by host, architecture,
    and slicing/virtualization mode.

    Returns a dict of (architecture, device_mode) -> list of hostnames.
    """
    config = make_api_client(site)
    now = int(time.time())
    one_hour_ago = now - 3600

    query = "avg:gpu.device.total{*} by {host,gpu_architecture,gpu_slicing_mode,gpu_virtualization_mode}"

    with ApiClient(config) as api_client:
        api = MetricsApi(api_client)
        response = api.query_metrics(
            _from=one_hour_ago,
            to=now,
            query=query,
        )

    groups: dict[tuple[str, str], set[str]] = defaultdict(set)

    if not hasattr(response, "series") or not response.series:
        return {}

    for series in response.series:
        tag_set = series.tag_set if hasattr(series, "tag_set") else []

        hostname = None
        architecture = None
        slicing_mode = None
        virtualization_mode = None

        for tag in tag_set:
            if tag.startswith("host:"):
                hostname = tag[5:]
            elif tag.startswith("gpu_architecture:"):
                architecture = tag[17:].lower()
            elif tag.startswith("gpu_slicing_mode:"):
                slicing_mode = tag[17:].lower()
            elif tag.startswith("gpu_virtualization_mode:"):
                virtualization_mode = tag[23:].lower()

        if not hostname or not architecture:
            continue

        # Determine device_mode
        if slicing_mode == "mig":
            device_mode = "mig"
        elif virtualization_mode == "vgpu":
            device_mode = "vgpu"
        else:
            device_mode = "physical"

        groups[(architecture, device_mode)].add(hostname)

    # Convert sets to sorted lists
    return {k: sorted(v) for k, v in groups.items()}


def sample_hosts(
    host_groups: dict[tuple[str, str], list[str]],
    max_per_group: int = MAX_HOSTS_PER_GROUP,
) -> dict[tuple[str, str], list[str]]:
    """Pick up to max_per_group hosts from each group."""
    return {
        group: hosts[:max_per_group]
        for group, hosts in host_groups.items()
    }


def query_metric_for_host(
    api: MetricsApi,
    metric_name: str,
    hostname: str,
    from_ts: int,
    to_ts: int,
) -> bool:
    """
    Query a single metric for a specific host. Returns True if data points exist.
    """
    query = f"avg:{metric_name}{{host:{hostname}}}"
    try:
        response = api.query_metrics(_from=from_ts, to=to_ts, query=query)
        if hasattr(response, "series") and response.series:
            for series in response.series:
                if hasattr(series, "pointlist") and series.pointlist:
                    return True
        return False
    except Exception as e:
        print(f"    WARNING: Query failed for {metric_name} on {hostname}: {e}")
        return False


def validate_metrics_for_host(
    site: str,
    hostname: str,
    expected_metrics: dict[str, list[dict]],
) -> dict[str, bool]:
    """
    For each expected metric, query it for the given host.
    Returns a dict of metric_name -> present (bool).
    """
    config = make_api_client(site)
    now = int(time.time())
    one_hour_ago = now - 3600

    results = {}

    with ApiClient(config) as api_client:
        api = MetricsApi(api_client)
        total = len(expected_metrics)
        for i, metric_name in enumerate(sorted(expected_metrics.keys()), 1):
            if i % 20 == 0 or i == total:
                print(f"    Querying metrics: {i}/{total}...")
            results[metric_name] = query_metric_for_host(
                api, metric_name, hostname, one_hour_ago, now,
            )

    return results


def validate_tags_for_host(
    site: str,
    hostname: str,
) -> list[dict]:
    """
    Run tag spot-checks for a host. For each check, query a representative
    metric grouped by the expected tags and verify they appear.

    Returns a list of check results.
    """
    config = make_api_client(site)
    now = int(time.time())
    one_hour_ago = now - 3600

    results = []

    with ApiClient(config) as api_client:
        api = MetricsApi(api_client)

        for check in TAG_SPOT_CHECKS:
            tag_keys = check["tags"]
            by_clause = ",".join(tag_keys)
            query = f"avg:{check['metric']}{{host:{hostname}}} by {{{by_clause}}}"

            found_tags: dict[str, set[str]] = {t: set() for t in tag_keys}
            has_data = False

            try:
                response = api.query_metrics(_from=one_hour_ago, to=now, query=query)
                if hasattr(response, "series") and response.series:
                    has_data = True
                    for series in response.series:
                        tag_set = series.tag_set if hasattr(series, "tag_set") else []
                        for tag in tag_set:
                            key, _, value = tag.partition(":")
                            if key in found_tags:
                                found_tags[key].add(value)
            except Exception as e:
                results.append({
                    "label": check["label"],
                    "metric": check["metric"],
                    "expected_tags": tag_keys,
                    "pass": False,
                    "has_data": False,
                    "detail": f"Query error: {e}",
                })
                continue

            if not has_data:
                results.append({
                    "label": check["label"],
                    "metric": check["metric"],
                    "expected_tags": tag_keys,
                    "pass": None,  # Indeterminate — no data
                    "has_data": False,
                    "detail": "No data returned for this metric on this host",
                })
                continue

            # A tag is "present" if we found at least one distinct value
            missing_tags = [t for t in tag_keys if len(found_tags[t]) == 0]
            passed = len(missing_tags) == 0

            detail_parts = []
            for t in tag_keys:
                vals = found_tags[t]
                if vals:
                    detail_parts.append(f"{t}: {len(vals)} distinct value(s)")
                else:
                    detail_parts.append(f"{t}: MISSING")

            results.append({
                "label": check["label"],
                "metric": check["metric"],
                "expected_tags": tag_keys,
                "pass": passed,
                "has_data": True,
                "detail": "; ".join(detail_parts),
            })

    return results


def generate_report(
    expected: dict[str, list[dict]],
    deprecated: set[str],
    live_metrics: set[str],
    site: str,
    arch_filter: str | None,
    host_groups: dict[tuple[str, str], list[str]],
    sampled_hosts: dict[tuple[str, str], list[str]],
    per_host_metric_results: dict[str, dict[str, bool]],
    per_host_tag_results: dict[str, list[dict]],
    spec: dict,
) -> str:
    """Generate an enhanced markdown compliance report."""
    expected_names = set(expected.keys())

    present_expected = expected_names & live_metrics
    missing_expected = expected_names - live_metrics
    present_unexpected = live_metrics - expected_names
    present_deprecated = present_unexpected & deprecated
    present_unknown = present_unexpected - deprecated

    lines = []
    lines.append("# GPU Metrics Validation Report")
    lines.append("")
    lines.append(f"**Generated**: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}")
    lines.append(f"**Site**: {site}")
    if arch_filter:
        lines.append(f"**Architecture filter**: {arch_filter}")
    lines.append("")

    # ── Section 1: Global Summary ──
    lines.append("## 1. Global Summary")
    lines.append("")
    lines.append("| Category | Count |")
    lines.append("|----------|-------|")
    lines.append(f"| Expected metrics in spec | {len(expected_names)} |")
    lines.append(f"| Live metrics found | {len(live_metrics)} |")
    lines.append(f"| Present & expected | {len(present_expected)} |")
    lines.append(f"| Missing & expected | {len(missing_expected)} |")
    lines.append(f"| Present but deprecated | {len(present_deprecated)} |")
    lines.append(f"| Present & unknown (not in spec) | {len(present_unknown)} |")
    lines.append("")

    if missing_expected:
        lines.append("### Missing Expected Metrics (Global)")
        lines.append("")
        lines.append("| Metric | Type | Tagsets | Unsupported Architectures |")
        lines.append("|--------|------|---------|---------------------------|")
        for name in sorted(missing_expected):
            entries = expected[name]
            metric_type = entries[0].get("type", "")
            tagsets = ", ".join(entries[0].get("tagsets", []))
            unsupported_archs = ", ".join(entries[0].get("unsupported_architectures", []))
            lines.append(f"| `{name}` | {metric_type} | {tagsets} | {unsupported_archs} |")
        lines.append("")

    if present_deprecated:
        lines.append("### Deprecated Metrics Still Present")
        lines.append("")
        for name in sorted(present_deprecated):
            lines.append(f"- `{name}`")
        lines.append("")

    if present_unknown:
        lines.append("### Unknown Metrics (Not in Spec)")
        lines.append("")
        for name in sorted(present_unknown):
            lines.append(f"- `{name}`")
        lines.append("")

    # ── Section 2: Device Group Coverage ──
    lines.append("## 2. Device Group Coverage")
    lines.append("")
    lines.append("| Architecture | Device Mode | Hosts Found | Hosts Sampled |")
    lines.append("|-------------|-------------|-------------|---------------|")
    for group in sorted(host_groups.keys()):
        arch, mode = group
        total = len(host_groups[group])
        sampled = len(sampled_hosts.get(group, []))
        lines.append(f"| {arch} | {mode} | {total} | {sampled} |")
    lines.append("")

    # ── Section 3: Per-Group Validation ──
    lines.append("## 3. Per-Group Validation")
    lines.append("")

    for group in sorted(sampled_hosts.keys()):
        arch, mode = group
        hosts = sampled_hosts[group]
        lines.append(f"### {arch} / {mode}")
        lines.append("")
        lines.append(f"**Sampled hosts**: {', '.join(f'`{h}`' for h in hosts)}")
        lines.append("")

        # Expected metrics for this group
        group_expected = get_expected_metrics_for_host(spec, arch, mode)
        lines.append(f"**Expected metrics for this group**: {len(group_expected)}")
        lines.append("")

        for hostname in hosts:
            lines.append(f"#### Host: `{hostname}`")
            lines.append("")

            # Metric presence
            metric_results = per_host_metric_results.get(hostname, {})
            if metric_results:
                present_count = sum(1 for v in metric_results.values() if v)
                total_count = len(metric_results)
                missing = [m for m, v in sorted(metric_results.items()) if not v]

                lines.append(f"**Metrics**: {present_count}/{total_count} present")
                lines.append("")

                if missing:
                    lines.append("<details>")
                    lines.append(f"<summary>Missing metrics ({len(missing)})</summary>")
                    lines.append("")
                    for m in missing:
                        entries = group_expected.get(m, [])
                        tagsets = ", ".join(entries[0].get("tagsets", [])) if entries else "?"
                        lines.append(f"- `{m}` (tagsets: {tagsets})")
                    lines.append("")
                    lines.append("</details>")
                    lines.append("")

            # Tag compliance
            tag_results = per_host_tag_results.get(hostname, [])
            if tag_results:
                lines.append("**Tag compliance**:")
                lines.append("")
                lines.append("| Check | Metric | Result | Details |")
                lines.append("|-------|--------|--------|---------|")
                for tr in tag_results:
                    if tr["pass"] is None:
                        status = "N/A"
                    elif tr["pass"]:
                        status = "PASS"
                    else:
                        status = "FAIL"
                    lines.append(f"| {tr['label']} | `{tr['metric']}` | {status} | {tr['detail']} |")
                lines.append("")

    # ── Section 4: Untested Combinations ──
    all_spec_groups = get_all_spec_device_groups(spec)
    tested_groups = set(sampled_hosts.keys())
    untested = all_spec_groups - tested_groups

    lines.append("## 4. Untested Combinations")
    lines.append("")
    if untested:
        lines.append("These (architecture, device_mode) combinations exist in the spec but had no live hosts:")
        lines.append("")
        lines.append("| Architecture | Device Mode |")
        lines.append("|-------------|-------------|")
        for arch, mode in sorted(untested):
            lines.append(f"| {arch} | {mode} |")
        lines.append("")
    else:
        lines.append("All spec combinations had live hosts.")
        lines.append("")

    # ── Appendix: Present & Expected (Global) ──
    if present_expected:
        lines.append(f"## Appendix: Present & Expected Metrics ({len(present_expected)})")
        lines.append("")
        lines.append("<details>")
        lines.append("<summary>Click to expand</summary>")
        lines.append("")
        for name in sorted(present_expected):
            lines.append(f"- `{name}`")
        lines.append("")
        lines.append("</details>")
        lines.append("")

    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Validate GPU metrics against spec")
    parser.add_argument("--spec", required=True, help="Path to gpu_metrics.yaml")
    parser.add_argument("--arch", help="Filter by architecture (e.g., turing, hopper)")
    parser.add_argument("--output", help="Output directory for report (default: ../reports/)")
    parser.add_argument("--site", help="Datadog site (overrides DD_SITE env var)")
    parser.add_argument("--dry-run", action="store_true",
                       help="Load spec and show expected metrics without querying API")
    parser.add_argument("--max-hosts", type=int, default=MAX_HOSTS_PER_GROUP,
                       help=f"Max hosts to sample per device group (default: {MAX_HOSTS_PER_GROUP})")
    args = parser.parse_args()

    # Load spec
    spec = load_spec(args.spec)
    expected = get_expected_metrics(spec, args.arch)
    deprecated = get_deprecated_metrics(spec)

    print(f"Loaded spec: {len(expected)} unique expected metric names")
    print(f"Deprecated metrics: {len(deprecated)}")

    if args.dry_run:
        print("\n--- Expected metrics ---")
        for name in sorted(expected.keys()):
            entries = expected[name]
            tagsets = ", ".join(entries[0].get("tagsets", [])) if entries else ""
            print(f"  {name} [tagsets: {tagsets}]")
        return

    # Determine site
    site = args.site or os.environ.get("DD_SITE", "datadoghq.com")
    print(f"\nQuerying Datadog API at {site}...")

    # Step 1: Global metric list
    print("\n── Step 1: Global metric list ──")
    live_metrics = query_live_metrics(site)
    print(f"Found {len(live_metrics)} live gpu.* metrics")

    # Step 2: Host discovery
    print("\n── Step 2: Host discovery ──")
    host_groups = discover_hosts(site)
    if not host_groups:
        print("WARNING: No hosts found via gpu.device.total query. "
              "Generating global-only report.")
    else:
        for group, hosts in sorted(host_groups.items()):
            print(f"  {group[0]}/{group[1]}: {len(hosts)} host(s)")

    sampled = sample_hosts(host_groups, args.max_hosts)
    total_sampled = sum(len(h) for h in sampled.values())
    print(f"Sampled {total_sampled} host(s) across {len(sampled)} group(s)")

    # Step 3: Per-host metric presence
    print("\n── Step 3: Per-host metric validation ──")
    per_host_metric_results: dict[str, dict[str, bool]] = {}

    for group, hosts in sorted(sampled.items()):
        arch, mode = group
        group_expected = get_expected_metrics_for_host(spec, arch, mode)
        print(f"\n  Group {arch}/{mode}: {len(group_expected)} expected metrics")

        for hostname in hosts:
            print(f"  Validating host: {hostname}")
            per_host_metric_results[hostname] = validate_metrics_for_host(
                site, hostname, group_expected,
            )
            present = sum(1 for v in per_host_metric_results[hostname].values() if v)
            print(f"    Result: {present}/{len(group_expected)} present")

    # Step 4: Per-host tag spot-checks
    print("\n── Step 4: Per-host tag spot-checks ──")
    per_host_tag_results: dict[str, list[dict]] = {}

    for _, hosts in sorted(sampled.items()):
        for hostname in hosts:
            print(f"  Tag checks for: {hostname}")
            per_host_tag_results[hostname] = validate_tags_for_host(site, hostname)
            for tr in per_host_tag_results[hostname]:
                status = "PASS" if tr["pass"] else ("N/A" if tr["pass"] is None else "FAIL")
                print(f"    {tr['label']}: {status}")

    # Generate report
    print("\n── Generating report ──")
    report = generate_report(
        expected, deprecated, live_metrics, site, args.arch,
        host_groups, sampled,
        per_host_metric_results, per_host_tag_results,
        spec,
    )

    # Write report
    output_dir = args.output or str(Path(args.spec).parent.parent / "reports")
    os.makedirs(output_dir, exist_ok=True)
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
    site_slug = site.replace(".", "-")
    report_path = os.path.join(output_dir, f"report-{site_slug}-{timestamp}.md")

    with open(report_path, "w") as f:
        f.write(report)

    print(f"\nReport written to: {report_path}")

    # Exit code: non-zero if there are missing expected metrics globally
    missing = set(expected.keys()) - live_metrics
    if missing:
        print(f"\nWARNING: {len(missing)} expected metrics not found in live environment")
        sys.exit(1)
    else:
        print("\nAll expected metrics found!")


if __name__ == "__main__":
    main()
