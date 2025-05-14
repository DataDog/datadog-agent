import unittest
from typing import TYPE_CHECKING, cast

from tasks.kernel_matrix_testing import platforms, vmconfig
from tasks.libs.types.arch import Arch

if TYPE_CHECKING:
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

        self.assertEqual(
            vmconfig.normalize_vm_def(possible, "ubuntu22-arm64-distro"), ("distro", "ubuntu_22.04", "arm64")
        )
        self.assertEqual(
            vmconfig.normalize_vm_def(possible, "ubuntu22-x86_64-distro"), ("distro", "ubuntu_22.04", "x86_64")
        )
        self.assertEqual(vmconfig.normalize_vm_def(possible, "focal-arm64-distro"), ("distro", "ubuntu_20.04", "arm64"))
        self.assertEqual(
            vmconfig.normalize_vm_def(possible, "focal-x86_64-distro"), ("distro", "ubuntu_20.04", "x86_64")
        )
