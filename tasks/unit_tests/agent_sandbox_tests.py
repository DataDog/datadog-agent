import json
import shlex
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
            mock.patch.object(self.manager, "clone_cached_ubuntu_base"),
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
        self.assertIn("ln -sf /opt/datadog-agent/bin/agent/agent /usr/local/bin/agent", script)
        user_data = paths.cloud_init_user_data.read_text()
        self.assertIn("ssh-ed25519 public agent-sandbox", user_data)
        self.assertIn("agent_sandbox_apt_cache", user_data)
        self.assertIn("/var/cache/apt/archives", user_data)
        self.assertIn("/var/lib/agent-sandbox/datadog.yaml", user_data)
        self.assertIn("/var/lib/agent-sandbox/install-host-agent.sh", user_data)

    def test_prepare_host_sandbox_rejects_duplicate(self):
        with (
            mock.patch.object(self.manager, "assert_supported_host"),
            mock.patch.object(self.manager, "ensure_ssh_key"),
            mock.patch.object(self.manager, "write_cloud_init_seed"),
            mock.patch.object(self.manager, "clone_cached_ubuntu_base"),
        ):
            self.manager.prepare_host_sandbox(name="dup")
            with self.assertRaisesRegex(AgentSandboxError, "already exists"):
                self.manager.prepare_host_sandbox(name="dup")

    def test_default_config_disables_cloud_and_remote_config_and_uses_fakeintake(self):
        config = Path(self.tmp.name) / "datadog.yaml"
        self.manager.write_default_datadog_config(config, "http://192.168.64.1:30080")

        content = config.read_text()
        self.assertIn("cloud_provider_metadata: []", content)
        self.assertIn("remote_configuration:\n  enabled: false", content)
        self.assertIn("dd_url: http://192.168.64.1:30080", content)
        self.assertIn("process_dd_url: http://192.168.64.1:30080", content)

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
        self.assertIn("ConnectTimeout=10", ssh)
        self.assertIn(f"UserKnownHostsFile={paths.ssh_dir / 'known_hosts'}", ssh)

        shell = self.manager.shell_command("cmd", "echo one; echo two")
        self.assertEqual(shell[-3:], ["bash", "-lc", shlex.quote("echo one; echo two")])

        agent = self.manager.agent_command("cmd", "status --json")
        self.assertEqual(agent[-4:], ["sudo", "/opt/datadog-agent/bin/agent/agent", "status", "--json"])

    def test_copy_disk_image_uses_clonefile_and_falls_back(self):
        source = Path(self.tmp.name) / "source.raw"
        destination = Path(self.tmp.name) / "nested" / "destination.raw"
        source.write_text("disk")
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text("old")

        with (
            mock.patch("tasks.libs.agent_sandbox.manager.platform.system", return_value="Darwin"),
            mock.patch("tasks.libs.agent_sandbox.manager.subprocess.run") as run,
            mock.patch("tasks.libs.agent_sandbox.manager.shutil.copyfile") as copyfile,
        ):
            run.return_value = mock.Mock(returncode=0)
            self.manager.copy_disk_image(source, destination)

        run.assert_called_once_with(["/bin/cp", "-c", str(source), str(destination)], check=False)
        copyfile.assert_not_called()
        destination.write_text("old")

        with (
            mock.patch("tasks.libs.agent_sandbox.manager.platform.system", return_value="Darwin"),
            mock.patch("tasks.libs.agent_sandbox.manager.subprocess.run") as run,
            mock.patch("tasks.libs.agent_sandbox.manager.shutil.copyfile") as copyfile,
        ):
            run.return_value = mock.Mock(returncode=1)
            self.manager.copy_disk_image(source, destination)

        copyfile.assert_called_once_with(source, destination)

    def test_kubernetes_values_render_image_fakeintake_and_operator_disabled(self):
        values = Path(self.tmp.name) / "values.yaml"
        self.manager.write_datadog_helm_values(values, "gcr.io/datadoghq/agent:7.80.1", "http://192.168.64.1:12345")

        content = values.read_text()
        self.assertIn("apiKey: a0000000000000000000000000000001", content)
        self.assertIn("dd_url: http://192.168.64.1:12345", content)
        self.assertIn("repository: gcr.io/datadoghq/agent", content)
        self.assertIn("tag: 7.80.1", content)
        self.assertIn("operator:\n    enabled: false", content)
        self.assertIn("DD_PROCESS_CONFIG_PROCESS_DD_URL", content)

    def test_ipv6_link_local_for_mac(self):
        self.assertEqual(
            self.manager.ipv6_link_local_for_mac("02:dd:27:35:c6:62"),
            "fe80::dd:27ff:fe35:c662%bridge100",
        )

    def test_split_image_defaults_latest_when_tag_is_absent(self):
        self.assertEqual(self.manager.split_image("gcr.io/datadoghq/agent"), ("gcr.io/datadoghq/agent", "latest"))
        self.assertEqual(
            self.manager.split_image("gcr.io/datadoghq/agent:7.80.1"), ("gcr.io/datadoghq/agent", "7.80.1")
        )

    def test_create_seed_iso_replaces_stale_iso_and_reports_failures(self):
        paths = self.manager.ensure_layout("iso")
        paths.cloud_init_dir.mkdir(parents=True, exist_ok=True)
        paths.seed_iso.write_text("stale")

        with mock.patch("tasks.libs.agent_sandbox.manager.shutil.which", return_value="/usr/bin/hdiutil"):
            with mock.patch("tasks.libs.agent_sandbox.manager.subprocess.run") as run:
                run.return_value = mock.Mock(returncode=0, stdout="", stderr="")
                self.manager.create_seed_iso(paths)

        self.assertFalse(paths.seed_iso.exists())
        run.assert_called_once()
        self.assertIn(str(paths.seed_iso), run.call_args.args[0])

        with mock.patch("tasks.libs.agent_sandbox.manager.shutil.which", return_value="/usr/bin/hdiutil"):
            with mock.patch("tasks.libs.agent_sandbox.manager.subprocess.run") as run:
                run.return_value = mock.Mock(returncode=1, stdout="", stderr="hdiutil failed")
                with self.assertRaisesRegex(AgentSandboxError, "hdiutil failed"):
                    self.manager.create_seed_iso(paths)

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
