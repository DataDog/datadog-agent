import base64
import contextlib
import io
import os
import tempfile
import unittest
import zipfile

from invoke import Context

from tasks.winbuild import (
    SYMBOL_STORE_DIR_NAME,
    _extract_pdbs,
    _format_index_key,
    _symbol_index_key,
    generate_symbol_store,
)

# The symbol-server key agent.exe.pdb must map to, cross-checked against
# agent.exe's PE RSDS debug-directory record.
SAMPLE_KEY = "28431E7AF1452C82FFD31B5B0E3722C91"
# GUID bytes (first three fields little-endian, as stored in the PDB) for the
# _format_index_key transform test.
SAMPLE_GUID = bytes.fromhex("7A1E432845F1822CFFD31B5B0E3722C9")

# Real byte ranges from a production agent.exe.pdb (mingw `ld --pdb` output),
# covering exactly the regions _symbol_index_key reads -- the MSF superblock,
# the stream-directory block map, the directory prefix (stream count, sizes[0:2]
# and stream 1's block list), and the PDB Info stream head (version, signature,
# age, GUID). Everything else is zero-filled. Using real bytes rather than a
# hand-built MSF keeps the test from baking in the same format assumptions as
# the parser, so a shared mistake can't pass silently.
_REAL_PDB_SIZE = 9216
_REAL_PDB_RANGES = [
    (32, "AAQAAAEAAADXUwAAsFUBAAAAAAADAAAA"),
    (3072, "BAAAAAUAAAAGAAAA"),
    (4096, "FgIAAAQAAABLAAAA"),
    (6236, "BwAAAAgAAAA="),
    (8192, "lC4xAbe9GWoBAAAAeh5DKEXxgiz/0xtbDjciyQ=="),
]


def _real_agent_pdb() -> bytes:
    """Reconstruct the sparse agent.exe.pdb fixture from its real byte ranges."""
    buf = bytearray(_REAL_PDB_SIZE)
    for off, b64 in _REAL_PDB_RANGES:
        raw = base64.b64decode(b64)
        buf[off : off + len(raw)] = raw
    return bytes(buf)


class TestFormatIndexKey(unittest.TestCase):
    def test_known_guid(self):
        self.assertEqual(_format_index_key(SAMPLE_GUID, 1), SAMPLE_KEY)

    def test_age_is_hex_uppercase_no_padding(self):
        self.assertTrue(_format_index_key(SAMPLE_GUID, 26).endswith("1A"))


class TestSymbolIndexKey(unittest.TestCase):
    def test_parses_real_pdb(self):
        with tempfile.TemporaryDirectory() as tmp:
            pdb = os.path.join(tmp, "agent.exe.pdb")
            with open(pdb, "wb") as f:
                f.write(_real_agent_pdb())
            self.assertEqual(_symbol_index_key(pdb), SAMPLE_KEY)


class TestExtractPdbs(unittest.TestCase):
    def _make_debug_zip(self, path, entries):
        with zipfile.ZipFile(path, "w") as z:
            for name, data in entries.items():
                z.writestr(name, data)

    def test_extracts_only_pdbs(self):
        with tempfile.TemporaryDirectory() as tmp:
            zip_path = os.path.join(tmp, "datadog-agent-7.x.x-x86_64.debug.zip")
            # .debug.zip carries both stripped-binary .debug copies and the .pdb
            # companions; only the latter are wanted.
            self._make_debug_zip(
                zip_path,
                {
                    "opt/datadog-agent/bin/agent/agent.exe.debug": "stripped binary",
                    "opt/datadog-agent/bin/agent/agent.exe.pdb": "agent pdb",
                    "opt/datadog-agent/embedded/foo.dll.debug": "stripped binary",
                },
            )
            dest = os.path.join(tmp, "out")
            pdbs = _extract_pdbs([zip_path], dest)

        self.assertEqual(len(pdbs), 1)
        self.assertTrue(pdbs[0].lower().endswith("agent.exe.pdb"))

    def test_flattens_and_separates_archives(self):
        with tempfile.TemporaryDirectory() as tmp:
            zip_a = os.path.join(tmp, "datadog-agent-7.x.x-x86_64.debug.zip")
            zip_b = os.path.join(tmp, "datadog-installer-7.x.x-1-x86_64.debug.zip")
            # Same PDB basename in two archives must not clobber each other:
            # each archive extracts into its own subdirectory.
            self._make_debug_zip(zip_a, {"opt/datadog-agent/bin/agent/agent.exe.pdb": "a"})
            self._make_debug_zip(zip_b, {"opt/datadog-installer/agent.exe.pdb": "b"})
            dest = os.path.join(tmp, "out")
            pdbs = _extract_pdbs([zip_a, zip_b], dest)

        self.assertEqual(len(pdbs), 2)
        self.assertEqual(len(set(pdbs)), 2)
        for p in pdbs:
            self.assertEqual(os.path.basename(p), "agent.exe.pdb")


class TestGenerateSymbolStore(unittest.TestCase):
    def test_lays_out_two_tier_store(self):
        with tempfile.TemporaryDirectory() as tmp:
            zip_path = os.path.join(tmp, "datadog-agent-7.x.x-x86_64.debug.zip")
            with zipfile.ZipFile(zip_path, "w") as z:
                z.writestr("opt/datadog-agent/bin/agent/agent.exe.pdb", _real_agent_pdb())
                z.writestr("opt/datadog-agent/bin/agent/agent.exe.debug", "stripped binary")

            with contextlib.redirect_stdout(io.StringIO()):
                generate_symbol_store(Context(), output_dir=tmp)

            expected = os.path.join(tmp, SYMBOL_STORE_DIR_NAME, "agent.exe.pdb", SAMPLE_KEY, "agent.exe.pdb")
            self.assertTrue(os.path.isfile(expected), f"missing symbol-store entry: {expected}")

    def test_no_debug_zip_is_noop(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                generate_symbol_store(Context(), output_dir=tmp)
            self.assertFalse(os.path.exists(os.path.join(tmp, SYMBOL_STORE_DIR_NAME)))
            self.assertIn("no .debug.zip found", out.getvalue())


if __name__ == "__main__":
    unittest.main()
