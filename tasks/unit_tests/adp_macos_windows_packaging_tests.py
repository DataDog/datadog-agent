import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestADPMacOSWindowsPackaging(unittest.TestCase):
    def test_agent_data_plane_hashes_are_defined_for_all_platforms(self):
        module_bazel = (REPO_ROOT / "deps/agent_data_plane/agent_data_plane.MODULE.bazel").read_text()

        for hash_key in [
            "linux-amd64",
            "linux-arm64",
            "fips-linux-amd64",
            "fips-linux-arm64",
            "darwin-amd64",
            "darwin-arm64",
            "windows-amd64",
            "windows-amd64-fips",
        ]:
            self.assertIn(f'"{hash_key}"', module_bazel)

    def test_macos_archive_selection_has_no_fips_variant(self):
        archives = (REPO_ROOT / "deps/agent_data_plane/BUILD.bazel").read_text()
        packages_agent = (REPO_ROOT / "packages/agent/BUILD.bazel").read_text()

        # Agent Data Plane has no FIPS build for macOS: base_flavor (not fips_flavor)
        # gates the macOS archive selection, so a FIPS+macOS build resolves no
        # select() branch at all and fails loudly instead of silently falling
        # back to the non-FIPS darwin archive.
        self.assertIn('"//packages/agent:macos_x86_64_base": "agent_data_plane_darwin_amd64"', archives)
        self.assertIn('"//packages/agent:macos_arm64_base": "agent_data_plane_darwin_arm64"', archives)
        self.assertNotIn("macos_x86_64_fips", archives)
        self.assertNotIn("macos_arm64_fips", archives)

        self.assertIn('name = "macos_x86_64_base"', packages_agent)
        self.assertIn('name = "macos_arm64_base"', packages_agent)
        self.assertNotIn("macos_x86_64_fips", packages_agent)
        self.assertNotIn("macos_arm64_fips", packages_agent)

    def test_adp_all_files_is_wired_into_dependencies_for_linux_macos_and_windows(self):
        dependencies = (REPO_ROOT / "packages/agent/dependencies/BUILD.bazel").read_text()

        self.assertIn('"//packages/agent:linux_default": [', dependencies)
        self.assertIn('"//packages/agent:linux_fips": [', dependencies)
        self.assertIn('"@platforms//os:macos": [\n            "//deps/agent_data_plane:all_files"', dependencies)
        self.assertEqual(dependencies.count("//deps/agent_data_plane:all_files"), 4)

    def test_system_launchdaemons_are_managed_consistently(self):
        # The agent/sysprobe/data-plane LaunchDaemons are no longer shipped as static plist XML
        # for postinst to cp into place: postinst installs all four stable jobs (including the
        # installer daemon) from the same embedded copies a Fleet configuration experiment swaps
        # between, via a single installer subcommand -- see InstallStableJobs in
        # pkg/fleet/installer/packages/datadog_agent_darwin.go. Go-side coverage for the rendered
        # job content itself lives in
        # pkg/fleet/installer/packages/launchd/jobset_test.go's
        # TestTheTwoVariantsDifferInExactlyFourWays.
        preinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/preinst").read_text()
        postinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst").read_text()
        uninstall = (REPO_ROOT / "cmd/agent/macos/uninstall_mac_os.sh").read_text()

        self.assertIn("install-stable-jobs", postinst)

        labels = [
            "com.datadoghq.agent",
            "com.datadoghq.sysprobe",
            "com.datadoghq.data-plane",
        ]
        for label in labels:
            with self.subTest(service=label):
                plist = f"/Library/LaunchDaemons/{label}.plist"
                # postinst still ships the GUI LaunchAgent and the AI usage desktop monitor this
                # way, so only the three jobs InstallStableJobs now owns are asserted absent here.
                self.assertNotIn(f"{label}.plist.example", postinst)
                self.assertIn(f"launchctl bootout system/{label}", preinst)
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
