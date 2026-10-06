import os
import tempfile
import unittest
from pathlib import Path
from typing import TYPE_CHECKING, cast

from tasks.kernel_matrix_testing import platforms, vmconfig
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS
from tasks.libs.types.arch import Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Component
    from tasks.libs.types.arch import KMTArchName


class TestVmconfig(unittest.TestCase):
    def test_all_list_possible__items_map_to_existing_platforms(self):
        possible = vmconfig.list_possible()
        plats = platforms.get_platforms()

        for name in possible:
            # Only test distros, not custom kernels
            if "distro" not in name:
                continue

            vmdef = vmconfig.normalize_vm_def(possible, name)
            _, version, arch = vmdef

            if arch == "local":
                arch = Arch.local().kmt_arch

            self.assertIn(arch, plats, f"{name} selects architecture {arch} which does not exist in the platform list")
            self.assertIn(
                version,
                plats[cast("KMTArchName", arch)],
                f"{name} maps to {version} which is not a valid version for architecture {arch}",
            )

    def test_normalize_vm_def__returns_expected_values(self):
        possible = vmconfig.list_possible()

        cases = [
            ("ubuntu22-arm64-distro", ("distro", "ubuntu_22.04", "arm64")),
            ("ubuntu22-x86_64-distro", ("distro", "ubuntu_22.04", "x86_64")),
            ("focal-arm64-distro", ("distro", "ubuntu_20.04", "arm64")),
            ("focal-x86_64-distro", ("distro", "ubuntu_20.04", "x86_64")),
            ("ubuntu_22-arm64-distro", ("distro", "ubuntu_22.04", "arm64")),
            ("ubuntu-22-x86_64-distro", ("distro", "ubuntu_22.04", "x86_64")),
        ]

        for input, expected in cases:
            self.assertEqual(vmconfig.normalize_vm_def(possible, input), expected)


class TestFilterByCIComponent(unittest.TestCase):
    """The generated vmconfig decides which microVMs get booted on the metal instances.

    Any (test set, architecture) pair that has no CI job would boot VMs nothing ever
    connects to, while still eating vCPU and memory on a heavily oversubscribed box.
    """

    components: "list[Component]" = ["security-agent", "system-probe"]

    def test_no_microvms_without_a_matching_ci_job(self):
        plats = platforms.get_platforms()

        for component in self.components:
            expected: dict[tuple[str, str], set[str]] = {}
            for job in platforms.get_ci_test_jobs(component):
                for test_set in job.test_set:
                    expected.setdefault((test_set, job.arch), set()).update(job.kernels)

            by_set = platforms.filter_by_ci_component(plats, component)
            for test_set, plat in by_set.items():
                for arch in KMT_SUPPORTED_ARCHS:
                    self.assertEqual(
                        set(plat[arch].keys()),
                        expected.get((test_set, arch), set()),
                        f"{component}: vmset {test_set} on {arch} does not match the kernels "
                        "of the CI jobs running that test set",
                    )

    def test_every_ci_job_gets_its_microvms(self):
        plats = platforms.get_platforms()

        for component in self.components:
            by_set = platforms.filter_by_ci_component(plats, component)
            for job in platforms.get_ci_test_jobs(component):
                for test_set in job.test_set:
                    self.assertIn(test_set, by_set, f"{component}: job {job.name} has no vmset")
                    self.assertTrue(
                        job.kernels.issubset(by_set[test_set][job.arch].keys()),
                        f"{component}: job {job.name} is missing microVMs for "
                        f"{job.kernels - set(by_set[test_set][job.arch].keys())}",
                    )


