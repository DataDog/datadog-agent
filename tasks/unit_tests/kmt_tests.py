import unittest
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
