import tempfile
import unittest
import zipfile
from pathlib import Path

from deps.agent_integrations import freeze


def make_wheel(
    directory: Path,
    distribution: str,
    version: str,
    *,
    metadata_name: str | None = None,
    metadata_version: str | None = None,
) -> Path:
    """Create a minimal wheel suitable for reading dist-info metadata."""
    wheel_name = f"{distribution}-{version}-py3-none-any.whl"
    wheel_path = directory / wheel_name
    dist_info = f"{distribution}-{version}.dist-info"
    metadata_lines = [
        "Metadata-Version: 2.1",
    ]
    if metadata_name is not None:
        metadata_lines.append(f"Name: {metadata_name}")
    if metadata_version is not None:
        metadata_lines.append(f"Version: {metadata_version}")

    with zipfile.ZipFile(wheel_path, "w") as wheel:
        wheel.writestr(f"{dist_info}/METADATA", "\n".join(metadata_lines) + "\n")
        wheel.writestr(f"{dist_info}/RECORD", "")

    return wheel_path


class FreezeTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.tmpdir = Path(self.tmp.name)

    def make_wheel(self, *args, directory: Path | None = None, **kwargs) -> Path:
        return make_wheel(directory or self.tmpdir, *args, **kwargs)

    def test_generate_constraints_reads_direct_wheels_and_sorts_by_normalized_name(self):
        zed = self.make_wheel("zed_pkg", "1.0.0", metadata_name="Zed-Pkg", metadata_version="1.0.0")
        alpha = self.make_wheel("alpha_pkg", "2.0.0", metadata_name="alpha_pkg", metadata_version="2.0.0")

        self.assertEqual(
            freeze.generate_constraints([zed, alpha]),
            [
                "alpha_pkg==2.0.0",
                "Zed-Pkg==1.0.0",
            ],
        )

    def test_generate_constraints_expands_wheelhouse_inputs(self):
        wheelhouse_a = self.tmpdir / "wheelhouse_a"
        wheelhouse_a.mkdir()
        wheelhouse_b = self.tmpdir / "wheelhouse_b"
        wheelhouse_b.mkdir()
        self.make_wheel("pkg_a", "2.0.0", directory=wheelhouse_a, metadata_name="pkg-a", metadata_version="2.0.0")
        self.make_wheel("pkg_b", "1.0.0", directory=wheelhouse_b, metadata_name="pkg-b", metadata_version="1.0.0")

        self.assertEqual(
            freeze.generate_constraints([wheelhouse_b, wheelhouse_a]),
            [
                "pkg-a==2.0.0",
                "pkg-b==1.0.0",
            ],
        )

    def test_generate_constraints_deduplicates_same_normalized_name_and_version(self):
        first_dir = self.tmpdir / "first"
        second_dir = self.tmpdir / "second"
        first_dir.mkdir()
        second_dir.mkdir()
        first = self.make_wheel(
            "foo_bar", "1.0.0", directory=first_dir, metadata_name="foo-bar", metadata_version="1.0.0"
        )
        second = self.make_wheel(
            "foo_bar", "1.0.0", directory=second_dir, metadata_name="foo_bar", metadata_version="1.0.0"
        )

        self.assertEqual(freeze.generate_constraints([first, second]), ["foo-bar==1.0.0"])

    def test_generate_constraints_rejects_conflicting_versions_for_same_normalized_name(self):
        first_dir = self.tmpdir / "first"
        second_dir = self.tmpdir / "second"
        first_dir.mkdir()
        second_dir.mkdir()
        first = self.make_wheel(
            "foo_bar", "1.0.0", directory=first_dir, metadata_name="foo-bar", metadata_version="1.0.0"
        )
        second = self.make_wheel(
            "foo_bar", "2.0.0", directory=second_dir, metadata_name="foo_bar", metadata_version="2.0.0"
        )

        with self.assertRaisesRegex(ValueError, "conflicting versions for foo-bar"):
            freeze.generate_constraints([first, second])

    def test_read_name_and_version_rejects_missing_metadata_fields(self):
        wheel = self.make_wheel("missing_version", "1.0.0", metadata_name="missing-version")

        with self.assertRaisesRegex(ValueError, "missing Name or Version"):
            freeze.read_name_and_version(wheel)


if __name__ == "__main__":
    unittest.main()
