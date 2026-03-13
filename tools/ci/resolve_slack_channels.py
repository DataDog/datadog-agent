"""
Resolve Slack channel IDs for github_slack_map.yaml and github_slack_review_map.yaml.

Usage:
    SLACK_TOKEN=$(pbpaste) python tools/ci/resolve_slack_channels.py
"""

import os
import re
import sys
import time

import requests

SLACK_TOKEN = os.environ.get("SLACK_TOKEN")
if not SLACK_TOKEN:
    print("Error: SLACK_TOKEN environment variable is required", file=sys.stderr)
    sys.exit(1)

FILES = [
    "tasks/libs/pipeline/github_slack_map.yaml",
    "tasks/libs/pipeline/github_slack_review_map.yaml",
]

DEFAULT_SLACK_CHANNEL = "#agent-devx-ops"


def main():
    print("Fetching Slack channels...")
    channel_ids = fetch_all_channels()
    print(f"Found {len(channel_ids)} channels")

    for path in FILES:
        print(f"Processing {path}...")
        process_file(path, channel_ids)
        print(f"  Updated {path}")

    print("Done.")


def fetch_all_channels():
    """Fetch all Slack channels using conversations.list with pagination."""
    channels = {}
    cursor = None
    while True:
        params = {"types": "public_channel,private_channel", "limit": 1000}
        if cursor:
            params["cursor"] = cursor
        for attempt in range(5):
            resp = requests.get(
                "https://slack.com/api/conversations.list",
                headers={"Authorization": f"Bearer {SLACK_TOKEN}"},
                params=params,
            )
            data = resp.json()
            if data.get("ok"):
                break
            if data.get("error") == "ratelimited":
                wait = int(resp.headers.get("Retry-After", 30))
                print(f"Rate limited, waiting {wait}s (attempt {attempt + 1}/5)...")
                time.sleep(wait)
                continue
            print(f"Slack API error: {data.get('error')}", file=sys.stderr)
            sys.exit(1)
        else:
            print("Failed after 5 rate-limit retries", file=sys.stderr)
            sys.exit(1)
        for ch in data.get("channels", []):
            channels[f"#{ch['name']}"] = ch["id"]
        cursor = data.get("response_metadata", {}).get("next_cursor")
        if not cursor:
            break
        time.sleep(2)  # rate limit courtesy
    return channels


def process_file(path, channel_ids):
    header_lines, entries = parse_file(path)

    missing = []
    with open(path, "w") as f:
        for line in header_lines:
            f.write(line + "\n")
        for team, channel_name in entries:
            resolved = DEFAULT_SLACK_CHANNEL if channel_name == "DEFAULT_SLACK_CHANNEL" else channel_name
            ch_id = channel_ids.get(resolved, "")
            if not ch_id:
                missing.append((team, resolved))
            f.write(f"'{team}':\n")
            f.write(f"  name: '{channel_name}'\n")
            f.write(f"  id: '{ch_id}'\n")

    if missing:
        print(f"Warning: could not resolve channels in {path}:", file=sys.stderr)
        for team, name in missing:
            print(f"  {team} -> {name}", file=sys.stderr)


TEAM_RE = re.compile(r"^'([^']+)':\s*(.*)$")
NAME_RE = re.compile(r"^\s+name:\s*(?:'([^']*)'|(\S+))")


def parse_file(path):
    """Parse the YAML file preserving order. Handles both formats:
    - Single-line:  '@team': '#channel'
    - Multi-line:   '@team':\\n  name: '#channel'
    Returns (header_lines, entries).
    """
    header_lines = []
    entries = []
    with open(path) as f:
        lines = [line.rstrip("\n") for line in f]

    i = 0
    # Collect header comments
    while i < len(lines) and (lines[i].startswith("#") or lines[i] == ""):
        if lines[i].startswith("#"):
            header_lines.append(lines[i])
        i += 1

    # Parse entries
    while i < len(lines):
        line = lines[i]
        m = TEAM_RE.match(line)
        if m:
            team = m.group(1)
            inline_value = m.group(2).strip()
            if inline_value:
                # Single-line format: '@team': '#channel' or '@team': DEFAULT_SLACK_CHANNEL
                val = inline_value.strip("'\"")
                entries.append((team, val))
                i += 1
            else:
                # Multi-line format: look for name: on next line
                i += 1
                val = ""
                while i < len(lines) and lines[i].startswith("  "):
                    nm = NAME_RE.match(lines[i])
                    if nm:
                        val = nm.group(1) if nm.group(1) is not None else nm.group(2)
                    i += 1
                entries.append((team, val))
        else:
            i += 1

    return header_lines, entries


if __name__ == "__main__":
    main()
