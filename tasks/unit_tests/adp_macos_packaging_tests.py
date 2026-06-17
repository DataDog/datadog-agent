import plistlib
import unittest
from pathlib import Path

from tasks.libs.common.omnibus import ENV_PASSHTROUGH, OS_SPECIFIC_ENV_PASSTHROUGH

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestADPMacOSPackaging(unittest.TestCase):
    def test_omnibus_recipe_selects_darwin_artifacts_and_supports_url_base_override(self):
        recipe = (REPO_ROOT / "omnibus/config/software/datadog-agent-data-plane.rb").read_text()

        self.assertIn("AGENT_DATA_PLANE_HASH_DARWIN_AMD64", recipe)
        self.assertIn("AGENT_DATA_PLANE_HASH_DARWIN_ARM64", recipe)
        self.assertIn("AGENT_DATA_PLANE_SOURCE_URL_BASE", recipe)
        self.assertIn('package_target = "darwin-#{target_arch}"', recipe)
        self.assertIn("Agent Data Plane FIPS artifacts are not available for macOS", recipe)

    def test_adp_dependency_is_included_on_supported_desktop_and_server_platforms(self):
        dependencies = (REPO_ROOT / "omnibus/config/software/datadog-agent-dependencies.rb").read_text()

        self.assertIn("(linux_target? || osx_target? || windows_target?) && !heroku_target?", dependencies)

    def test_darwin_adp_hashes_and_url_base_are_forwarded_to_omnibus(self):
        darwin_env = OS_SPECIFIC_ENV_PASSTHROUGH["darwin"]

        self.assertIn("AGENT_DATA_PLANE_HASH_DARWIN_AMD64", darwin_env)
        self.assertIn("AGENT_DATA_PLANE_HASH_DARWIN_ARM64", darwin_env)
        self.assertIn("AGENT_DATA_PLANE_SOURCE_URL_BASE", ENV_PASSHTROUGH)

    def test_omnibus_recipe_selects_windows_artifacts(self):
        recipe = (REPO_ROOT / "omnibus/config/software/datadog-agent-data-plane.rb").read_text()

        self.assertIn("AGENT_DATA_PLANE_HASH_WINDOWS_AMD64", recipe)
        self.assertIn("AGENT_DATA_PLANE_HASH_FIPS_WINDOWS_AMD64", recipe)
        self.assertIn('package_target = "windows-#{target_arch}"', recipe)
        self.assertIn('package_target = "#{package_target}-fips"', recipe)
        self.assertIn('package_extension = "zip"', recipe)
        self.assertIn("copy 'bin/agent-data-plane.exe'", recipe)
        self.assertIn("copy 'bin/aws_lc_fips_*_crypto.dll'", recipe)

    def test_adp_dependency_is_included_on_windows(self):
        dependencies = (REPO_ROOT / "omnibus/config/software/datadog-agent-dependencies.rb").read_text()

        self.assertIn("(linux_target? || osx_target? || windows_target?) && !heroku_target?", dependencies)

    def test_windows_adp_hashes_are_forwarded_to_omnibus(self):
        windows_env = OS_SPECIFIC_ENV_PASSTHROUGH["win32"]

        self.assertIn("AGENT_DATA_PLANE_HASH_WINDOWS_AMD64", windows_env)
        self.assertIn("AGENT_DATA_PLANE_HASH_FIPS_WINDOWS_AMD64", windows_env)

    def test_agent_project_strips_and_signs_adp_binary_on_windows(self):
        project = (REPO_ROOT / "omnibus/config/projects/agent.rb").read_text()

        self.assertIn('"#{install_dir}\\\\bin\\\\agent\\\\agent-data-plane.exe"', project)

    def test_windows_installer_packages_adp_binary(self):
        binaries = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/WixSetup/Datadog Agent/AgentBinaries.cs"
        ).read_text()
        installer = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/WixSetup/Datadog Agent/AgentInstaller.cs"
        ).read_text()

        self.assertIn('public string AgentDataPlane => $@"{_binSource}\\agent-data-plane.exe";', binaries)
        self.assertIn(
            "agentBinDir.AddFile(new WixSharp.File(_agentBinaries.AgentDataPlane, dataPlaneService));", installer
        )

    def test_windows_installer_registers_adp_service(self):
        constants = (REPO_ROOT / "tools/windows/DatadogAgentInstaller/CustomActions/Constants.cs").read_text()
        installer = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/WixSetup/Datadog Agent/AgentInstaller.cs"
        ).read_text()

        self.assertIn('public const string DataPlaneServiceName = "datadog-agent-data-plane";', constants)
        self.assertIn('new Id("ddagentdataplaneservice")', installer)
        self.assertIn("Constants.DataPlaneServiceName", installer)
        self.assertIn(
            "agentBinDir.AddFile(new WixSharp.File(_agentBinaries.AgentDataPlane, dataPlaneService));", installer
        )
        self.assertIn('agentBinDir.Add(new Files($@"{BinSource}\\aws_lc_fips_*_crypto.dll"));', installer)

    def test_windows_custom_actions_manage_adp_service(self):
        service_actions = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/CustomActions/ServiceCustomAction.cs"
        ).read_text()

        self.assertIn("_serviceController.SetCredentials(Constants.DataPlaneServiceName", service_actions)
        self.assertIn("Constants.DataPlaneServiceName,", service_actions)

    def test_windows_runtime_config_and_service_launcher_include_adp(self):
        config_setup = (REPO_ROOT / "pkg/config/setup/config.go").read_text()
        dependent_services = (REPO_ROOT / "cmd/agent/subcommands/run/dependent_services_windows.go").read_text()

        self.assertIn('goos == "linux" || goos == "darwin" || goos == "windows"', config_setup)
        self.assertIn('"data_plane.enabled": coreConf', dependent_services)
        self.assertIn('serviceName:    "datadog-agent-data-plane"', dependent_services)

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
