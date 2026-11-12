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
        macos_srcs = dependencies.partition('"@platforms//os:macos": [')[2].partition("],")[0]
        self.assertIn('"//deps/agent_data_plane:all_files"', macos_srcs)
        self.assertEqual(dependencies.count("//deps/agent_data_plane:all_files"), 4)

    def test_system_launchdaemons_are_managed_consistently(self):
        # The agent/sysprobe/data-plane LaunchDaemons are no longer shipped as plist examples under
        # etc/ for postinst to cp into place: postinst runs `installer postinst datadog-agent dmg`,
        # which installs the stable jobs from the definitions embedded in the installer -- see
        # postInstallDatadogAgent and installStableJobs in
        # pkg/fleet/installer/packages/datadog_agent_darwin.go. Go-side coverage for the rendered
        # job content itself lives in pkg/fleet/installer/packages/embedded/embed_test.go.
        preinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/preinst").read_text()
        postinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst").read_text()
        uninstall = (REPO_ROOT / "cmd/agent/macos/uninstall_mac_os.sh").read_text()

        self.assertIn("postinst datadog-agent dmg", postinst)

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

    def test_preinst_hands_the_teardown_to_the_installed_installer(self):
        # Only the installed version knows the LaunchDaemons it installed, so preinst runs its
        # installer's pre-remove (preRemoveDatadogAgent in
        # pkg/fleet/installer/packages/datadog_agent_darwin.go) before booting out the jobs it knows
        # itself, as the rpm preinst does.
        preinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/preinst").read_text()

        supported = preinst.find('"$OLD_INSTALLER_BIN" is-prerm-supported')
        handoff = preinst.find('"$OLD_INSTALLER_BIN" prerm --upgrade datadog-agent dmg')
        fallback = preinst.find("echo '# Stopping running services'")
        self.assertNotEqual(supported, -1)
        self.assertNotEqual(handoff, -1)
        self.assertNotEqual(fallback, -1)
        self.assertLess(supported, handoff)
        self.assertLess(handoff, fallback)

    def test_uninstall_hands_the_teardown_to_the_installed_installer(self):
        # The uninstall script hands the uninstall to the installed installer's pre-remove, without
        # --upgrade, and only removes the Agent itself when there is no installer or it fails.
        uninstall = (REPO_ROOT / "cmd/agent/macos/uninstall_mac_os.sh").read_text()

        supported = uninstall.find('"$installer_bin" is-prerm-supported')
        handoff = uninstall.find('"$installer_bin" prerm datadog-agent dmg')
        definition = uninstall.find("uninstall_without_installer() {")
        fallback = uninstall.find("        uninstall_without_installer\n", handoff)
        self.assertNotEqual(supported, -1)
        self.assertNotEqual(handoff, -1)
        self.assertNotEqual(definition, -1)
        self.assertNotEqual(fallback, -1)
        self.assertNotIn("prerm --upgrade", uninstall)
        self.assertLess(definition, supported)
        self.assertLess(supported, handoff)
        self.assertLess(handoff, fallback)

    def test_preinst_kills_every_detached_experiment_process(self):
        # An upgrade must not leave a detached configuration experiment process (the watcher, or the
        # installer daemon hand-over or hand-back) swapping jobs back in while the binaries it runs
        # are replaced.
        preinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/preinst").read_text()
        pkill = next(line for line in preinst.splitlines() if "pkill -9 -f" in line and "package-command" in line)
        for command in ("watchConfigExperiment", "handOverInstaller", "handBackInstaller"):
            self.assertIn(command, pkill)

    def test_uninstall_without_installer_removes_the_experiment_jobs(self):
        # When the installer can't uninstall, the script still removes a configuration experiment's
        # jobs, before the stable ones, so none is left registered against binaries that are gone.
        uninstall = (REPO_ROOT / "cmd/agent/macos/uninstall_mac_os.sh").read_text()
        start = uninstall.find("uninstall_without_installer() {")
        end = uninstall.find("\n}\n", start)
        self.assertNotEqual(start, -1)
        body = uninstall[start:end]
        first_stable_bootout = body.find("launchctl bootout system/com.datadoghq.procmgr")
        for label in (
            "com.datadoghq.installer-exp",
            "com.datadoghq.agent-exp",
            "com.datadoghq.sysprobe-exp",
            "com.datadoghq.data-plane-exp",
        ):
            bootout = body.find(f"launchctl bootout system/{label} ")
            self.assertNotEqual(bootout, -1, label)
            self.assertLess(bootout, first_stable_bootout, label)
            self.assertIn(f"rm -f /Library/LaunchDaemons/{label}.plist", body)

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

    def test_postinst_fails_the_install_when_the_installer_fails(self):
        # postinst hands the whole install to `installer postinst datadog-agent dmg`, as deb and rpm
        # do, and exits non-zero when the installer is missing or fails, so Installer.app, MDM or
        # `installer -pkg` report the failure. It has no shell copy of the installer's work.
        postinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst").read_text()
        build_file = (REPO_ROOT / "packages/macos/app/BUILD.bazel").read_text()

        self.assertIn('"$INSTALLER_BIN" postinst datadog-agent dmg', postinst)
        self.assertTrue(postinst.rstrip().endswith("exit $status"))
        self.assertEqual(postinst.count("status=1"), 2)
        for absent in ["launchctl", "dscl", "fallback", "embedded/share/launchd"]:
            with self.subTest(absent=absent):
                self.assertNotIn(absent, postinst)
        self.assertNotIn("launchd_fallback_jobs", build_file)

    def test_launchdaemons_start_when_bootstrapped(self):
        # The installer starts the jobs by bootstrapping them, never with `launchctl kickstart`: a
        # job launchd cannot spawn, such as one whose account is missing, blocks that call, and
        # Installer.app aborts the install after waiting on it.
        for label in [
            "com.datadoghq.procmgr",
            "com.datadoghq.agent",
            "com.datadoghq.sysprobe",
            "com.datadoghq.data-plane",
            "com.datadoghq.installer",
        ]:
            with self.subTest(job=label):
                definition = (
                    REPO_ROOT / f"pkg/fleet/installer/packages/embedded/tmpl/gen/darwin/{label}.plist"
                ).read_text()
                self.assertRegex(definition, r"<key>RunAtLoad</key>\s*<true/>")
