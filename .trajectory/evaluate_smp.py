#!/usr/bin/env python3
"""Evaluate SMP markers in a temporary Trajectory cache."""

import argparse
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
from contextlib import closing
from pathlib import Path

ASSETS = Path(__file__).resolve().parent
TABLES = ("sessions", "turns", "tool_calls")


def run(binary, environment, *arguments):
    result = subprocess.run([binary, *arguments], env=environment, text=True, capture_output=True, check=False)
    if result.returncode:
        raise RuntimeError(f"Trajectory command failed:\n{result.stdout}{result.stderr}")
    return result.stdout


def insert(connection, table, values):
    columns = ", ".join(f'"{name}"' for name in values)
    placeholders = ", ".join("?" for _ in values)
    connection.execute(f"INSERT INTO {table} ({columns}) VALUES ({placeholders})", tuple(values.values()))


def load_demo(connection):
    cases = json.loads((ASSETS / "smp-demo.json").read_text())
    timestamp = "2026-01-01T00:00:00Z"
    for case in cases:
        session_id = "smp-demo-" + case["id"]
        case["session_id"] = session_id
        tool = case.get("tool")
        insert(
            connection,
            "sessions",
            {
                "session_id": session_id,
                "date": "2026-01-01",
                "started_at": timestamp,
                "ended_at": timestamp,
                "indexed_at": timestamp,
                "client_source": "claude-code",
                "project_dir": "datadog-agent",
                "total_turns": 1,
                "total_tool_calls": int(tool is not None),
                "is_complete": 1,
            },
        )
        insert(
            connection,
            "turns",
            {
                "session_id": session_id,
                "turn_id": 1,
                "started_at": timestamp,
                "ended_at": timestamp,
                "input_prompt": case["prompt"],
                "agent_response": case["response"],
                "content_present": 1,
                "tool_call_count": int(tool is not None),
            },
        )
        if tool:
            insert(
                connection,
                "tool_calls",
                {
                    "session_id": session_id,
                    "turn_id": 1,
                    "tool_use_id": "demo-tool",
                    "tool_name": tool["name"],
                    "started_at": timestamp,
                    "ended_at": timestamp,
                    "input_json": json.dumps({"command": tool["command"]}),
                    "output_text": tool["output"],
                    "success": int(tool["success"]),
                },
            )
    connection.commit()
    return cases


def open_cache(path):
    path = path.expanduser().resolve()
    if not path.is_file():
        raise ValueError(f"Trajectory cache not found: {path}. Check the path and use straight shell quotes.")
    return sqlite3.connect(path.as_uri() + "?mode=ro", uri=True)


def list_sessions(source_path, client=None):
    with closing(open_cache(source_path)) as source:
        rows = source.execute(
            "SELECT s.session_id, s.client_source, COALESCE(MAX(t.started_at), s.started_at, '') AS last_turn, "
            "COUNT(t.turn_id), s.project_dir FROM sessions s "
            "LEFT JOIN turns t ON t.session_id = s.session_id "
            "WHERE (? IS NULL OR s.client_source = ?) GROUP BY s.session_id "
            "ORDER BY last_turn DESC, s.session_id LIMIT 20",
            (client, client),
        ).fetchall()
    print("TRAJECTORY SESSION ID | CLIENT | LAST CAPTURED TURN | TURNS | PROJECT")
    for row in rows:
        print(" | ".join(str(value) if value is not None else "" for value in row))
    if not rows:
        print("No captured sessions match. Check the cache path, client filter, and capture setup.")


def copy_session(connection, source_path, session_id):
    # Copy only the selected session. The source connection cannot write.
    with closing(open_cache(source_path)) as source:
        source.row_factory = sqlite3.Row
        source.execute("BEGIN")
        session = source.execute("SELECT * FROM sessions WHERE session_id = ?", (session_id,)).fetchone()
        if session is None:
            raise ValueError(
                f"Session not found in this Trajectory cache: {session_id}. "
                "Use --list-sessions with the same --cache-db to see captured IDs. "
                "A native client session can exist before Trajectory captures it."
            )
        turns = source.execute("SELECT * FROM turns WHERE session_id = ?", (session_id,)).fetchall()
        # Older captured turns can have an end timestamp without a lifecycle status.
        completed = {row["turn_id"] for row in turns if row["ended_at"] and row["lifecycle_status"] in (None, "", "ok")}
        if not completed:
            raise ValueError("No completed turns are available yet. Wait for a turn to finish and capture to update.")
        for table in TABLES:
            columns = {row[1] for row in connection.execute(f"PRAGMA table_info({table})")}
            for row in source.execute(f"SELECT * FROM {table} WHERE session_id = ?", (session_id,)):
                if table != "sessions" and row["turn_id"] not in completed:
                    continue
                insert(connection, table, {key: row[key] for key in row.keys() if key in columns})
        connection.execute("UPDATE sessions SET total_turns = ? WHERE session_id = ?", (len(completed), session_id))
    connection.commit()
    return {"session_complete": bool(session["is_complete"]), "skipped_turns": len(turns) - len(completed)}


