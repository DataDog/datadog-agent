"""Check that session evaluation preserves the source cache."""

import hashlib
import sqlite3
import tempfile
import unittest
from contextlib import closing
from pathlib import Path

from evaluate_smp import copy_session


class CopySessionTest(unittest.TestCase):
    def test_copy_is_scoped_and_preserves_source(self):
        with tempfile.TemporaryDirectory() as temporary:
            source_path = Path(temporary) / "source.db"
            with closing(sqlite3.connect(source_path)) as source:
                source.executescript(
                    "CREATE TABLE sessions (session_id TEXT PRIMARY KEY, is_complete INTEGER);"
                    "CREATE TABLE turns (session_id TEXT, turn_id INTEGER, input_prompt TEXT);"
                    "CREATE TABLE tool_calls (session_id TEXT, tool_use_id TEXT, input_json TEXT);"
                    "INSERT INTO sessions VALUES ('selected', 1), ('other', 1);"
                    "INSERT INTO turns VALUES ('selected', 1, 'selected prompt'), ('other', 1, 'private prompt');"
                    "INSERT INTO tool_calls VALUES ('selected', 'a', '{}'), ('other', 'b', '{}');"
                )
            before = hashlib.sha256(source_path.read_bytes()).hexdigest()
            with closing(sqlite3.connect(":memory:")) as target:
                target.executescript(
                    "CREATE TABLE sessions (session_id TEXT PRIMARY KEY, is_complete INTEGER);"
                    "CREATE TABLE turns (session_id TEXT, turn_id INTEGER, input_prompt TEXT);"
                    "CREATE TABLE tool_calls (session_id TEXT, tool_use_id TEXT, input_json TEXT);"
                )
                copy_session(target, source_path, "selected")
                for table in ("sessions", "turns", "tool_calls"):
                    self.assertEqual(target.execute(f"SELECT session_id FROM {table}").fetchall(), [("selected",)])
                self.assertEqual(target.execute("SELECT input_prompt FROM turns").fetchone(), ("selected prompt",))
            self.assertEqual(before, hashlib.sha256(source_path.read_bytes()).hexdigest())


if __name__ == "__main__":
    unittest.main()
