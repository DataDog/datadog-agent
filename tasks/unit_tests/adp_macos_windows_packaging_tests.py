import plistlib
import unittest
from pathlib import Path

from tasks.libs.common.omnibus import ENV_PASSHTROUGH

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestADPMacOSWindowsPackaging(unittest.TestCase):
    def test_omnibus_recipe_selects_darwin_artifacts_and_supports_url_base_override(self):
        recipe = (REPO_ROOT / "omnibus/config/software/datadog-agent-data-plane.rb").read_text()

        self.assertIn('adp_hashes', recipe)
        self.assertIn('"darwin-amd64"', recipe)
        self.assertIn('"darwin-arm64"', recipe)
        self.assertIn("AGENT_DATA_PLANE_SOURCE_URL_BASE", recipe)
        self.assertIn('package_target = "darwin-#{target_arch}"', recipe)
        self.assertIn("Agent Data Plane FIPS artifacts are not available for macOS", recipe)
        self.assertIn('package_target = "fips-#{package_target}" if fips_mode?', recipe)
        self.assertIn('"windows-amd64"', recipe)
        self.assertIn('"fips-windows-amd64"', recipe)
        self.assertIn('adp_hash_key = "fips-#{package_target}"', recipe)
        self.assertIn('package_target = "#{package_target}-fips"', recipe)
        self.assertIn('package_extension = "zip"', recipe)
        self.assertNotIn("aws_lc_fips", recipe)

    def test_adp_dependency_is_included_on_linux_macos_and_windows(self):
        dependencies = (REPO_ROOT / "omnibus/config/software/datadog-agent-dependencies.rb").read_text()

        self.assertIn("(linux_target? || osx_target? || windows_target?) && !heroku_target?", dependencies)

    def test_adp_url_base_override_is_forwarded_to_omnibus(self):
        self.assertIn("AGENT_DATA_PLANE_SOURCE_URL_BASE", ENV_PASSHTROUGH)

    def test_macos_app_installs_adp_launchdaemon_template(self):
        build_file = (REPO_ROOT / "packages/macos/app/BUILD.bazel").read_text()

        self.assertIn('name = "launchd_data_plane_plist_example"', build_file)
        self.assertIn('out = "etc/com.datadoghq.data-plane.plist.example"', build_file)
        self.assertIn('":launchd_data_plane_plist_example"', build_file)

    def test_adp_launchdaemon_invokes_adp_with_macos_config(self):
        plist_path = REPO_ROOT / "packages/macos/app/launchd.data-plane.plist.example.in"
        plist = plistlib.loads(plist_path.read_bytes())

        self.assertEqual(plist["Label"], "com.datadoghq.data-plane")
        self.assertEqual(
            plist["ProgramArguments"],
            [
                "/opt/datadog-agent/embedded/bin/agent-data-plane",
                "--config",
                "/opt/datadog-agent/etc/datadog.yaml",
                "run",
                "--pidfile",
                "/opt/datadog-agent/run/agent-data-plane.pid",
            ],
        )
        self.assertEqual(plist["UserName"], "_dd-agent")
        self.assertEqual(plist["GroupName"], "daemon")

    def test_system_launchdaemons_are_managed_consistently(self):
        build_file = (REPO_ROOT / "packages/macos/app/BUILD.bazel").read_text()
        preinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/preinst").read_text()
        postinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst").read_text()
        uninstall = (REPO_ROOT / "cmd/agent/macos/uninstall_mac_os.sh").read_text()

        services = [
            ("agent", "launchd_plist_example", "com.datadoghq.agent"),
            ("sysprobe", "launchd_sysprobe_plist_example", "com.datadoghq.sysprobe"),
            ("data-plane", "launchd_data_plane_plist_example", "com.datadoghq.data-plane"),
        ]
        for name, build_target, label in services:
            with self.subTest(service=name):
                plist = f"/Library/LaunchDaemons/{label}.plist"
                example_plist = f"{label}.plist.example"

                self.assertIn(f'":{build_target}"', build_file)
                self.assertIn(f"launchctl bootout system/{label}", preinst)
                self.assertIn(example_plist, postinst)
                self.assertIn(plist, postinst)
                self.assertIn(f"chown root:wheel {plist}", postinst)
                self.assertIn(f"chmod 644 {plist}", postinst)
                self.assertIn(f"launchctl enable system/{label}", postinst)
                self.assertIn(f"launchctl bootstrap system {plist}", postinst)
                self.assertIn(f"launchctl bootout system/{label}", uninstall)
                self.assertIn(plist, uninstall)

    def test_windows_fips_packaging_does_not_expect_adp_sidecar_dlls(self):
        agent_installer = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/WixSetup/Datadog Agent/AgentInstaller.cs"
        ).read_text()
        omnibus_project = (REPO_ROOT / "omnibus/config/projects/agent.rb").read_text()

        self.assertNotIn("aws_lc_fips", agent_installer)
        self.assertNotIn("aws_lc_fips", omnibus_project)

    def test_windows_adp_procmgr_config_is_embedded_for_fleet_installer(self):
        generated_path = (
            REPO_ROOT / "pkg/fleet/installer/packages/embedded/tmpl/gen/windows/datadog-agent-data-plane.yaml"
        )
        generated = generated_path.read_text()
        embed_go = (REPO_ROOT / "pkg/fleet/installer/packages/embedded/embed.go").read_text()
        adp_procmgr = (
            REPO_ROOT / "pkg/fleet/installer/packages/processmanager/adp_procmgr_config_windows.go"
        ).read_text()

        self.assertIn("__ADP_INSTALL_ROOT__", generated)
        self.assertIn("__ADP_ETC_ROOT__", generated)
        self.assertIn("agent-data-plane.exe", generated)
        self.assertIn("tmpl/gen/windows/datadog-agent-data-plane.yaml", embed_go)
        self.assertIn("ADPWindowsProcmgrConfig", embed_go)
        self.assertIn("embedded.ADPWindowsProcmgrConfig", adp_procmgr)