class TestKMTHelperTargets(unittest.TestCase):
    def test_linux_platforms(self):
        from tasks.system_probe import linux_platform_flags

        self.assertEqual(linux_platform_flags(Arch.from_str("x86_64")), ["--platforms=//bazel/platforms:linux_x86_64"])
        self.assertEqual(linux_platform_flags(Arch.from_str("arm64")), ["--platforms=//bazel/platforms:linux_arm64"])

    def test_helper_dests_are_unique_and_exist(self):
        from tasks.kmt import _KMT_PKG_HELPER_TARGETS, _KMT_TOOL_TARGETS, kmt_pkg_helper_dest

        pkg_dests = [kmt_pkg_helper_dest(t) for t in _KMT_PKG_HELPER_TARGETS]
        self.assertEqual(kmt_pkg_helper_dest("//pkg/gpu/testdata:cudasample"), "pkg/gpu/testdata/cudasample")

        dests = list(_KMT_TOOL_TARGETS.values()) + pkg_dests
        self.assertEqual(len(dests), len(set(dests)))

        repo = Path(__file__).resolve().parents[2]
        for dest in pkg_dests:
            # dest is .../<pkg>/<binary>; the package dir must exist in the tree
            pkg_dir = repo / Path(dest).parent
            self.assertTrue(pkg_dir.is_dir(), pkg_dir)

        self.assertTrue((repo / "pkg/gpu/testdata/cudasample.c").is_file())
        self.assertTrue((repo / "pkg/gpu/testdata/BUILD.bazel").is_file())


class TestKMTGoTestTargetPick(unittest.TestCase):
    KMT_TAGS = {"bpf", "ec2", "netcgo", "npm", "nvml", "test", "zlib"}

    @staticmethod
    def _go_test(label: str, macro: str, *gotags: str):
        from tasks.kmt import KMTGoTest

        return KMTGoTest(label, macro, frozenset(gotags))

    def _pick(self, pkg, tests):
        from tasks.kmt import pick_kmt_go_test_target

        return pick_kmt_go_test_target(pkg, tests, self.KMT_TAGS)

    def test_prefers_variant_covering_most_kmt_tags(self):
        tests = [
            self._go_test("//pkg/gpu:gpu_test", "gpu_test", "test"),
            self._go_test("//pkg/gpu:gpu_test_bpf", "gpu_test", "bpf", "test"),
            self._go_test("//pkg/gpu:gpu_test_bpf_nvml", "gpu_test", "bpf", "nvml", "test"),
        ]
        self.assertEqual(self._pick("pkg/gpu", tests), "//pkg/gpu:gpu_test_bpf_nvml")

    def test_ties_go_to_bpf(self):
        tests = [
            self._go_test("//pkg/network/usm:usm_test_npm", "usm_test", "npm", "test"),
            self._go_test("//pkg/network/usm:usm_test_bpf", "usm_test", "bpf", "test"),
        ]
        self.assertEqual(self._pick("pkg/network/usm", tests), "//pkg/network/usm:usm_test_bpf")

    def test_skips_variants_needing_tags_kmt_does_not_build(self):
        tests = [
            self._go_test("//pkg/foo:foo_test", "foo_test", "test"),
            self._go_test("//pkg/foo:foo_test_docker", "foo_test", "docker", "test"),
        ]
        self.assertEqual(self._pick("pkg/foo", tests), "//pkg/foo:foo_test")

    def test_ignores_split_go_test_rules(self):
        tests = [
            self._go_test("//pkg/dyninst/loader:relocations_test_bpf", "relocations_test", "bpf", "test"),
            self._go_test("//pkg/dyninst/loader:stats_test_bpf", "stats_test", "bpf", "test"),
        ]
        self.assertIsNone(self._pick("pkg/dyninst/loader", tests))

    def test_missing_package(self):
        tests = [self._go_test("//pkg/ebpf:ebpf_test_bpf", "ebpf_test", "bpf", "test")]
        self.assertIsNone(self._pick("pkg/network/usm", tests))


class TestCopyKMTTestdata(unittest.TestCase):
    def test_copies_testdata_of_each_package(self):
        from tasks.kmt import copy_kmt_testdata

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            src = root / "src"
            (src / "pkg/a/testdata").mkdir(parents=True)
            (src / "pkg/a/testdata/fixture.yml").write_text("a")
            (src / "pkg/b").mkdir(parents=True)
            dest = root / "dest"

            cwd = os.getcwd()
            os.chdir(src)
            try:
                copy_kmt_testdata(dest, [str(src / "pkg/a"), str(src / "pkg/b")])
            finally:
                os.chdir(cwd)

            self.assertEqual((dest / "pkg/a/testdata/fixture.yml").read_text(), "a")
            self.assertFalse((dest / "pkg/b").exists())
