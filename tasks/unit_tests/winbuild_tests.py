import os
import tempfile
import unittest
import zipfile

from tasks.winbuild import _extract_pdbs, _strip_symstore_metadata


class TestExtractPdbs(unittest.TestCase):
    def _make_debug_zip(self, path, entries):
        with zipfile.ZipFile(path, "w") as z:
            for name, data in entries.items():
                z.writestr(name, data)

    def test_extracts_only_pdbs(self):
        with tempfile.TemporaryDirectory() as tmp:
            zip_path = os.path.join(tmp, "datadog-agent-7.x.x-x86_64.debug.zip")
            # .debug.zip mirrors install_dir and carries both stripped-binary
            # .debug copies and the .pdb companions; only the latter are wanted.
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


class TestStripSymstoreMetadata(unittest.TestCase):
    def test_removes_metadata_keeps_pdbs(self):
        with tempfile.TemporaryDirectory() as store:
            # A symstore tree: one indexed PDB plus transaction bookkeeping.
            pdb_dir = os.path.join(store, "agent.exe.pdb", "ABCDEF1234567890ABCDEF12345678901")
            os.makedirs(pdb_dir)
            pdb_file = os.path.join(pdb_dir, "agent.exe.pdb")
            with open(pdb_file, "w") as f:
                f.write("symbols")
            with open(os.path.join(pdb_dir, "refs.ptr"), "w") as f:
                f.write("ref")

            os.makedirs(os.path.join(store, "000Admin"))
            with open(os.path.join(store, "000Admin", "history.txt"), "w") as f:
                f.write("history")
            for name in ("pingme.txt", "server.txt", "lastid.txt"):
                with open(os.path.join(store, name), "w") as f:
                    f.write("x")

            _strip_symstore_metadata(store)

            self.assertTrue(os.path.exists(pdb_file))
            self.assertFalse(os.path.exists(os.path.join(store, "000Admin")))
            self.assertFalse(os.path.exists(os.path.join(pdb_dir, "refs.ptr")))
            for name in ("pingme.txt", "server.txt", "lastid.txt"):
                self.assertFalse(os.path.exists(os.path.join(store, name)))


if __name__ == "__main__":
    unittest.main()
