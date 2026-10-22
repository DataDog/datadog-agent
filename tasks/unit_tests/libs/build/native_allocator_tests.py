import os
import shlex
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tasks.libs.build.native_allocator import append_extldflags, with_native_allocator


class TestNativeAllocator(unittest.TestCase):
    def test_composes_external_flags(self):
        original = "-s -w -r /embedded/lib '-extldflags=-Wl,--version-script=agent.map -Wl,-z,lazy -static'"
        args = shlex.split(append_extldflags(original, "-Wl,--whole-archive /jemalloc.a -Wl,--no-whole-archive"))
        external = [arg for arg in args if arg.startswith("-extldflags=")]
        self.assertEqual(len(external), 1)
        for flag in ["--version-script=agent.map", "-Wl,-z,lazy", "-static", "/jemalloc.a"]:
            self.assertIn(flag, external[0])
        self.assertEqual(args[:5], ["-s", "-w", "-r", "/embedded/lib", external[0]])

    @patch("tasks.libs.build.native_allocator.bazel")
    @patch("tasks.libs.build.native_allocator.sys.platform", "linux")
    def test_other_target_operating_systems_never_build_allocator(self, bazel):
        for target in ["windows", "darwin", "aix", "freebsd"]:
            with self.subTest(target=target), patch.dict(os.environ, {"GOOS": target}, clear=True):
                self.assertEqual(with_native_allocator("-w", {"CGO_ENABLED": "1"}), "-w")
        bazel.assert_not_called()

    @patch("tasks.libs.build.native_allocator.bazel")
    def test_pure_go_never_builds_allocator(self, bazel):
        with patch.dict(os.environ, {"GOOS": "linux", "CGO_ENABLED": "1"}, clear=True):
            self.assertEqual(with_native_allocator("-w", {"CGO_ENABLED": "0"}), "-w")
        bazel.assert_not_called()

    @patch("tasks.libs.build.native_allocator.sys.platform", "darwin")
    def test_linux_target_and_architecture_override_host(self):
        with tempfile.TemporaryDirectory() as root:
            archive = Path(root, "libjemalloc_pic.a")
            archive.touch()
            for arch, platform in [("amd64", "linux_x86_64"), ("arm64", "linux_arm64")]:
                with self.subTest(arch=arch), patch.dict(os.environ, {}, clear=True):
                    with patch("tasks.libs.build.native_allocator.bazel", return_value=str(archive)) as bazel:
                        flags = with_native_allocator("-w", {"GOOS": "linux", "GOARCH": arch, "CGO_ENABLED": "1"})
                    self.assertIn(f"--platforms=//bazel/platforms:{platform}", bazel.call_args_list[0].args)
                    self.assertIn(str(archive), flags)
                    self.assertIn("-linkmode=external", flags)
                    self.assertEqual(flags.count("--version-script="), 1)
                    self.assertNotIn("-ljemalloc", flags)

    @patch("tasks.libs.build.native_allocator.bazel")
    def test_call_environment_takes_precedence(self, bazel):
        with patch.dict(os.environ, {"GOOS": "linux"}, clear=True):
            self.assertEqual(with_native_allocator("", {"GOOS": "windows"}), "")
        bazel.assert_not_called()

    def test_both_go_external_flag_spellings(self):
        flags = append_extldflags("-extldflags '-Wl,-z,lazy' -w", "-lm")
        self.assertEqual(shlex.split(flags), ["-w", "-extldflags=-Wl,-z,lazy -lm"])

    def test_archive_and_map_paths_with_spaces(self):
        with tempfile.TemporaryDirectory(prefix="allocator build ") as root:
            archive = Path(root, "libjemalloc_pic.a")
            archive.touch()
            with (
                patch.dict(os.environ, {"GOOS": "linux", "GOARCH": "arm64"}, clear=True),
                patch("tasks.libs.build.native_allocator.bazel", return_value=str(archive)),
                patch("tasks.libs.build.native_allocator.get_repo_root", return_value=root),
            ):
                flags = shlex.split(with_native_allocator("-w", {}))
            external = next(flag.removeprefix("-extldflags=") for flag in flags if flag.startswith("-extldflags="))
            self.assertIn(str(archive.resolve()), shlex.split(external))
            self.assertIn(f"-Wl,--version-script={root}/datadog-agent.map", shlex.split(external))

    def test_resolves_outputs_without_workspace_convenience_symlinks(self):
        with tempfile.TemporaryDirectory() as execroot:
            relative = "bazel-out/target/bin/libjemalloc_pic.a"
            archive = Path(execroot, relative)
            archive.parent.mkdir(parents=True)
            archive.touch()
            with (
                patch.dict(os.environ, {"GOOS": "linux", "GOARCH": "arm64"}, clear=True),
                patch("tasks.libs.build.native_allocator.bazel", side_effect=["", relative, execroot]),
            ):
                flags = with_native_allocator("", {})
            self.assertIn(str(archive), flags)
