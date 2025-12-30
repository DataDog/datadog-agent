#!/usr/bin/env python3
"""
Fetch static quality gate relative on-disk size deltas from Datadog
and display them in a table with PR links.

Usage:
    dd-auth -- bazel run //bazel/tools:fetch_size_deltas

Requires DD_API_KEY and DD_APP_KEY environment variables to be set.
"""

import json
import os
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

GITHUB_REPO_URL = "https://github.com/DataDog/datadog-agent/pull"

# Datadog API URL mapping for different sites
DATADOG_SITES = {
    "datadoghq.com": "https://api.datadoghq.com",
    "us3.datadoghq.com": "https://api.us3.datadoghq.com",
    "us5.datadoghq.com": "https://api.us5.datadoghq.com",
    "datadoghq.eu": "https://api.datadoghq.eu",
    "ap1.datadoghq.com": "https://api.ap1.datadoghq.com",
    "ap2.datadoghq.com": "https://api.ap2.datadoghq.com",
    "ddog-gov.com": "https://api.ddog-gov.com",
}


def http_get(url: str, headers: dict | None = None, timeout: int = 30) -> dict:
    """Make an HTTP GET request and return JSON response."""
    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def query_datadog_metrics(days_back: int = 7) -> list[dict]:
    """Query Datadog for relative_on_disk_size metrics on main branch."""
    api_key = os.environ.get("DD_API_KEY")
    app_key = os.environ.get("DD_APP_KEY")
    dd_site = os.environ.get("DD_SITE", "datadoghq.com")

    if not api_key or not app_key:
        raise OSError(
            "DD_API_KEY and DD_APP_KEY environment variables must be set.\n"
            "Use dd-auth to authenticate:\n"
            "  dd-auth -- bazel run //bazel/tools:fetch_size_deltas"
        )

    # Get API base URL for the site
    api_base_url = DATADOG_SITES.get(dd_site, f"https://api.{dd_site}")

    now = int(time.time())
    from_time = now - (days_back * 24 * 60 * 60)

    headers = {
        "DD-API-KEY": api_key,
        "DD-APPLICATION-KEY": app_key,
    }

    params = urllib.parse.urlencode(
        {
            "query": "avg:datadog.agent.static_quality_gate.relative_on_disk_size{git_ref:main} by {ci_commit_sha}",
            "from": from_time,
            "to": now,
        }
    )

    api_url = f"{api_base_url}/api/v1/query?{params}"
    data = http_get(api_url, headers=headers)

    results = []
    for series in data.get("series", []):
        scope = series.get("scope", "")
        # Parse scope - format is "ci_commit_sha:abc123,git_ref:main"
        commit_sha = None
        for tag in scope.split(","):
            if tag.startswith("ci_commit_sha:"):
                commit_sha = tag.replace("ci_commit_sha:", "")
                break

        if not commit_sha:
            continue

        points = [(p[0], p[1]) for p in series.get("pointlist", []) if p[1] is not None]
        if points:
            avg_delta = sum(p[1] for p in points) / len(points)
            results.append({"commit_sha": commit_sha, "delta": avg_delta, "point_count": len(points)})

    return results


def get_commit_info_from_github(commit_sha: str) -> tuple[str, str | None]:
    """Fetch commit info from GitHub API."""
    github_api_url = f"https://api.github.com/repos/DataDog/datadog-agent/commits/{commit_sha}"
    data = http_get(github_api_url, timeout=10)
    message = data.get("commit", {}).get("message", "").split("\n")[0]
    pr_match = re.search(r"\(#(\d+)\)$", message)
    pr_number = pr_match.group(1) if pr_match else None
    return message, pr_number


def get_commit_info(commit_sha: str, repo_dir: str | None = None) -> tuple[str, str | None]:
    """Get commit message and extract PR number from git or GitHub API."""
    # Determine repo directory - default to current working directory
    if repo_dir is None:
        repo_dir = os.getcwd()

    # Try local git first
    result = subprocess.run(
        ["git", "log", "--format=%s", commit_sha, "-1"],
        capture_output=True,
        text=True,
        cwd=repo_dir,
    )

    if result.returncode == 0 and result.stdout.strip():
        message = result.stdout.strip()
        pr_match = re.search(r"\(#(\d+)\)$", message)
        pr_number = pr_match.group(1) if pr_match else None
        return message, pr_number

    # Fallback to GitHub API
    try:
        return get_commit_info_from_github(commit_sha)
    except Exception as e:
        return f"Commit {commit_sha[:8]} not found ({e})", None


def format_delta(delta: float) -> str:
    """Format delta with sign."""
    if delta > 0:
        return f"+{delta:.2f}"
    elif delta < 0:
        return f"{delta:.2f}"
    else:
        return "0.00"


def print_table(data: list[dict]):
    """Print results as a formatted table."""
    # Table header
    print()
    print("=" * 100)
    print("Static Quality Gate - Relative On-Disk Size Deltas (main branch)")
    print("=" * 100)
    print()
    print(f"{'Delta (bytes)':<18} {'Commit SHA':<12} {'PR':<8} {'Title'}")
    print("-" * 100)

    for row in data:
        delta_str = format_delta(row["delta"])
        short_sha = row["commit_sha"][:8]
        pr_num = row.get("pr_number", "N/A")
        pr_display = f"#{pr_num}" if pr_num else "N/A"
        title = row.get("title", "Unknown")[:50]

        print(f"{delta_str:<18} {short_sha:<12} {pr_display:<8} {title}")

    print("-" * 100)
    print()
    print("PR Links:")
    for row in data:
        if row.get("pr_number"):
            print(f"  #{row['pr_number']}: {GITHUB_REPO_URL}/{row['pr_number']}")
    print()


def main():
    print("Fetching metrics from Datadog...")
    metrics_data = query_datadog_metrics(days_back=7)

    if not metrics_data:
        print("No data found for the specified time range.")
        return

    print(f"Found {len(metrics_data)} commits with data.")
    print("Fetching latest commits from origin/main...")

    # Determine repo directory - use BUILD_WORKSPACE_DIRECTORY if available (bazel run)
    repo_dir = os.environ.get("BUILD_WORKSPACE_DIRECTORY", os.getcwd())

    # Fetch latest from origin to ensure we have the commits
    subprocess.run(
        ["git", "fetch", "origin", "main"],
        capture_output=True,
        cwd=repo_dir,
        check=False,
    )

    print("Fetching commit information from git...")

    # Enrich with git commit info
    for item in metrics_data:
        title, pr_number = get_commit_info(item["commit_sha"], repo_dir)
        item["title"] = title
        item["pr_number"] = pr_number

    # Sort by delta descending
    metrics_data.sort(key=lambda x: x["delta"], reverse=True)

    # Print the table
    print_table(metrics_data)


if __name__ == "__main__":
    main()
