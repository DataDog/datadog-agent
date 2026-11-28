"""
Local end-to-end harness for CWS: a locally built system-probe sends its events
to an HTTP sink on 127.0.0.1 instead of the Datadog intake.

This module holds the logic that does not need invoke. It is also the sink
itself: `python3 -I local_e2e.py sink <events-file> <port>`.
"""

from __future__ import annotations

import gzip
import json
import sys
import time
import zlib
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

# Unix socket paths are limited to 108 bytes, so everything lives under /run
# rather than in a (usually long) worktree or scratch path.
STATE_DIR = Path("/run/cws-local-e2e")
BPF_DIR = STATE_DIR / "ebpf"
POLICIES_DIR = STATE_DIR / "policies"
EVENTS_FILE = STATE_DIR / "events.jsonl"
SYSPROBE_LOG = STATE_DIR / "system-probe.log"
SINK_LOG = STATE_DIR / "sink.log"
STATE_FILE = STATE_DIR / "state.json"
SYSPROBE_PIDFILE = STATE_DIR / "system-probe.pid"

DEFAULT_PORT = 18080
HOSTNAME = "cws-local-e2e"

# The runtime-security objects system-probe loads for CWS. The `-baloum`
# variants are C unit-test builds and must not be picked up.
CWS_OBJECTS = (
    "runtime-security.o",
    "runtime-security-fentry.o",
    "runtime-security-syscall-wrapper.o",
    "runtime-security-offset-guesser.o",
)


def datadog_yaml(port: int) -> dict:
    return {
        "api_key": "0" * 32,
        "hostname": HOSTNAME,
        "log_level": "info",
        # No core agent to ask: with its IPC port set, system-probe spends
        # ~50s of startup retrying the core agent for its hostname.
        "cmd_port": 0,
        "remote_configuration": {"enabled": False},
        # On by default; without a core agent it waits 60s for an IPC auth
        # token and then kills system-probe.
        "remote_agent": {"configstream": {"consumer": {"enabled": False}}},
        # Read from datadog.yaml only: placed in system-probe.yaml it is
        # silently ignored and events go to the real intake.
        "runtime_security_config": {
            "endpoints": {
                "logs_dd_url": f"127.0.0.1:{port}",
                "logs_no_ssl": True,
                "use_http": True,
                "use_compression": False,
            }
        },
    }


def system_probe_yaml() -> dict:
    return {
        "system_probe_config": {
            "bpf_dir": str(BPF_DIR),
            "sysprobe_socket": str(STATE_DIR / "sysprobe.sock"),
        },
        # CWS only: other modules load their own eBPF objects, which are not
        # staged, and add noise to the log.
        "discovery": {"enabled": False},
        "runtime_security_config": {
            "enabled": True,
            "direct_send_from_system_probe": True,
            "socket": str(STATE_DIR / "runtime-security.sock"),
            "policies": {"dir": str(POLICIES_DIR)},
            "remote_configuration": {"enabled": False},
            # Off so the only events in the sink are the ones the policy asks
            # for; re-enable with --set when testing these features.
            "self_test": {"enabled": False},
            "activity_dump": {"enabled": False},
            "security_profile": {"enabled": False},
        },
    }


def apply_override(config: dict, override: str) -> None:
    """Apply a `dotted.key=value` override in place; the value is parsed as YAML."""
    import yaml

    key, sep, raw = override.partition("=")
    if not sep or not key:
        raise ValueError(f"invalid override {override!r}, expected dotted.key=value")
    *parents, leaf = key.split(".")
    node = config
    for part in parents:
        child = node.setdefault(part, {})
        if not isinstance(child, dict):
            raise ValueError(f"invalid override {override!r}: {part} is not a mapping")
        node = child
    node[leaf] = yaml.safe_load(raw)


def read_events(path: Path = EVENTS_FILE) -> list[dict]:
    """Return the sink records, oldest first."""
    if not path.exists():
        return []
    with open(path, "rb") as f:
        data = f.read()
    records = []
    # A record being written concurrently has no trailing newline yet; it is
    # picked up on the next poll.
    for line in data.split(b"\n")[:-1]:
        if line:
            records.append(json.loads(line))
    return records


def rule_id(event: dict) -> str | None:
    agent = event.get("agent")
    return agent.get("rule_id") if isinstance(agent, dict) else None


