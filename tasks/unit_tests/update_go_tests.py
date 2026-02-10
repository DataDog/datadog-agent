import os
import shutil
import tempfile
import unittest

from pathlib import Path
from invoke.exceptions import Exit
from unittest.mock import patch
from tasks.libs.common.gomodules import GoModule
from tasks.update_go import (
    PATTERN_MAJOR_MINOR,
    PATTERN_MAJOR_MINOR_BUGFIX,
    _get_major_minor_version,
    _get_pattern,
    _update_go_mods,
)


class TestUpdateGo(unittest.TestCase):
    def test_get_minor_version(self):
        self.assertEqual(_get_major_minor_version("1.2.3"), "1.2")

    def test_get_pattern(self):
        self.assertEqual(_get_pattern("p+e", "p.st", is_bugfix=True), rf'(p\+e){PATTERN_MAJOR_MINOR_BUGFIX}(p\.st)')
        self.assertEqual(_get_pattern("p(re)", "p*st", is_bugfix=False), rf'(p\(re\)){PATTERN_MAJOR_MINOR}(p\*st)')


class TestUpdateGoMods(unittest.TestCase):
    def setUp(self):
        self._tmpdir = tempfile.mkdtemp()
        self._oldpwd = os.getcwd()
        os.chdir(self._tmpdir)

    def tearDown(self):
        os.chdir(self._oldpwd)
        shutil.rmtree(self._tmpdir)

    @patch(
        "tasks.update_go.get_default_modules",
        return_value={
            ".": GoModule(path="."),
            "comp/core/test": GoModule(path="comp/core/test"),
        },
    )
    def test_update_toolchain_with_real_files(self, mock_get_modules):
        root_gomod = Path("go.mod")
        root_gomod.write_text("""
module github.com/DataDog/datadog-agent

go 1.25.6
toolchain go1.25.6
""")
        child_gomod = Path("comp/core/test/go.mod")
        child_gomod.parent.mkdir(parents=True)
        child_gomod.write_text("""
module github.com/DataDog/datadog-agent/comp/core/test

go 1.25.6
""")

        _update_go_mods(warn=False, version="1.25.7", include_otel_modules=False)

        self.assertEqual(
            root_gomod.read_text(),
            """
module github.com/DataDog/datadog-agent

go 1.25.0
toolchain go1.25.7
""",
        )
        self.assertEqual(
            child_gomod.read_text(),
            """
module github.com/DataDog/datadog-agent/comp/core/test

go 1.25.0
""",
        )

    @patch(
        "tasks.update_go.get_default_modules",
        return_value={
            ".": GoModule(path="."),
        },
    )
    def test_update_toolchain_fails_when_invalid(self, mock_get_modules):
        for case, (expected_error, content) in {
            "missing toolchain": (
                "expected 1 matches but got 0",
                """
module github.com/DataDog/datadog-agent

go 1.25.6
""",
            ),
            "duplicate toolchain": (
                "expected 1 matches but got 2",
                """
module github.com/DataDog/datadog-agent

go 1.25.6
toolchain go1.25.6
toolchain go1.25.5
""",
            ),
        }.items():
            with self.subTest(case):
                Path("go.mod").write_text(content)
                with self.assertRaisesRegex(Exit, expected_error):
                    _update_go_mods(warn=False, version="1.25.7", include_otel_modules=False)
