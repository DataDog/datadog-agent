from pathlib import Path

from coordinator.db import state_dir
from coordinator.inbox import (
    ack_and_archive,
    claim_inbox,
    _ack_log,
    _archive_dir,
    _inbox_path,
    _reading_path,
)


def test_claim_empty_returns_none(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    assert claim_inbox(tmp_path) is None


def test_claim_whitespace_returns_none_and_removes(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    _inbox_path(tmp_path).write_text("   \n\n")
    assert claim_inbox(tmp_path) is None
    assert not _inbox_path(tmp_path).exists()
    assert not _reading_path(tmp_path).exists()


def test_full_roundtrip(tmp_path: Path):
    state_dir(tmp_path).mkdir(parents=True)
    _inbox_path(tmp_path).write_text("please prioritise scan FP reduction\n")

    msg = claim_inbox(tmp_path)
    assert msg is not None
    assert "scan FP reduction" in msg.content
    assert not _inbox_path(tmp_path).exists()
    assert _reading_path(tmp_path).exists()

    ack_id = ack_and_archive(
        msg,
        interpretation="user wants scan FPs to be the next bet",
        planned_change="no change — current phase already targets scans",
        root=tmp_path,
    )

    assert ack_id == msg.id
    assert not _reading_path(tmp_path).exists()
    archived = list(_archive_dir(tmp_path).iterdir())
    assert len(archived) == 1

    log = _ack_log(tmp_path).read_text()
    assert msg.id in log
    assert "scan FP reduction" in log
    assert "user wants scan FPs" in log


def test_rename_beats_truncation_race(tmp_path: Path):
    """Simulate user appending to inbox.md after coordinator has claimed it.

    The coordinator's rename takes a snapshot; the user's later write
    starts a fresh inbox.md. Both messages end up in the archive over
    two iterations.
    """
    state_dir(tmp_path).mkdir(parents=True)
    _inbox_path(tmp_path).write_text("first message\n")

    msg1 = claim_inbox(tmp_path)
    assert msg1 is not None

    # User writes again while coordinator still holds .reading
    _inbox_path(tmp_path).write_text("second message\n")

    ack_and_archive(msg1, "i1", "p1", root=tmp_path)

    msg2 = claim_inbox(tmp_path)
    assert msg2 is not None
    assert "second message" in msg2.content
    ack_and_archive(msg2, "i2", "p2", root=tmp_path)

    archived = sorted(_archive_dir(tmp_path).iterdir())
    assert len(archived) == 2
