#!/usr/bin/env python3
"""Tests for managed generated-episode fork publication readback validation."""

from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

MODULE = Path(__file__).with_name("verify_managed_episode_fork_readback.py")
SPEC = importlib.util.spec_from_file_location("verify_managed_episode_fork_readback", MODULE)
assert SPEC and SPEC.loader
verifier = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(verifier)


class VerifyManagedEpisodeForkReadbackTest(unittest.TestCase):
    def setUp(self) -> None:
        self.plan = {
            "schema_version": 1,
            "output_group": "global-1hz-eval",
            "source": {"blueprint": {"sha256": "sha256:blueprint"}},
            "arms": [
                {"arm": "control", "episode_name": "global-1hz-2483-control", "app_bundle": {"sha256": "sha256:control"}},
                {"arm": "treatment", "episode_name": "global-1hz-2483-treatment", "app_bundle": {"sha256": "sha256:treatment"}},
            ],
        }
        self.readback = {
            "schema_version": 1,
            "arms": [
                {"arm": "control", "episode_id": 5001, "group": "global-1hz-eval", "episode_name": "global-1hz-2483-control", "blueprint_sha256": "sha256:blueprint", "blueprint_version": 1, "app_bundle_sha256": "sha256:control", "app_bundle_version": 1},
                {"arm": "treatment", "episode_id": 5002, "group": "global-1hz-eval", "episode_name": "global-1hz-2483-treatment", "blueprint_sha256": "sha256:blueprint", "blueprint_version": 1, "app_bundle_sha256": "sha256:treatment", "app_bundle_version": 1},
            ],
        }

    def test_accepts_exact_distinct_arm_readback(self) -> None:
        result = verifier.verify_readback(self.plan, self.readback)
        self.assertEqual([5001, 5002], result["episode_ids"])

    def test_rejects_shared_episode_identity(self) -> None:
        self.readback["arms"][1]["episode_id"] = 5001
        with self.assertRaisesRegex(ValueError, "distinct"):
            verifier.verify_readback(self.plan, self.readback)

    def test_rejects_bundle_hash_drift(self) -> None:
        self.readback["arms"][1]["app_bundle_sha256"] = "sha256:wrong"
        with self.assertRaisesRegex(ValueError, "app_bundle_sha256"):
            verifier.verify_readback(self.plan, self.readback)


if __name__ == "__main__":
    unittest.main()
