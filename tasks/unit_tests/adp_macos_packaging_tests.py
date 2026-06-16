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

    def test_adp_dependency_is_included_on_linux_and_macos(self):
        dependencies = (REPO_ROOT / "omnibus/config/software/datadog-agent-dependencies.rb").read_text()

        self.assertIn("(linux_target? || osx_target?) && !heroku_target?", dependencies)

    def test_darwin_adp_hashes_and_url_base_are_forwarded_to_omnibus(self):
        darwin_env = OS_SPECIFIC_ENV_PASSTHROUGH["darwin"]

        self.assertIn("AGENT_DATA_PLANE_HASH_DARWIN_AMD64", darwin_env)
        self.assertIn("AGENT_DATA_PLANE_HASH_DARWIN_ARM64", darwin_env)
        self.assertIn("AGENT_DATA_PLANE_SOURCE_URL_BASE", ENV_PASSHTROUGH)

    def test_macos_app_installs_adp_launchdaemon_template(self):
        build_file = (REPO_ROOT / "packages/macos/app/BUILD.bazel").read_text()

        self.assertIn('name = "launchd_data_plane_plist_example"', build_file)
        self.assertIn('out = "etc/com.datadoghq.data-plane.plist.example"', build_file)
        self.assertIn('":launchd_data_plane_plist_example"', build_file)

    def test_agent_launchdaemon_watches_its_config(self):
        plist_path = REPO_ROOT / "packages/macos/app/launchd.plist.example.in"
        plist = plistlib.loads(plist_path.read_bytes())

        self.assertEqual(plist["Label"], "com.datadoghq.agent")
        self.assertEqual(plist["WatchPaths"], ["/opt/datadog-agent/etc/datadog.yaml"])

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
        self.assertEqual(plist["WatchPaths"], ["/opt/datadog-agent/etc/datadog.yaml"])
        self.assertEqual(plist["UserName"], "_dd-agent")
        self.assertEqual(plist["GroupName"], "daemon")

    def test_sysprobe_launchdaemon_watches_its_config(self):
        plist_path = REPO_ROOT / "packages/macos/app/launchd.sysprobe.plist.example.in"
        plist = plistlib.loads(plist_path.read_bytes())

        self.assertEqual(plist["Label"], "com.datadoghq.sysprobe")
        self.assertEqual(plist["WatchPaths"], ["/opt/datadog-agent/etc/system-probe.yaml"])

    def test_install_script_kickstarts_subprocesses_when_it_starts_agent(self):
        install_script = (REPO_ROOT / "cmd/agent/macos/install_mac_os_v1.sh").read_text()

        subprocesses = [
            ("data-plane", "data_plane", "com.datadoghq.data-plane"),
            ("system-probe", "sysprobe", "com.datadoghq.sysprobe"),
        ]
        for name, function_prefix, label in subprocesses:
            with self.subTest(subprocess=name):
                self.assertIn(f"{function_prefix}_service_name={label}", install_script)
                self.assertIn(f"kickstart_{function_prefix}()", install_script)
                self.assertIn(
                    f'$sudo_cmd launchctl kickstart "system/${function_prefix}_service_name"', install_script
                )
                kickstart_call = f"kickstart_{function_prefix}"
                self.assertGreater(
                    install_script.find('$cmd_launchctl start $service_name'),
                    -1,
                    "install script should still start the per-user agent",
                )
                self.assertGreater(
                    install_script.find(kickstart_call, install_script.find('$cmd_launchctl start $service_name')),
                    -1,
                    f"install script should kickstart {name} after restarting the per-user agent",
                )
                self.assertGreater(
                    install_script.find('launchctl kickstart "system/$service_name"'),
                    -1,
                    "install script should still kickstart the system agent",
                )
                self.assertGreater(
                    install_script.find(
                        kickstart_call,
                        install_script.find('launchctl kickstart "system/$service_name"'),
                    ),
                    -1,
                    f"install script should kickstart {name} after kickstarting the system agent",
                )

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
