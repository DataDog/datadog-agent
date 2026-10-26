#!/usr/bin/env python3
"""Tests for deterministic local-GKE / gs-flow portable pilot bundle derivation."""

from __future__ import annotations

import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

MODULE = Path(__file__).with_name("build_pilot_bundles.py")
SPEC = importlib.util.spec_from_file_location("build_pilot_bundles", MODULE)
assert SPEC and SPEC.loader
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)

AGENT_TEMPLATE = """apiVersion: apps/v1
kind: {kind}
spec:
  template:
    spec:
      containers:
        - name: datadog-agent
          image: gcr.io/datadoghq/agent:7
          env:
            - name: DD_ENV
              value: {{{{ .Values.datadog.env | quote }}}}
            - name: KUBERNETES
              value: \"yes\"
"""


class BuildPilotBundlesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.source_root = self.root / "sources"
        self.output_root = self.root / "output"
        self.source_root.mkdir()
        self._write_source(2483, 3, "DaemonSet")
        self._write_source(2481, 3, "Deployment")

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def _write_source(self, episode_id: int, version: int, kind: str) -> None:
        source = self.source_root / f"episode-{episode_id}-v{version}"
        (source / "chart" / "templates").mkdir(parents=True)
        (source / "episodes").mkdir()
        (source / "chart" / "templates" / "datadog-agent.yaml").write_text(
            AGENT_TEMPLATE.format(kind=kind), encoding="utf-8"
        )
        (source / "episodes" / "scenario.yaml").write_text(
            f"name: scenario-{episode_id}\n", encoding="utf-8"
        )

    def _manifest(self) -> Path:
        entries = []
        for episode_id, version, role in [
            (2483, 3, "exposed_count"),
            (2481, 3, "exposed_recurring_check"),
        ]:
            source = self.source_root / f"episode-{episode_id}-v{version}"
            episode_path = source / "episodes" / "scenario.yaml"
            entries.append(
                {
                    "episode_id": episode_id,
                    "app_bundle_version": version,
                    "role": role,
                    "source_directory": str(source),
                    "canonical_episode_file": "episodes/scenario.yaml",
                    "canonical_episode_sha256": hashlib.sha256(episode_path.read_bytes()).hexdigest(),
                }
            )
        manifest = self.root / "pilot.json"
        manifest.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "agent_image": "datadog/agent-dev@sha256:custom",
                    "scenarios": entries,
                }
            ),
            encoding="utf-8",
        )
        return manifest

    def test_builds_matched_arm_bundles_and_records_only_toggle_difference(self) -> None:
        result = builder.build_bundles(self._manifest(), self.output_root)

        self.assertEqual([2481, 2483], [entry["episode_id"] for entry in result["scenarios"]])
        for entry in result["scenarios"]:
            control = self.output_root / entry["arms"]["A"]["directory"]
            treatment = self.output_root / entry["arms"]["B"]["directory"]
            control_agent = (control / "chart/templates/datadog-agent.yaml").read_text(encoding="utf-8")
            treatment_agent = (treatment / "chart/templates/datadog-agent.yaml").read_text(encoding="utf-8")
            self.assertIn("image: datadog/agent-dev@sha256:custom", control_agent)
            self.assertIn('DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED\n              value: "false"', control_agent)
            self.assertIn('DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED\n              value: "true"', treatment_agent)
            self.assertEqual(
                control_agent.replace('value: "false"', "<toggle>"),
                treatment_agent.replace('value: "true"', "<toggle>"),
            )
            self.assertEqual(["chart/templates/datadog-agent.yaml"], entry["allowed_arm_diff_paths"])
            self.assertTrue(entry["arms"]["A"]["tree_sha256"])
            self.assertTrue(entry["arms"]["B"]["tree_sha256"])

        self.assertTrue((self.output_root / "pilot-bundle-manifest.json").exists())

    def test_rejects_tampered_source_episode_file(self) -> None:
        manifest = self._manifest()
        (self.source_root / "episode-2483-v3/episodes/scenario.yaml").write_text("tampered\n", encoding="utf-8")

        with self.assertRaisesRegex(ValueError, "canonical episode hash"):
            builder.build_bundles(manifest, self.output_root)

    def test_rejects_agent_template_without_single_stock_image(self) -> None:
        manifest = self._manifest()
        agent_template = self.source_root / "episode-2483-v3/chart/templates/datadog-agent.yaml"
        agent_template.write_text(agent_template.read_text(encoding="utf-8") + "\n        - name: datadog-agent\n          image: another-agent:7\n", encoding="utf-8")

        with self.assertRaisesRegex(ValueError, "exactly one agent image"):
            builder.build_bundles(manifest, self.output_root)


if __name__ == "__main__":
    unittest.main()
