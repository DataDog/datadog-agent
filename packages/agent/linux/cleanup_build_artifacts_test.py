# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

import tempfile
import unittest
from pathlib import Path

from packages.agent.linux.cleanup_build_artifacts import cleanup_build_artifacts


class CleanupBuildArtifactsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.install_dir = Path(self.temporary_directory.name)
        self.embedded_dir = self.install_dir / "embedded"
        self.site_packages = self.embedded_dir / "lib/python3.13/site-packages"

    def tearDown(self) -> None:
        self.temporary_directory.cleanup()

    def create_file(self, relative_path: str, content: bytes = b"content") -> Path:
        path = self.install_dir / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
        return path

    def test_removes_linux_agent_build_artifacts(self) -> None:
        removed_paths = [
            "embedded/include/python3.13/Python.h",
            "embedded/include/openssl/ssl.h",
            "embedded/lib/libpcap.a",
            "embedded/lib/python3.13/site-packages/psycopg_c/_psycopg.c",
            "embedded/lib/python3.13/site-packages/psycopg_c/pq.c",
            "embedded/lib/python3.13/site-packages/package/header.h",
            "embedded/lib/python3.13/site-packages/package/header.hpp",
            "embedded/lib/python3.13/site-packages/package/module.pyx",
            "embedded/lib/python3.13/site-packages/package/declaration.pxd",
            "embedded/lib/python3.13/site-packages/package/include.pxi",
            "embedded/lib/python3.13/site-packages/package/CMakeLists.txt",
            "embedded/lib/python3.13/site-packages/package/build.cmake",
            "embedded/lib/python3.13/site-packages/package/config.cmake.in",
        ]
        for path in removed_paths:
            self.create_file(path)

        cleanup_build_artifacts(self.install_dir)

        for path in removed_paths:
            self.assertFalse((self.install_dir / path).exists(), path)

    def test_preserves_runtime_and_wheel_metadata(self) -> None:
        preserved_paths = [
            "embedded/lib/python3.13/site-packages/package/__init__.py",
            "embedded/lib/python3.13/site-packages/package/api.pyi",
            "embedded/lib/python3.13/site-packages/package/extension.so",
            "embedded/lib/python3.13/site-packages/package/config.yaml",
            "embedded/lib/python3.13/site-packages/package/LICENSE",
            "embedded/lib/python3.13/site-packages/package-1.0.dist-info/METADATA",
            "embedded/lib/python3.13/site-packages/package-1.0.dist-info/RECORD",
            "embedded/lib/python3.13/site-packages/package/cmake_runtime.py",
            "embedded/lib/runtime.c",
            "embedded/share/system-probe/ebpf/runtime/runtime-security.c",
            "etc/datadog-agent/datadog.yaml.example",
        ]
        for path in preserved_paths:
            self.create_file(path)
        self.create_file("embedded/include/python3.13/Python.h")

        cleanup_build_artifacts(self.install_dir)

        for path in preserved_paths:
            self.assertTrue((self.install_dir / path).is_file(), path)

    def test_handles_absent_optional_artifacts(self) -> None:
        self.site_packages.mkdir(parents=True)

        cleanup_build_artifacts(self.install_dir)
        cleanup_build_artifacts(self.install_dir)


if __name__ == "__main__":
    unittest.main()
