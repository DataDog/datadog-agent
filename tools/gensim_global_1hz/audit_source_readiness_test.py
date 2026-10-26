import io
import json
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).parent))
import audit_source_readiness as audit


IDENTITY = {
    "app_bundle_version": 3,
    "archive_key": "schema-v1/episode/1/v3/archive.tar",
    "manifest_sha256": "a" * 64,
    "canonical_episode_sha256": "b" * 64,
}


def tar_bytes(*, env_line='  env: ""\n', rca_chain=None):
    episode = {
        "name": "example",
        "description": "Example root cause",
        "long_description": "A detailed, source-authored causal narrative.",
    }
    scenario = {}
    if rca_chain is not None:
        scenario["rca_causal_chain"] = rca_chain
    files = {
        "episodes/example.yaml": yaml.safe_dump(episode).encode(),
        "scenario.yaml": yaml.safe_dump(scenario).encode(),
        "play-episode.sh": b"#!/bin/sh\n",
        "chart/values.yaml": ("datadog:\n" + env_line).encode(),
        "chart/templates/datadog-agent.yaml": (
            "image: gcr.io/datadoghq/agent:7\n"
            "            - name: DD_ENV\n"
            "              value: {{ .Values.datadog.env | quote }}\n"
            "          volumeMounts:\n"
            "      volumes:\n"
            "---\napiVersion: v1\nkind: Service\nmetadata:\n"
            "  name: datadog-agent\n"
        ).encode(),
    }
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w") as archive:
        for name, data in files.items():
            member = tarfile.TarInfo(name)
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))
    return output.getvalue(), episode


class AuditSourceReadinessTest(unittest.TestCase):
    def test_rca_chain_uses_campaign_archive_contract(self):
        valid = {
            "immediate_cause": "bad config",
            "findings_chain": ["symptom", "cause"],
            "contributing_factors": ["missing guardrail"],
            "deeper_findings_total": 2,
        }
        self.assertEqual("valid", audit.rca_chain_status({"rca_causal_chain": valid}))

        invalid = {**valid, "deeper_findings_total": 1}
        self.assertEqual("invalid_count", audit.rca_chain_status({"rca_causal_chain": invalid}))
        self.assertEqual("missing", audit.rca_chain_status({}))

    def test_agent_adapter_distinguishes_current_and_safe_extension(self):
        cases = [
            ('  env: ""\n', "compatible"),
            ("  env: ''\n", "requires_empty_string_adapter_extension"),
        ]
        for env_line, expected in cases:
            with self.subTest(env_line=env_line):
                raw, _ = tar_bytes(env_line=env_line)
                files = audit.read_safe_tar(raw)
                self.assertEqual(expected, audit.agent_adapter_status(files))

    def test_indexes_top_level_source_identity_bindings(self):
        indexed = audit.index_judge_fixtures(
            {
                "cases": [{"episode_id": 1, "ground_truth": "cause"}],
                "source_identity_bindings": {"1": IDENTITY},
            }
        )

        self.assertEqual(IDENTITY, indexed[1]["source_identity"])

    def test_judge_fixture_must_be_bound_to_exact_source_identity(self):
        fixture = {"episode_id": 1, "source_identity": dict(IDENTITY)}
        self.assertEqual("bound_match", audit.judge_fixture_status(fixture, IDENTITY))

        fixture["source_identity"]["app_bundle_version"] = 4
        self.assertEqual("bound_mismatch", audit.judge_fixture_status(fixture, IDENTITY))
        self.assertEqual("present_unbound", audit.judge_fixture_status({"episode_id": 1}, IDENTITY))
        self.assertEqual("missing", audit.judge_fixture_status(None, IDENTITY))

    def test_audit_marks_missing_archive_unknown_without_inference(self):
        result = audit.audit_candidate(
            candidate={"episode_id": 1, "admission_class": "resolution_exposed"},
            identity=IDENTITY,
            judge_fixture=None,
            archive_path=None,
            expected_archive_sha256=None,
            canonical_episode_path="episodes/example.yaml",
        )
        self.assertEqual("missing", result["archive_status"])
        self.assertEqual("unknown", result["authoritative_ground_truth_status"])
        self.assertEqual("unknown", result["agent_adapter_status"])
        self.assertFalse(result["static_execution_ready"])

    def test_audit_verifies_hash_ground_truth_and_static_contracts(self):
        raw, episode = tar_bytes()
        canonical = yaml.safe_dump(episode).encode()
        identity = {**IDENTITY, "canonical_episode_sha256": audit.sha256_bytes(canonical)}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "episode-1-v3.tar"
            path.write_bytes(raw)
            result = audit.audit_candidate(
                candidate={"episode_id": 1, "admission_class": "resolution_exposed"},
                identity=identity,
                judge_fixture={"episode_id": 1, "source_identity": identity},
                archive_path=path,
                expected_archive_sha256=audit.sha256_bytes(raw),
                canonical_episode_path="episodes/example.yaml",
            )
        self.assertEqual("verified", result["archive_status"])
        self.assertEqual("source_detailed", result["authoritative_ground_truth_status"])
        self.assertEqual("compatible", result["agent_adapter_status"])
        self.assertEqual("missing", result["archive_rca_chain_status"])
        self.assertEqual("bound_match", result["judge_fixture_status"])
        self.assertTrue(result["static_execution_ready"])


if __name__ == "__main__":
    unittest.main()
