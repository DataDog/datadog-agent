import json
from pathlib import Path

from coordinator.db import state_dir
from coordinator.journal import append


def test_append_creates_file_and_appends(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    append("iter_started", {"iter": 0}, tmp_path)
    append("iter_started", {"iter": 1}, tmp_path)
    lines = (state_dir(tmp_path) / "journal.jsonl").read_text().splitlines()
    assert len(lines) == 2
    r0 = json.loads(lines[0])
    r1 = json.loads(lines[1])
    assert r0["type"] == "iter_started"
    assert r0["iter"] == 0
    assert r1["iter"] == 1
    assert "ts" in r0 and "ts" in r1
