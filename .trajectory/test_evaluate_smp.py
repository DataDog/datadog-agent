"""Check that session evaluation preserves the source cache."""

import hashlib
import io
import sqlite3
import tempfile
import unittest
from contextlib import closing, redirect_stdout
from pathlib import Path

from evaluate_smp import copy_session, list_sessions

SCHEMA = (
    "CREATE TABLE sessions (session_id TEXT PRIMARY KEY, is_complete INTEGER, total_turns INTEGER);"
    "CREATE TABLE turns (session_id TEXT, turn_id INTEGER, input_prompt TEXT, ended_at TEXT, lifecycle_status TEXT);"
    "CREATE TABLE tool_calls (session_id TEXT, turn_id INTEGER, tool_use_id TEXT, input_json TEXT);"
)


class CopySessionTest(unittest.TestCase):
    def test_copy_is_scoped_and_preserves_source(self):
        with tempfile.TemporaryDirectory() as temporary:
            source_path = Path(temporary) / "source.db"
            with closing(sqlite3.connect(source_path)) as source:
                source.executescript(
                    SCHEMA + "INSERT INTO sessions VALUES ('selected', 1, 1), ('other', 1, 1);"
                    "INSERT INTO turns VALUES ('selected', 1, 'selected prompt', '2026-01-01', 'ok'),"
                    "('other', 1, 'private prompt', '2026-01-01', NULL);"
                    "INSERT INTO tool_calls VALUES ('selected', 1, 'a', '{}'), ('other', 1, 'b', '{}');"
                )
            before = hashlib.sha256(source_path.read_bytes()).hexdigest()
            with closing(sqlite3.connect(":memory:")) as target:
                target.executescript(SCHEMA)
                copy_session(target, source_path, "selected")
                for table in ("sessions", "turns", "tool_calls"):
                    self.assertEqual(target.execute(f"SELECT session_id FROM {table}").fetchall(), [("selected",)])
                self.assertEqual(target.execute("SELECT input_prompt FROM turns").fetchone(), ("selected prompt",))
            self.assertEqual(before, hashlib.sha256(source_path.read_bytes()).hexdigest())

    def test_open_session_excludes_unfinished_turns_and_their_tools(self):
        with tempfile.TemporaryDirectory() as temporary:
            source_path = Path(temporary) / "source.db"
            with closing(sqlite3.connect(source_path)) as source:
                source.executescript(
                    SCHEMA + "INSERT INTO sessions VALUES ('selected', 0, 3);"
                    "INSERT INTO turns VALUES ('selected', 1, 'finished', '2026-01-01', 'ok'),"
                    "('selected', 2, 'unfinished', NULL, NULL),"
                    "('selected', 3, 'unknown', '2026-01-01', 'unknown');"
                    "INSERT INTO tool_calls VALUES ('selected', 1, 'finished', '{}'),"
                    "('selected', 2, 'unfinished', '{}'), ('selected', 3, 'unknown', '{}');"
                )
            before = source_path.read_bytes()
            with closing(sqlite3.connect(":memory:")) as target:
                target.executescript(SCHEMA)
                snapshot = copy_session(target, source_path, "selected")
                self.assertEqual(snapshot, {"session_complete": False, "skipped_turns": 2})
                self.assertEqual(target.execute("SELECT is_complete, total_turns FROM sessions").fetchall(), [(0, 1)])
                self.assertEqual(target.execute("SELECT turn_id FROM turns").fetchall(), [(1,)])
                self.assertEqual(target.execute("SELECT tool_use_id FROM tool_calls").fetchall(), [("finished",)])
            self.assertEqual(before, source_path.read_bytes())

    def test_no_finished_turn_is_not_reported_as_zero_findings(self):
        with tempfile.TemporaryDirectory() as temporary:
            source_path = Path(temporary) / "source.db"
            with closing(sqlite3.connect(source_path)) as source:
                source.executescript(
                    SCHEMA + "INSERT INTO sessions VALUES ('selected', 0, 1);"
                    "INSERT INTO turns VALUES ('selected', 1, 'unfinished', NULL, NULL);"
                )
            with closing(sqlite3.connect(":memory:")) as target:
                target.executescript(SCHEMA)
                with self.assertRaisesRegex(ValueError, "No completed turns"):
                    copy_session(target, source_path, "selected")
                self.assertEqual(target.execute("SELECT COUNT(*) FROM sessions").fetchone()[0], 0)

    def test_missing_session_points_to_captured_id_lookup(self):
        with tempfile.TemporaryDirectory() as temporary:
            source_path = Path(temporary) / "source.db"
            with closing(sqlite3.connect(source_path)) as source:
                source.executescript(SCHEMA)
            with closing(sqlite3.connect(":memory:")) as target:
                target.executescript(SCHEMA)
                with self.assertRaisesRegex(ValueError, "--list-sessions"):
                    copy_session(target, source_path, "native-id-not-captured")


class ListSessionsTest(unittest.TestCase):
    def test_list_filters_clients_uses_latest_turn_and_excludes_content(self):
        with tempfile.TemporaryDirectory() as temporary:
            source_path = Path(temporary) / "source.db"
            with closing(sqlite3.connect(source_path)) as source:
                source.executescript(
                    "CREATE TABLE sessions (session_id TEXT, client_source TEXT, started_at TEXT, project_dir TEXT);"
                    "CREATE TABLE turns (session_id TEXT, turn_id INTEGER, started_at TEXT, input_prompt TEXT);"
                    "INSERT INTO sessions VALUES ('claude-older', 'claude-code', '2026-01-01', '/repo'),"
                    "('claude-newer', 'claude-code', '2026-01-02', '/repo'),"
                    "('codex', 'codex', '2026-01-05', '/other');"
                    "INSERT INTO turns VALUES ('claude-older', 1, '2026-01-04', 'private conversation');"
                )
            before = source_path.read_bytes()
            output = io.StringIO()
            with redirect_stdout(output):
                list_sessions(source_path, "claude-code")
            text = output.getvalue()
            self.assertIn("claude-older | claude-code | 2026-01-04 | 1 | /repo", text)
            self.assertLess(text.index("claude-older"), text.index("claude-newer"))
            self.assertNotIn("codex", text)
            self.assertNotIn("private conversation", text)
            self.assertEqual(before, source_path.read_bytes())


if __name__ == "__main__":
    unittest.main()
