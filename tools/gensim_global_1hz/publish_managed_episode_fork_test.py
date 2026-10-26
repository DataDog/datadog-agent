#!/usr/bin/env python3
"""Tests for the owner-run managed GS Admin fork publisher."""

from __future__ import annotations

import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

MODULE = Path(__file__).with_name("publish_managed_episode_fork.py")
SPEC = importlib.util.spec_from_file_location("publish_managed_episode_fork", MODULE)
assert SPEC and SPEC.loader
publisher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publisher)


class FakeClient:
    def __init__(self, *, existing: bool = False, corrupt_archive: bool = False) -> None:
        self.existing = existing
        self.corrupt_archive = corrupt_archive
        self.calls: list[tuple[str, object]] = []
        self.next_id = 900
        self.archives: dict[tuple[int, int, str], bytes] = {}

    def find_episode_by_exact_name(self, group: str, name: str):
        self.calls.append(("find", (group, name)))
        if self.existing:
            return SimpleNamespace(id=123, group=group, name=name)
        return None

    def put_episode(self, group: str, name: str, tags: list[str]):
        self.calls.append(("put_episode", (group, name, tags)))
        self.next_id += 1
        return SimpleNamespace(id=self.next_id, group=group, name=name)

    def latest_blueprint_version(self, episode_id: int):
        self.calls.append(("latest_blueprint", episode_id))
        return None

    def latest_app_bundle_version(self, episode_id: int):
        self.calls.append(("latest_app_bundle", episode_id))
        return None

    def put_blueprint(self, episode_id: int, tar_bytes: bytes, tags: list[str]):
        self.calls.append(("put_blueprint", (episode_id, tags)))
        self.archives[(episode_id, 1, "blueprint")] = tar_bytes
        return SimpleNamespace(version=1)

    def put_app_bundle(self, episode_id: int, blueprint_version: int, tar_bytes: bytes, tags: list[str]):
        self.calls.append(("put_app_bundle", (episode_id, blueprint_version, tags)))
        self.archives[(episode_id, 1, "app_bundle")] = tar_bytes
        return SimpleNamespace(version=1)

    def download_blueprint_archive(self, episode_id: int, version: int) -> bytes:
        data = self.archives[(episode_id, version, "blueprint")]
        return b"corrupt" if self.corrupt_archive else data

    def app_bundle_presigned_url(self, episode_id: int, version: int) -> str:
        return f"fake://{episode_id}/{version}"

    def fetch_app_bundle(self, url: str) -> bytes:
        _, _, episode_id, version = url.rsplit("/", 3)
        return self.archives[(int(episode_id), int(version), "app_bundle")]


class PublishManagedEpisodeForkTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.blueprint = self._artifact("blueprint.tar", b"blueprint")
        self.control = self._artifact("control.tar", b"control")
        self.treatment = self._artifact("treatment.tar", b"treatment")
        self.plan = publisher.load_plan(self._write_plan())

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def _artifact(self, name: str, data: bytes) -> Path:
        path = self.root / name
        path.write_bytes(data)
        return path

    def _artifact_ref(self, path: Path) -> dict[str, str]:
        return {"path": str(path), "sha256": "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest(), "bytes": str(path.stat().st_size)}

    def _write_plan(self) -> Path:
        path = self.root / "plan.json"
        path.write_text(json.dumps({
            "schema_version": 1,
            "mode": "no_write_plan",
            "output_group": "global-1hz-eval",
            "source": {
                "episode_id": 2483,
                "blueprint": self._artifact_ref(self.blueprint),
            },
            "arms": [
                {"arm": "control", "episode_name": "global-1hz-2483-control", "origin_tags": {"global-1hz.arm": "control"}, "app_bundle": self._artifact_ref(self.control)},
                {"arm": "treatment", "episode_name": "global-1hz-2483-treatment", "origin_tags": {"global-1hz.arm": "treatment"}, "app_bundle": self._artifact_ref(self.treatment)},
            ],
        }), encoding="utf-8")
        return path

    def test_dry_run_validates_local_plan_without_a_client(self) -> None:
        summary = publisher.dry_run_summary(self.plan)
        self.assertEqual("dry_run", summary["mode"])
        self.assertEqual(["control", "treatment"], summary["arms"])

    def test_publishes_distinct_arms_and_emits_verifier_readback(self) -> None:
        client = FakeClient()
        readback = publisher.publish(self.plan, client, client.fetch_app_bundle)
        self.assertEqual([901, 902], [arm["episode_id"] for arm in readback["arms"]])
        self.assertEqual([1, 1], [arm["blueprint_version"] for arm in readback["arms"]])
        self.assertEqual([1, 1], [arm["app_bundle_version"] for arm in readback["arms"]])
        self.assertEqual(2, len([call for call in client.calls if call[0] == "put_episode"]))
        self.assertEqual(2, len([call for call in client.calls if call[0] == "put_blueprint"]))
        self.assertEqual(2, len([call for call in client.calls if call[0] == "put_app_bundle"]))

    def test_refuses_existing_destination_before_any_write(self) -> None:
        client = FakeClient(existing=True)
        with self.assertRaisesRegex(ValueError, "already exists"):
            publisher.publish(self.plan, client, client.fetch_app_bundle)
        self.assertFalse(any(call[0].startswith("put_") for call in client.calls))

    def test_stops_when_readback_archive_hash_drifts(self) -> None:
        client = FakeClient(corrupt_archive=True)
        with self.assertRaisesRegex(ValueError, "blueprint archive hash"):
            publisher.publish(self.plan, client, client.fetch_app_bundle)


if __name__ == "__main__":
    unittest.main()
