import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from tasks.libs.agent_sandbox.manager import AgentSandboxError, AgentSandboxManager, SandboxMetadata


class TestAgentSandboxManager(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / "sandbox"
        self.manager = AgentSandboxManager(state_root=self.root)

    def _write_fake_ssh_key(self, paths):
        paths.ssh_dir.mkdir(parents=True, exist_ok=True)
        paths.private_key.write_text("private")
        paths.public_key.write_text("ssh-ed25519 public agent-sandbox")

    def test_prepare_host_sandbox_writes_state_and_provisioning(self):
        config = Path(self.tmp.name) / "datadog.yaml"
        config.write_text("api_key: 00000000000000000000000000000000\n")

        with (
            mock.patch.object(self.manager, "assert_supported_host"),
            mock.patch.object(self.manager, "ensure_ssh_key", side_effect=self._write_fake_ssh_key),
            mock.patch.object(self.manager, "create_seed_iso"),
            mock.patch.object(
                self.manager, "ensure_cached_ubuntu_image", return_value=Path(self.tmp.name) / "ubuntu.img"
            ),
            mock.patch.object(self.manager, "prepare_disk_image"),
        ):
            metadata = self.manager.prepare_host_sandbox(name="review", agent_version="7.66.0", config=config)

        paths = self.manager.paths("review")
        self.assertEqual(metadata.name, "review")
        self.assertEqual(metadata.mode, "host-agent")
        self.assertEqual(metadata.state, "created")
        self.assertTrue(paths.metadata_file.exists())
        self.assertTrue(paths.host_install_script.exists())
        self.assertTrue(paths.cloud_init_user_data.exists())
        self.assertEqual((paths.provisioning_dir / "datadog.yaml").read_text(), config.read_text())

        data = json.loads(paths.metadata_file.read_text())
        self.assertEqual(data["agent_version"], "7.66.0")
        script = paths.host_install_script.read_text()
        self.assertIn("DD_AGENT_MINOR_VERSION=66.0", script)
        self.assertIn("install_script_agent7.sh", script)
        self.assertIn("/opt/datadog-agent/bin/agent/agent version", script)
        user_data = paths.cloud_init_user_data.read_text()
        self.assertIn("ssh-ed25519 public agent-sandbox", user_data)
        self.assertIn("/var/lib/agent-sandbox/datadog.yaml", user_data)
        self.assertIn("/var/lib/agent-sandbox/install-host-agent.sh", user_data)

    def test_prepare_host_sandbox_rejects_duplicate(self):
        with (
            mock.patch.object(self.manager, "assert_supported_host"),
            mock.patch.object(self.manager, "ensure_ssh_key"),
            mock.patch.object(self.manager, "write_cloud_init_seed"),
            mock.patch.object(
                self.manager, "ensure_cached_ubuntu_image", return_value=Path(self.tmp.name) / "ubuntu.img"
            ),
            mock.patch.object(self.manager, "prepare_disk_image"),
        ):
            self.manager.prepare_host_sandbox(name="dup")
            with self.assertRaisesRegex(AgentSandboxError, "already exists"):
                self.manager.prepare_host_sandbox(name="dup")

    def test_ssh_and_agent_commands_use_managed_metadata(self):
        paths = self.manager.ensure_layout("cmd")
        paths.private_key.write_text("private")
        paths.public_key.write_text("public")
        self.manager.write_metadata(
            paths.metadata_file,
            SandboxMetadata(
                name="cmd",
                mode="host-agent",
                state="created",
                guest_user="ubuntu",
                mac_address="02:dd:00:00:00:01",
            ),
        )
        self.manager.update_connection("cmd", "127.0.0.1", 2222)

        ssh = self.manager.ssh_command("cmd")
        self.assertEqual(ssh[:2], ["ssh", "-i"])
        self.assertIn(str(paths.private_key), ssh)
        self.assertIn("2222", ssh)
        self.assertIn("ubuntu@127.0.0.1", ssh)

        agent = self.manager.agent_command("cmd", "status --json")
        self.assertEqual(agent[-4:], ["sudo", "/opt/datadog-agent/bin/agent/agent", "status", "--json"])

    def test_ssh_command_requires_endpoint(self):
        paths = self.manager.ensure_layout("no-endpoint")
        self.manager.write_metadata(
            paths.metadata_file,
            SandboxMetadata(
                name="no-endpoint",
                mode="host-agent",
                state="created",
                guest_user="ubuntu",
                mac_address="02:dd:00:00:00:02",
            ),
        )
        with self.assertRaisesRegex(AgentSandboxError, "no SSH endpoint"):
            self.manager.ssh_command("no-endpoint")


if __name__ == "__main__":
    unittest.main()
