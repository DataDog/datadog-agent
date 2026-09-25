"""Unit tests for the ELF/Mach-O magic-number detection."""

import os
import sys
import tempfile
import unittest

import file_type

_SHARED_LIB_FIXTURE = os.path.abspath(sys.argv[1])


class RealFixtureTest(unittest.TestCase):
    def test_real_shared_library_is_detected(self):
        self.assertNotEqual(file_type.is_elf(_SHARED_LIB_FIXTURE), file_type.is_macho(_SHARED_LIB_FIXTURE))

    def test_symlink_to_real_fixture_is_followed(self):
        with tempfile.TemporaryDirectory() as d:
            link = os.path.join(d, "link")
            os.symlink(_SHARED_LIB_FIXTURE, link)
            self.assertEqual(file_type.is_elf(link), file_type.is_elf(_SHARED_LIB_FIXTURE))
            self.assertEqual(file_type.is_macho(link), file_type.is_macho(_SHARED_LIB_FIXTURE))


class NonBinaryFileTest(unittest.TestCase):
    def _write(self, dirpath: str, name: str, content: bytes) -> str:
        path = os.path.join(dirpath, name)
        with open(path, "wb") as f:
            f.write(content)
        return path

    def test_text_file_is_not_elf_or_macho(self):
        with tempfile.TemporaryDirectory() as d:
            txt = self._write(d, "notes.txt", b"hello world\n")
            self.assertFalse(file_type.is_elf(txt))
            self.assertFalse(file_type.is_macho(txt))

    def test_short_file_is_not_elf_or_macho(self):
        with tempfile.TemporaryDirectory() as d:
            short = self._write(d, "tiny", b"\x7f")
            self.assertFalse(file_type.is_elf(short))
            self.assertFalse(file_type.is_macho(short))

    def test_missing_file(self):
        self.assertFalse(file_type.is_elf("/nonexistent/path/to/nothing"))
        self.assertFalse(file_type.is_macho("/nonexistent/path/to/nothing"))


if __name__ == "__main__":
    unittest.main(argv=sys.argv[:1])
