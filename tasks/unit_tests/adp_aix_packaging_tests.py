import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
AIX_ROOT = REPO_ROOT / "packaging/aix"


class TestADPAIXPackaging(unittest.TestCase):
    def test_aix_build_checks_out_saluki_version_from_release_json(self):
        env_sh = (AIX_ROOT / "lib/env.sh").read_text()
        checkout = (AIX_ROOT / "stages/00-checkout.sh").read_text()

        self.assertIn("SALUKI_SRC=", env_sh)
        self.assertIn("AGENT_DATA_PLANE_VERSION", env_sh)
        self.assertIn("DataDog/saluki", checkout)
        self.assertIn("$AGENT_DATA_PLANE_VERSION", checkout)

    def test_aix_build_runs_agent_data_plane_stage_after_agent_stage(self):
        build_sh = (AIX_ROOT / "build.sh").read_text()

        self.assertIn("04-agent\n04-agent-data-plane\n05-python-extensions", build_sh)

    def test_agent_data_plane_stage_builds_with_saluki_aix_target_and_installs_artifacts(self):
        stage = (AIX_ROOT / "stages/04-agent-data-plane.sh").read_text()

        self.assertIn('ADP_AIX_BUILD_COMMAND="make build-adp-aix"', stage)
        self.assertIn('if [ "${ADP_AIX_BUILD_COMMAND+x}" = x ]; then', stage)
        self.assertIn("ADP_AIX_BUILD_PROFILE=${ADP_AIX_BUILD_PROFILE:-aix-optimized-release}", stage)
        self.assertIn("CARGO_TARGET_DIR=${CARGO_TARGET_DIR:-$BUILD_DIR/saluki-target}", stage)
        self.assertIn("$CARGO_TARGET_DIR/$ADP_AIX_BUILD_PROFILE/agent-data-plane", stage)
        self.assertIn("unset ARFLAGS", stage)
        self.assertIn("BUILD_PROFILE=\"$ADP_AIX_BUILD_PROFILE\" sh -c \"$ADP_AIX_BUILD_COMMAND\"", stage)
        self.assertIn("ADP_RELEASE_TARBALL_PATH", stage)
        self.assertIn("Prebuilt artifacts must be explicit", stage)
        self.assertIn("license-list-data/archive/refs/tags/v$ADP_SPDX_LICENSES_VERSION.tar.gz", stage)
        self.assertIn("gzip -dc", stage)
        self.assertNotIn("fetch-spdx-licenses.sh", stage)
        self.assertIn("collect-third-party-licenses.sh", stage)
        self.assertNotIn('ADP_THIRD_PARTY_SRC="$SALUKI_SRC/LICENSES"', stage)
        self.assertIn("agent-data-plane", stage)
        self.assertIn("LICENSE-agent-data-plane-3rdparty.csv", stage)
        self.assertIn("THIRD-PARTY-*", stage)
        self.assertIn("XCOFF64", stage)

    def test_aix_build_checks_saluki_aix_build_tools(self):
        build_sh = (AIX_ROOT / "build.sh").read_text()

        self.assertIn("check_tool bash", build_sh)
        self.assertIn("check_tool protoc", build_sh)

    def test_aix_package_lifecycle_manages_agent_data_plane_src_service(self):
        service = "datadog-agent-data-plane"
        for script_name in ["preinst", "postinst", "config", "prerm", "unconfig"]:
            with self.subTest(script=script_name):
                script = (AIX_ROOT / f"package-scripts/{script_name}").read_text()
                self.assertIn(service, script)

        postinst = (AIX_ROOT / "package-scripts/postinst").read_text()
        self.assertIn("/opt/datadog-agent/embedded/bin/agent-data-plane", postinst)
        self.assertIn("--pidfile /opt/datadog-agent/run/agent-data-plane.pid", postinst)

        unconfig = (AIX_ROOT / "package-scripts/unconfig").read_text()
        self.assertNotIn("odmdelete -o SRCsubsys -q \"subsysname='datadog-agent-data-plane'\"", unconfig)

    def test_aix_package_preflight_requires_agent_data_plane_artifacts(self):
        package_sh = (AIX_ROOT / "package.sh").read_text()

        self.assertIn("agent-data-plane", package_sh)
        self.assertIn("LICENSE-agent-data-plane-3rdparty.csv", package_sh)
        self.assertIn("THIRD-PARTY", package_sh)
