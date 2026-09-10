"""Unit tests for the ELF magic-number detection."""

import os
import tempfile
import unittest

import file_type


class IsELFTest(unittest.TestCase):
    def _write(self, dirpath: str, name: str, content: bytes) -> str:
        path = os.path.join(dirpath, name)
        with open(path, "wb") as f:
            f.write(content)
        return path

    def test_elf_magic_is_detected(self):
        with tempfile.TemporaryDirectory() as d:
            elf = self._write(d, "fake-bin", b"\x7fELF" + b"\x02\x01\x01\x00" + b"\x00" * 32)
            self.assertTrue(file_type.is_elf(elf))

    def test_text_file_is_not_elf(self):
        with tempfile.TemporaryDirectory() as d:
            txt = self._write(d, "notes.txt", b"hello world\n")
            self.assertFalse(file_type.is_elf(txt))

    def test_short_file_is_not_elf(self):
        with tempfile.TemporaryDirectory() as d:
            short = self._write(d, "tiny", b"\x7f")
            self.assertFalse(file_type.is_elf(short))

    def test_symlink_to_elf_is_followed(self):
        with tempfile.TemporaryDirectory() as d:
            target = self._write(d, "fake-bin", b"\x7fELF\x00\x00\x00\x00")
            link = os.path.join(d, "link")
            os.symlink(target, link)
            self.assertTrue(file_type.is_elf(link))

    def test_missing_file(self):
        self.assertFalse(file_type.is_elf("/nonexistent/path/to/nothing"))


class IsMachOTest(unittest.TestCase):
    def _write(self, dirpath: str, name: str, content: bytes) -> str:
        path = os.path.join(dirpath, name)
        with open(path, "wb") as f:
            f.write(content)
        return path

    def test_thin_64bit_le_is_detected(self):
        with tempfile.TemporaryDirectory() as d:
            f = self._write(d, "lib.dylib", b"\xcf\xfa\xed\xfe" + b"\x00" * 32)
            self.assertTrue(file_type.is_macho(f))

    def test_thin_64bit_be_is_detected(self):
        with tempfile.TemporaryDirectory() as d:
            f = self._write(d, "lib.dylib", b"\xfe\xed\xfa\xcf" + b"\x00" * 32)
            self.assertTrue(file_type.is_macho(f))

    def test_fat_magic_is_detected(self):
        with tempfile.TemporaryDirectory() as d:
            f = self._write(d, "universal", b"\xca\xfe\xba\xbe" + b"\x00" * 32)
            self.assertTrue(file_type.is_macho(f))

    def test_elf_is_not_macho(self):
        with tempfile.TemporaryDirectory() as d:
            f = self._write(d, "fake-bin", b"\x7fELF" + b"\x00" * 32)
            self.assertFalse(file_type.is_macho(f))

    def test_text_is_not_macho(self):
        with tempfile.TemporaryDirectory() as d:
            f = self._write(d, "notes.txt", b"hello\n")
            self.assertFalse(file_type.is_macho(f))


if __name__ == "__main__":
    unittest.main()
