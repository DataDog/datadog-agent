import os
import tempfile
import unittest
from pathlib import Path

from tasks.kernel_matrix_testing.infra import find_ssh_key_files


class TestFindSSHKeyFiles(unittest.TestCase):
    def _write(self, d: Path, name: str, content: str = "") -> None:
        (d / name).write_text(content)

    def test_dotted_names_and_pem_private_keys(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            self._write(d, "id_ed25519.pub", "ssh-ed25519 AAAA user@host\n")
            self._write(d, "id_ed25519")
            self._write(d, "id_rsa_sandbox_first.last.pub", "ssh-rsa AAAA first.last\n")
            self._write(d, "id_rsa_sandbox_first.last.pem")
            self._write(d, "no_comment.pub", "ssh-rsa AAAA\n")

            keys = {k['name']: k['path'] for k in find_ssh_key_files(tmp)}

        self.assertEqual(keys["user@host"], os.path.join(tmp, "id_ed25519"))
        self.assertEqual(keys["first.last"], os.path.join(tmp, "id_rsa_sandbox_first.last.pem"))
        self.assertEqual(keys["no_comment"], os.path.join(tmp, "no_comment"))

    def test_pem_fallback_keeps_basename_as_key_name(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            self._write(d, "sandbox.pub", "ssh-rsa AAAA\n")
            self._write(d, "sandbox.pem")

            keys = {k['name']: k['path'] for k in find_ssh_key_files(tmp)}

        self.assertEqual(keys, {"sandbox": os.path.join(tmp, "sandbox.pem")})
