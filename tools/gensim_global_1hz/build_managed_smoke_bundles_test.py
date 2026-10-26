#!/usr/bin/env python3
"""Tests for minimal managed smoke A/B artifact derivation."""

from __future__ import annotations

import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

MODULE = Path(__file__).with_name("build_managed_smoke_bundles.py")
SPEC = importlib.util.spec_from_file_location("build_managed_smoke_bundles", MODULE)
assert SPEC and SPEC.loader
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)

AGENT_TEMPLATE = """apiVersion: apps/v1
kind: DaemonSet
spec:
  template:
    spec:
      containers:
        - name: datadog-agent
          image: gcr.io/datadoghq/agent:7
          env:
            - name: DD_CRI_SOCKET_PATH
              value: /var/run/containerd/containerd.sock
            - name: KUBERNETES
              value: \"yes\"
          volumeMounts:
            - name: logpods
              mountPath: /var/log/pods
"""


class BuildManagedSmokeBundlesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.source = self.root / "episode-2483-v3"
        (self.source / "chart" / "templates").mkdir(parents=True)
        (self.source / "episodes").mkdir()
        (self.source / "chart" / "templates" / "datadog-agent.yaml").write_text(AGENT_TEMPLATE, encoding="utf-8")
        episode = self.source / "episodes" / "serialization-regression.yaml"
        episode.write_text("name: serialization-regression\n", encoding="utf-8")
        self.manifest = self.root / "pilot.json"
        self.manifest.write_text(json.dumps({
            "schema_version": 1,
            "agent_image": "datadog/agent-dev@sha256:custom",
            "scenarios": [{
                "episode_id": 2483,
                "app_bundle_version": 3,
                "role": "exposed_count",
                "source_directory": str(self.source),
                "source_tar_sha256": "a" * 64,
                "canonical_episode_file": "episodes/serialization-regression.yaml",
                "canonical_episode_sha256": hashlib.sha256(episode.read_bytes()).hexdigest(),
            }],
        }), encoding="utf-8")
        self.output = self.root / "output"

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def test_preserves_daemonset_socket_and_mounts_with_only_arm_toggle_difference(self) -> None:
        result = builder.build_smoke_bundles(self.manifest, 2483, self.output)
        control = self.output / result["arms"]["control"]["directory"]
        treatment = self.output / result["arms"]["treatment"]["directory"]
        control_template = (control / "chart/templates/datadog-agent.yaml").read_text(encoding="utf-8")
        treatment_template = (treatment / "chart/templates/datadog-agent.yaml").read_text(encoding="utf-8")
        self.assertIn("kind: DaemonSet", control_template)
        self.assertIn("/var/run/containerd/containerd.sock", control_template)
        self.assertIn("mountPath: /var/log/pods", control_template)
        self.assertEqual(
            control_template.replace('value: "false"', "<toggle>"),
            treatment_template.replace('value: "true"', "<toggle>"),
        )
        self.assertEqual(["chart/templates/datadog-agent.yaml"], result["allowed_arm_diff_paths"])
        self.assertTrue((self.output / result["arms"]["control"]["tar_path"]).is_file())
        self.assertTrue((self.output / result["arms"]["treatment"]["tar_path"]).is_file())
        self.assertNotEqual(result["arms"]["control"]["tar_sha256"], result["arms"]["treatment"]["tar_sha256"])

    def test_rejects_non_daemonset_source_for_first_smoke(self) -> None:
        template = self.source / "chart/templates/datadog-agent.yaml"
        template.write_text(template.read_text(encoding="utf-8").replace("kind: DaemonSet", "kind: Deployment"), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "DaemonSet"):
            builder.build_smoke_bundles(self.manifest, 2483, self.output)


if __name__ == "__main__":
    unittest.main()