def event_time(event: dict) -> float | None:
    """The event's own timestamp (`date`, RFC 3339 with nanoseconds) as a Unix time."""
    from datetime import datetime

    date = event.get("date")
    if not isinstance(date, str):
        return None
    # fromisoformat before 3.11 takes neither a `Z` suffix nor more than six
    # fractional digits.
    date = date.replace("Z", "+00:00")
    head, dot, rest = date.partition(".")
    if dot:
        digits = len(rest) - len(rest.lstrip("0123456789"))
        date = f"{head}.{rest[:min(digits, 6)]}{rest[digits:]}"
    try:
        return datetime.fromisoformat(date).timestamp()
    except ValueError:
        return None


def lookup(event: dict, dotted: str):
    """Resolve `a.b.c` in nested dicts; a numeric part indexes a list."""
    node = event
    for part in dotted.split("."):
        if isinstance(node, dict) and part in node:
            node = node[part]
        elif isinstance(node, list) and part.isdigit() and int(part) < len(node):
            node = node[int(part)]
        else:
            raise KeyError(dotted)
    return node


def check_expectations(event: dict, expects: list[str]) -> list[str]:
    """Return one message per `dotted.path=value` expectation the event fails."""
    failures = []
    for expect in expects:
        path, sep, want = expect.partition("=")
        if not sep:
            raise ValueError(f"invalid expectation {expect!r}, expected dotted.path=value")
        try:
            got = lookup(event, path)
        except KeyError:
            # Paths are into the serialized event, which does not mirror SECL
            # field names (e.g. SECL process.file.name is process.executable.name).
            failures.append(f"{path}: missing (want {want!r}; top-level keys: {', '.join(event)})")
            continue
        # Compare as JSON scalars so `--expect exit.code=0` and
        # `--expect file.in_upper_layer=false` work without quoting rules.
        rendered = got if isinstance(got, str) else json.dumps(got)
        if rendered != want:
            failures.append(f"{path}: got {rendered!r}, want {want!r}")
    return failures


def rule_statuses(ruleset_loaded: dict) -> list[tuple[str, str, str, str]]:
    """(policy, rule id, status, message) for every file-sourced rule in a ruleset_loaded event."""
    out = []
    for policy in ruleset_loaded.get("policies", []):
        if policy.get("source") != "file":
            continue
        for rule in policy.get("rules", []):
            out.append(
                (
                    policy.get("name", "?"),
                    rule.get("id", "?"),
                    rule.get("status", "?"),
                    rule.get("message", ""),
                )
            )
    return out


class _SinkHandler(BaseHTTPRequestHandler):
    events_file: Path

    def do_POST(self):  # noqa: V105 (called by BaseHTTPRequestHandler)
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        encoding = self.headers.get("Content-Encoding", "")
        try:
            if encoding == "gzip":
                body = gzip.decompress(body)
            elif encoding == "deflate":
                body = zlib.decompress(body)
            entries = json.loads(body)
        except Exception as e:
            print(f"undecodable payload on {self.path} (encoding={encoding!r}): {e}", flush=True)
            entries = []
        lines = []
        for entry in entries if isinstance(entries, list) else [entries]:
            message = entry.get("message", "") if isinstance(entry, dict) else ""
            try:
                event = json.loads(message)
            except (TypeError, ValueError):
                event = {"raw_message": message}
            # One line per event, written in a single call so a concurrent
            # reader never sees half a record followed by a newline.
            record = {"received": time.time(), "path": self.path, "event": event}
            lines.append(json.dumps(record) + "\n")
        if lines:
            with open(self.events_file, "a") as f:
                f.write("".join(lines))
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"{}")

    def log_message(self, *args):  # noqa: V105 (silences per-request stderr logging)
        pass


def serve_sink(events_file: Path, port: int) -> None:
    handler = type("SinkHandler", (_SinkHandler,), {"events_file": events_file})
    HTTPServer(("127.0.0.1", port), handler).serve_forever()


if __name__ == "__main__":
    if len(sys.argv) != 4 or sys.argv[1] != "sink":
        sys.exit("usage: local_e2e.py sink <events-file> <port>")
    serve_sink(Path(sys.argv[2]), int(sys.argv[3]))
