#!/usr/bin/env python3
"""
Extract all CLI commands from Datadog Agent cobra command definitions.
Scans cmd/**/*.go and pkg/cli/subcommands/**/*.go for cobra.Command structs.
"""

import os
import re
import json
from pathlib import Path
from collections import defaultdict
from typing import Dict, List, Optional, Tuple

# Root of the datadog-agent repo
REPO_ROOT = Path(__file__).parent.parent.parent

# Directories to scan for commands
SCAN_DIRS = [
    REPO_ROOT / "cmd",
    REPO_ROOT / "pkg" / "cli" / "subcommands",
]

# Pattern to match cobra.Command struct fields
# Matches: Use: "command-name", Short: "description", etc.
FIELD_PATTERNS = {
    "use": re.compile(r'Use:\s*[`"]([^`"]+)[`"]', re.MULTILINE),
    "short": re.compile(r'Short:\s*[`"]([^`"]+)[`"]', re.MULTILINE),
    "long": re.compile(r'Long:\s*[`"]([^`"]+)[`"]', re.MULTILINE),
    "aliases": re.compile(r'Aliases:\s*\[\]string\{([^}]+)\}', re.MULTILINE),
}

# Binary name mappings from directory
BINARY_NAMES = {
    "agent": "agent",
    "cluster-agent": "cluster-agent",
    "cluster-agent-cloudfoundry": "cluster-agent-cloudfoundry",
    "dogstatsd": "dogstatsd",
    "trace-agent": "trace-agent",
    "system-probe": "system-probe",
    "security-agent": "security-agent",
    "process-agent": "process-agent",
    "otel-agent": "otel-agent",
    "installer": "installer",
    "cws-instrumentation": "cws-instrumentation",
    "secrethelper": "secrethelper",
    "serverless-init": "serverless-init",
    "iot-agent": "iot-agent",
    "sbomgen": "sbomgen",
    "host-profiler": "host-profiler",
    "systray": "systray",
    "loader": "loader",
    "privateactionrunner": "privateactionrunner",
}


def extract_commands_from_file(filepath: Path) -> List[Dict]:
    """Extract cobra command definitions from a Go file."""
    commands = []

    try:
        content = filepath.read_text(encoding='utf-8')
    except Exception as e:
        print(f"Error reading {filepath}: {e}")
        return commands

    # Skip test files
    if "_test.go" in filepath.name:
        return commands

    # Find all Use: fields as anchors for commands
    use_matches = list(FIELD_PATTERNS["use"].finditer(content))

    for use_match in use_matches:
        use_value = use_match.group(1).strip()
        use_pos = use_match.start()

        # Look for Short: near this Use: (within ~500 chars)
        search_region = content[max(0, use_pos - 200):use_pos + 500]

        short_match = FIELD_PATTERNS["short"].search(search_region)
        short_value = short_match.group(1).strip() if short_match else ""

        # Skip internal/empty commands
        if not use_value or use_value.startswith("_"):
            continue

        # Determine the binary and subcommand from file path
        rel_path = filepath.relative_to(REPO_ROOT)
        parts = rel_path.parts

        binary = None
        subcommand_path = []

        if "cmd" in parts:
            cmd_idx = parts.index("cmd")
            if cmd_idx + 1 < len(parts):
                binary = parts[cmd_idx + 1]
                # Get subcommand path
                if "subcommands" in parts:
                    sub_idx = parts.index("subcommands")
                    subcommand_path = list(parts[sub_idx + 1:-1])  # exclude filename
        elif "pkg" in parts and "cli" in parts and "subcommands" in parts:
            # Shared commands in pkg/cli/subcommands
            sub_idx = parts.index("subcommands")
            if sub_idx + 1 < len(parts):
                subcommand_path = [parts[sub_idx + 1]]
            binary = "shared"

        commands.append({
            "binary": binary or "unknown",
            "use": use_value,
            "short": short_value,
            "subcommand_path": subcommand_path,
            "source_file": str(rel_path),
            "line": content[:use_pos].count('\n') + 1,
        })

    return commands


def scan_directories() -> List[Dict]:
    """Scan all directories for command definitions."""
    all_commands = []

    for scan_dir in SCAN_DIRS:
        if not scan_dir.exists():
            print(f"Directory not found: {scan_dir}")
            continue

        for go_file in scan_dir.rglob("*.go"):
            commands = extract_commands_from_file(go_file)
            all_commands.extend(commands)

    return all_commands


def organize_commands(commands: List[Dict]) -> Dict[str, List[Dict]]:
    """Organize commands by binary."""
    by_binary = defaultdict(list)

    for cmd in commands:
        binary = cmd["binary"]
        by_binary[binary].append(cmd)

    # Sort commands within each binary
    for binary in by_binary:
        by_binary[binary].sort(key=lambda x: x["use"])

    return dict(by_binary)


def deduplicate_commands(commands: List[Dict]) -> List[Dict]:
    """Remove duplicate commands (same use value in same binary)."""
    seen = set()
    unique = []

    for cmd in commands:
        key = (cmd["binary"], cmd["use"])
        if key not in seen:
            seen.add(key)
            unique.append(cmd)

    return unique


