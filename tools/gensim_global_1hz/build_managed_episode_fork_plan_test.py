#!/usr/bin/env python3
"""Tests for the no-write managed generated-episode fork plan."""

from __future__ import annotations

import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

MODULE = Path(__file__).with_name("build_managed_episode_fork_plan.py")
SPEC = importlib.util.spec_from_file_location("build_managed_episode_fork_plan", MODULE)
assert SPEC and SPEC.loader
fork_plan = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fork_plan)


class BuildManagedEpisodeForkPlanTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.source = self.root / "episode-2483-v3"
        (self.source / "episodes").mkdir(parents=True)
        episode = self.source / "episodes" / "serialization-regression.yaml"
        episode.write_text("name: serialization-regression\n", encoding="utf-8")
        self.episode_hash = hashlib.sha256(episode.read_bytes()).hexdigest()
        self.source_tar_hash = "a" * 64
        self.pilot_manifest = self.root / "pilot.json"
        self.source_manifest = self.root / "source.json"
        self.blueprint = self._artifact("blueprint.tar", b"blueprint")
        self.control = self._artifact("control.tar", b"control")
        self.treatment = self._artifact("treatment.tar", b"treatment")
        self._write_manifests()

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def _artifact(self, name: str, content: bytes) -> Path:
        path = self.root / name
        path.write_bytes(content)
        return path

    def _write_manifests(self) -> None:
        self.pilot_manifest.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "source_manifest": str(self.source_manifest),
                    "scenarios": [
                        {
                            "episode_id": 2483,
                            "app_bundle_version": 3,
                            "role": "exposed_count",
                            "source_directory": str(self.source),
                            "source_tar_sha256": self.source_tar_hash,
                            "canonical_episode_file": "episodes/serialization-regression.yaml",
                            "canonical_episode_sha256": self.episode_hash,
                        }
                    ],
                }
            ),
            encoding="utf-8",
        )
        self.source_manifest.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "selection": [
                        {
                            "episode_id": 2483,
                            "episode_group": "gensim",
                            "episode_name": "events-pipeline-serialization-fault",
                            "app_bundle_version": 3,
                            "tar_sha256": self.source_tar_hash,
                            "canonical_episode_file": {
                                "path": "episodes/serialization-regression.yaml",
                                "sha256": self.episode_hash,
                            },
                        }
                    ],
                }
            ),
            encoding="utf-8",
        )

    def test_builds_append_only_plan_for_two_distinct_arm_identities(self) -> None:
        plan = fork_plan.build_plan(
            pilot_manifest_path=self.pilot_manifest,
            source_manifest_path=self.source_manifest,
            episode_id=2483,
            output_group="global-1hz-eval",
            control_name="global-1hz-2483-control",
            treatment_name="global-1hz-2483-treatment",
            blueprint_path=self.blueprint,
            control_bundle_path=self.control,
            treatment_bundle_path=self.treatment,
        )

        self.assertEqual(2483, plan["source"]["episode_id"])
        self.assertEqual(self.source_tar_hash, plan["source"]["source_tar_sha256"])
        self.assertEqual("sha256:" + hashlib.sha256(b"blueprint").hexdigest(), plan["source"]["blueprint"]["sha256"])
        self.assertEqual(["control", "treatment"], [arm["arm"] for arm in plan["arms"]])
        self.assertEqual(
            ["global-1hz-2483-control", "global-1hz-2483-treatment"],
            [arm["episode_name"] for arm in plan["arms"]],
        )
        self.assertEqual(6, len(plan["planned_operations"]))
        self.assertTrue(all(operation["external_write"] for operation in plan["planned_operations"]))
        self.assertTrue(all(operation["requires_owner_approval"] for operation in plan["planned_operations"]))
        self.assertEqual("no_write_plan", plan["mode"])

    def test_rejects_source_manifest_mismatch(self) -> None:
        data = json.loads(self.source_manifest.read_text(encoding="utf-8"))
        data["selection"][0]["tar_sha256"] = "b" * 64
        self.source_manifest.write_text(json.dumps(data), encoding="utf-8")

        with self.assertRaisesRegex(ValueError, "source tar hash"):
            fork_plan.build_plan(
                self.pilot_manifest,
                self.source_manifest,
                2483,
                "global-1hz-eval",
                "global-1hz-2483-control",
                "global-1hz-2483-treatment",
                self.blueprint,
                self.control,
                self.treatment,
            )

    def test_rejects_duplicate_arm_identity(self) -> None:
        with self.assertRaisesRegex(ValueError, "distinct"):
            fork_plan.build_plan(
                self.pilot_manifest,
                self.source_manifest,
                2483,
                "global-1hz-eval",
                "global-1hz-2483-control",
                "global-1hz-2483-control",
                self.blueprint,
                self.control,
                self.treatment,
            )


if __name__ == "__main__":
    unittest.main()
