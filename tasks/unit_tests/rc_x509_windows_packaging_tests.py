import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestRcX509WindowsPackaging(unittest.TestCase):
    def test_runtime_dlls_are_wired_through_package_msi_and_signing(self):
        dependencies = (REPO_ROOT / "packages/agent/dependencies/BUILD.bazel").read_text()
        agent_binaries = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/WixSetup/Datadog Agent/AgentBinaries.cs"
        ).read_text()
        agent_installer = (
            REPO_ROOT / "tools/windows/DatadogAgentInstaller/WixSetup/Datadog Agent/AgentInstaller.cs"
        ).read_text()
        omnibus_project = (REPO_ROOT / "omnibus/config/projects/agent.rb").read_text()
        signing_block = omnibus_project.partition("BINARIES_TO_SIGN =")[2].partition("BINARIES_TO_SIGN.each")[0]

        expected_dlls = {
            "RcX509Ffi": ("rc_x509_ffi.dll", "//deps/libdd_rc:rc_x509_ffi_windows_dll"),
            "AwsLcFipsCrypto": (
                "aws_lc_fips_0_14_2_crypto.dll",
                "//deps/libdd_rc:aws_lc_fips_crypto_windows_dll",
            ),
        }
        for property_name, (filename, package_target) in expected_dlls.items():
            with self.subTest(dll=filename):
                self.assertIn(package_target, dependencies)
                self.assertIn(f"public string {property_name}", agent_binaries)
                self.assertIn(filename, agent_binaries)
                self.assertIn(f"new WixSharp.File(_agentBinaries.{property_name})", agent_installer)
                self.assertIn(filename, signing_block)

    def test_runtime_dependencies_are_in_license_collection(self):
        dependencies = (REPO_ROOT / "packages/agent/dependencies/BUILD.bazel").read_text()
        libdd_rc_build = (REPO_ROOT / "deps/libdd_rc/libdd_rc.BUILD.bazel").read_text()
        libdd_rc_module = (REPO_ROOT / "deps/libdd_rc/libdd_rc.MODULE.bazel").read_text()

        license_rule = dependencies.partition("package_licenses(")[2].partition(")")[0]
        self.assertIn('src = ":all_files"', license_rule)
        self.assertIn('package = "libdd-rc"', libdd_rc_build)
        self.assertIn('package = "aws-lc-fips-sys"', libdd_rc_build)
        self.assertIn('license_text = "LICENSE"', libdd_rc_build)
        self.assertIn("aws-lc-fips-sys-0.14.2//:LICENSE", libdd_rc_build)
        self.assertIn('exports_files(["LICENSE"], visibility = ["//visibility:public"])', libdd_rc_module)

    def test_windows_smoke_checks_pe_layout_and_public_client(self):
        lifecycle = (REPO_ROOT / "deps/libdd_rc/smoke/run_windows_msvc_lifecycle.ps1").read_text()
        lifecycle_go = (REPO_ROOT / "deps/libdd_rc/smoke/windows_msvc_lifecycle.go").read_text()

        self.assertIn("github.com/DataDog/libdd-rc/ffi-hosts/go/rcx509", lifecycle_go)
        self.assertIn("rcx509.NewClient", lifecycle_go)
        self.assertIn("client.Close()", lifecycle_go)
        self.assertIn("@winlibs_mingw64//:objdump", lifecycle)
        self.assertIn("file format pei-x86-64", lifecycle)
        self.assertIn("rc_x509_ffi.dll", lifecycle)
        self.assertIn("aws_lc_fips_0_14_2_crypto.dll", lifecycle)
        self.assertIn("bazel-out|execroot", lifecycle)


if __name__ == "__main__":
    unittest.main()