def generate_markdown(organized: Dict[str, List[Dict]]) -> str:
    """Generate markdown documentation from organized commands."""

    lines = [
        "# Datadog Agent CLI Commands Reference",
        "",
        "Auto-generated from cobra command definitions in the source code.",
        "",
        "---",
        "",
    ]

    # Summary table
    lines.extend([
        "## Summary",
        "",
        "| Binary | Command Count |",
        "|--------|---------------|",
    ])

    # Sort binaries with main ones first
    priority_order = ["agent", "cluster-agent", "system-probe", "security-agent",
                      "process-agent", "trace-agent", "dogstatsd", "otel-agent"]

    def sort_key(binary):
        if binary in priority_order:
            return (0, priority_order.index(binary))
        elif binary == "shared":
            return (2, 0)
        else:
            return (1, binary)

    sorted_binaries = sorted(organized.keys(), key=sort_key)

    for binary in sorted_binaries:
        cmds = organized[binary]
        lines.append(f"| `{binary}` | {len(cmds)} |")

    lines.extend(["", "---", ""])

    # Detailed sections for each binary
    for binary in sorted_binaries:
        cmds = organized[binary]

        if binary == "shared":
            lines.append("## Shared Commands (pkg/cli/subcommands)")
            lines.append("")
            lines.append("These commands are reused across multiple binaries.")
        else:
            lines.append(f"## {binary}")

        lines.extend(["", "| Command | Description | Source |", "|---------|-------------|--------|"])

        for cmd in cmds:
            use = cmd["use"].replace("|", "\\|")
            short = cmd["short"].replace("|", "\\|")
            source = cmd["source_file"]
            line_num = cmd["line"]

            # Format the full command
            if binary != "shared" and binary != "unknown":
                full_cmd = f"`{binary} {use}`"
            else:
                full_cmd = f"`{use}`"

            lines.append(f"| {full_cmd} | {short} | `{source}:{line_num}` |")

        lines.extend(["", "---", ""])

    # MCP tool categories
    lines.extend([
        "## MCP Tool Categories",
        "",
        "Commands organized by use case for MCP server implementation:",
        "",
        "### High-Value Troubleshooting (Priority 1)",
        "- `agent status` - Overall agent health and status",
        "- `agent health` - Health check results",
        "- `agent diagnose` - Installation and configuration validation",
        "- `agent flare` - Collect diagnostic bundle",
        "- `agent config` - Runtime configuration",
        "- `agent check <name>` - Run specific check",
        "- `agent tagger-list` - View entity tags",
        "- `agent workload-list` - View detected workloads",
        "",
        "### Configuration & Runtime (Priority 2)",
        "- `agent config list-runtime` - List runtime settings",
        "- `agent config get/set` - Get/set runtime values",
        "- `agent configcheck` - Validate loaded configs",
        "- `agent secret` - Secret management",
        "",
        "### System-Level Inspection (Priority 3)",
        "- `system-probe debug` - System probe state",
        "- `system-probe ebpf map list/dump` - eBPF inspection",
        "- `system-probe runtime process-cache dump` - Process cache",
        "",
        "### Security & Compliance (Priority 4)",
        "- `security-agent status` - Security agent status",
        "- `system-probe compliance check` - Run compliance checks",
        "- `system-probe runtime security-profile list` - Active profiles",
        "",
        "---",
        "",
        "## Notes for MCP Implementation",
        "",
        "1. **Common Parameters**: Most commands accept `-c <config_path>` for config file",
        "2. **Output Formats**: Many commands support `--json` for JSON output",
        "3. **Container Context**: In K8s, run via `kubectl exec <pod> -- agent <cmd>`",
        "4. **Permissions**: Some commands require root (especially system-probe)",
        "5. **Agent Running**: Most diagnostic commands need the agent running",
        "",
    ])

    return "\n".join(lines)


def main():
    print("Scanning for cobra commands...")

    # Scan all directories
    all_commands = scan_directories()
    print(f"Found {len(all_commands)} raw command definitions")

    # Deduplicate
    unique_commands = deduplicate_commands(all_commands)
    print(f"After deduplication: {len(unique_commands)} unique commands")

    # Organize by binary
    organized = organize_commands(unique_commands)

    # Print summary
    print("\nCommands by binary:")
    for binary, cmds in sorted(organized.items()):
        print(f"  {binary}: {len(cmds)} commands")

    # Generate markdown
    markdown = generate_markdown(organized)

    # Write to commands.md
    output_path = Path(__file__).parent / "commands.md"
    output_path.write_text(markdown, encoding='utf-8')
    print(f"\nWritten to: {output_path}")

    # Also write JSON for programmatic use
    json_path = Path(__file__).parent / "commands.json"
    json_data = {
        "total_commands": len(unique_commands),
        "by_binary": {k: len(v) for k, v in organized.items()},
        "commands": unique_commands,
    }
    json_path.write_text(json.dumps(json_data, indent=2), encoding='utf-8')
    print(f"Written to: {json_path}")


if __name__ == "__main__":
    main()
