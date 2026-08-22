import hashlib
import tempfile
import unittest
from io import StringIO
from unittest.mock import patch

from invoke.exceptions import Exit

from tasks.anomalydetection import AWS_VAULT_PROFILE, _local_aws_command, _local_ddeval_command, eval_ddeval
from tasks.libs.anomalydetection.ddeval import (
    ARTIFACT_PREFIX,
    RedactingWriter,
    artifact_metadata_matches,
    build_experiment_config,
    local_testbench_key,
    redacted_presigned_url,
    sha256_file,
    validate_linux_amd64_executable,
    validate_presigned_artifact_url,
    workflow_run_args,
)

TESTBENCH_SHA256 = hashlib.sha256(b"testbench").hexdigest()
PRESIGNED_URL = (
    "https://qbranch-gensim-recordings.s3.amazonaws.com/"
    f"{ARTIFACT_PREFIX}/build/anomalydetection-testbench"
    "?X-Amz-Credential=temporary&X-Amz-Signature=secret"
)


class TestLocalDDEvalArtifacts(unittest.TestCase):
    @patch.dict("os.environ", {"AWS_PROFILE": "legacy-profile"})
    def test_default_aws_command_ignores_ambient_profile(self):
        self.assertEqual(_local_aws_command(""), ["aws-vault", "exec", AWS_VAULT_PROFILE, "--", "aws"])

    def test_custom_aws_profile_bypasses_aws_vault(self):
        self.assertEqual(_local_aws_command("custom-profile"), ["aws", "--profile", "custom-profile"])

    def test_custom_testbench_binary_requires_no_build(self):
        with self.assertRaisesRegex(Exit, "custom --testbench-binary requires --no-build"):
            eval_ddeval.body(None, testbench_binary="/tmp/custom-testbench")

    def test_custom_ddeval_executable_is_not_shell_parsed(self):
        executable = r"C:\Program Files\DDEval\ddeval.exe"

        command, command_dir = _local_ddeval_command(executable, "")

        self.assertEqual(command, [executable])
        self.assertIsNone(command_dir)

    def test_sha256_file_streams_file(self):
        with tempfile.NamedTemporaryFile() as testbench:
            testbench.write(b"testbench")
            testbench.flush()
            self.assertEqual(sha256_file(testbench.name), TESTBENCH_SHA256)

    def test_binary_must_target_linux_amd64(self):
        elf_header = bytearray(20)
        elf_header[:6] = b"\x7fELF\x02\x01"
        elf_header[18:20] = (62).to_bytes(2, "little")
        with tempfile.NamedTemporaryFile() as testbench:
            testbench.write(elf_header)
            testbench.flush()
            validate_linux_amd64_executable(testbench.name)

        with tempfile.NamedTemporaryFile() as testbench:
            testbench.write(b"native macOS binary")
            testbench.flush()
            with self.assertRaisesRegex(ValueError, "Linux/amd64"):
                validate_linux_amd64_executable(testbench.name)

    def test_local_testbench_key_is_content_addressed(self):
        key = local_testbench_key(TESTBENCH_SHA256)

        self.assertEqual(
            key,
            f"{ARTIFACT_PREFIX}/sha256/{TESTBENCH_SHA256}/anomalydetection-testbench",
        )

    def test_artifact_metadata_must_match_digest_and_size(self):
        head_object = {"ContentLength": 9, "Metadata": {"sha256": TESTBENCH_SHA256}}

        self.assertTrue(artifact_metadata_matches(head_object, TESTBENCH_SHA256, 9))
        self.assertFalse(artifact_metadata_matches(head_object, "0" * 64, 9))
        self.assertFalse(artifact_metadata_matches(head_object, TESTBENCH_SHA256, 10))

    def test_experiment_config_preserves_settings_and_replaces_artifacts(self):
        base_config = {
            "input_parameters": {"existing": True},
            "executor_config": {
                "sigma": 30,
                "binary_artifacts": {"scorer": {"uri": "s3://legacy/scorer"}},
            },
            "evaluator_config": {"threshold": 0.5},
        }
        component_config = {"components": {"bocpd": {"enabled": True}}}

        config = build_experiment_config(
            base_config,
            testbench_url=PRESIGNED_URL,
            testbench_sha256=TESTBENCH_SHA256,
            testbench_config=component_config,
        )

        self.assertEqual(config["executor_config"]["sigma"], 30)
        self.assertEqual(
            config["executor_config"]["binary_artifacts"],
            {
                "testbench": {
                    "uri": PRESIGNED_URL,
                    "sha256": TESTBENCH_SHA256,
                    "filename": "anomalydetection-testbench",
                }
            },
        )
        self.assertEqual(config["input_parameters"]["testbench_config"], component_config)
        self.assertEqual(config["evaluator_config"], {"threshold": 0.5})
        self.assertNotIn("testbench_config", base_config["input_parameters"])

    def test_presigned_url_validation_is_restricted_to_local_artifacts(self):
        validate_presigned_artifact_url(PRESIGNED_URL)

        invalid_urls = [
            PRESIGNED_URL.replace("https://", "http://"),
            PRESIGNED_URL.replace("qbranch-gensim-recordings", "another-bucket"),
            PRESIGNED_URL.replace(f"{ARTIFACT_PREFIX}/", "another-prefix/"),
            PRESIGNED_URL.split("?")[0],
        ]
        for url in invalid_urls:
            with self.subTest(url=url), self.assertRaises(ValueError):
                validate_presigned_artifact_url(url)

    def test_redacting_writer_handles_a_secret_split_across_writes(self):
        output = StringIO()
        replacement = redacted_presigned_url(PRESIGNED_URL)
        writer = RedactingWriter(output, PRESIGNED_URL, replacement)

        split = len(PRESIGNED_URL) // 2
        writer.write(f"artifact={PRESIGNED_URL[:split]}")
        writer.write(f"{PRESIGNED_URL[split:]} done\n")
        writer.finish()

        rendered = output.getvalue()
        self.assertEqual(rendered, f"artifact={replacement} done\n")
        self.assertNotIn("X-Amz-Signature", rendered)
        self.assertNotIn("secret", rendered)

    def test_redacting_writer_does_not_flush_a_partial_secret(self):
        output = StringIO()
        replacement = redacted_presigned_url(PRESIGNED_URL)
        writer = RedactingWriter(output, PRESIGNED_URL, replacement)

        writer.write(PRESIGNED_URL[:-3])
        writer.finish()

        self.assertNotIn("X-Amz-Signature", output.getvalue())

    def test_workflow_args_target_ddbuild_without_a_test_drive(self):
        args = workflow_run_args(
            ["ddeval"],
            service="eval_worker_agent_aad",
            project="observer-log-ad",
            dataset="dataset",
            dataset_version=6,
            config_path="/tmp/config.json",
            jobs=20,
            max_attempts=1,
            data_env="staging",
            limit=2,
            where_in="metadata.record_id=a,b",
        )

        self.assertIn("ddbuild", args)
        self.assertIn("eval_worker_agent_aad", args)
        self.assertEqual(args[args.index("--dataset-version") + 1], "6")
        self.assertIn("metadata.record_id=a,b", args)
        self.assertNotIn("--workflow-test-drive", args)


if __name__ == "__main__":
    unittest.main()