def report(connection, cases, snapshot=None):
    findings = connection.execute(
        "SELECT session_id, turn_id, marker_type FROM markers "
        "WHERE marker_type LIKE 'smp-%' AND kind = 'point' ORDER BY session_id, turn_id, marker_type"
    ).fetchall()
    if cases is not None:
        actual = {}
        for session_id, _, marker in findings:
            actual.setdefault(session_id, []).append(marker)
        failed = False
        print("Synthetic cases (candidate signals, not confirmed feature gaps):")
        for case in cases:
            markers = sorted(actual.get(case["session_id"], []))
            passed = markers == sorted(case["expected"])
            failed |= not passed
            print(f"{'PASS' if passed else 'FAIL'} {case['id']}: {', '.join(markers) or 'none'}")
            if not passed:
                print(f"  expected: {case['expected']}")
        if failed:
            raise RuntimeError("Demo results differ from the expected signals.")
        print(f"\n{len(cases)} cases passed.")
    else:
        print(f"SMP candidate signals: {len(findings)}")
        turns = connection.execute("SELECT COUNT(*) FROM turns").fetchone()[0]
        tools = connection.execute("SELECT COUNT(*) FROM tool_calls").fetchone()[0]
        print(f"Selected session snapshot: turns={turns}, tool calls={tools}.")
        if not snapshot["session_complete"]:
            print("The session is still open. These results cover completed turns only.")
        print(f"Skipped incomplete or unknown turn states: {snapshot['skipped_turns']}.")
    for session_id, turn_id, marker in findings:
        turn = connection.execute(
            "SELECT input_prompt, agent_response FROM turns WHERE session_id = ? AND turn_id = ?",
            (session_id, turn_id),
        ).fetchone()
        print(f"\n{marker} | session={session_id} | turn={turn_id}")
        if turn:
            print(f"  Developer: {turn[0] or ''}")
            print(f"  Agent: {turn[1] or ''}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--demo", action="store_true", help="Evaluate synthetic examples and check expected signals.")
    mode.add_argument("--session", help="Evaluate completed turns from an open or closed local session.")
    mode.add_argument(
        "--list-sessions", action="store_true", help="List captured session IDs without conversation text."
    )
    parser.add_argument("--client", help="Exact client filter for --list-sessions, such as claude-code.")
    parser.add_argument(
        "--cache-db", type=Path, help="Source cache for --session or --list-sessions. Opened read-only."
    )
    parser.add_argument("--trajectory", default="trajectory", help="Trajectory executable (tested with 0.6.2).")
    args = parser.parse_args()
    if bool(args.session or args.list_sessions) != bool(args.cache_db):
        parser.error("Use --cache-db with --session or --list-sessions.")
    if args.client and not args.list_sessions:
        parser.error("Use --client only with --list-sessions.")
    if args.list_sessions:
        list_sessions(args.cache_db, args.client)
        return
    binary = shutil.which(args.trajectory)
    if not binary:
        parser.error("Trajectory is missing. Use --trajectory with its executable path.")
    with tempfile.TemporaryDirectory(prefix="smp-trajectory-") as temporary:
        home = Path(temporary)
        # Isolate marker layers and configuration from the running capture service.
        environment = {key: value for key, value in os.environ.items() if not key.startswith("TRAJECTORY_")}
        environment["TRAJECTORY_HOME"] = str(home)
        environment["TRAJECTORY_PROJECT_MARKERS_PATH"] = str(ASSETS / "markers.yaml")
        database = home / "cache.db"
        run(binary, environment, "patterns", "markers", "validate", "--config", str(ASSETS / "markers.yaml"))
        # The installed binary creates its own current schema in an empty cache.
        run(binary, environment, "patterns", "markers", "reevaluate", "--db", str(database))
        with closing(sqlite3.connect(database)) as connection:
            cases = load_demo(connection) if args.demo else None
            snapshot = None
            if args.session:
                snapshot = copy_session(connection, args.cache_db, args.session)
            run(binary, environment, "patterns", "markers", "reevaluate", "--db", str(database))
            report(connection, cases, snapshot)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, ValueError, sqlite3.Error) as error:
        print(f"Evaluation failed: {error}", file=sys.stderr)
        sys.exit(1)
