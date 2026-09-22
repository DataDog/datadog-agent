import os
import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestOCIPackaging(unittest.TestCase):
    def test_merge_passes_base_archives_before_fips_variants(self):
        config = yaml.load((REPO_ROOT / '.gitlab/build/packaging/oci.yml').read_text(), Loader=yaml.BaseLoader)
        merge_script = next(script for script in config['.package_oci']['script'] if 'datadog-package merge' in script)
        cases = {
            'windows': [
                'datadog-agent-7.85.0-1-windows-amd64.oci.tar',
                'datadog-agent-7.85.0-1-windows-amd64-fips.oci.tar',
            ],
            'linux_and_windows': [
                'datadog-agent-7.85.0-1-amd64.tar',
                'datadog-agent-7.85.0-1-arm64.tar',
                'datadog-agent-7.85.0-1-windows-amd64.oci.tar',
                'datadog-agent-7.85.0-1-windows-amd64-fips.oci.tar',
                'datadog-fips-agent-7.85.0-1-amd64.tar',
                'datadog-fips-agent-7.85.0-1-arm64.tar',
            ],
            'installer': [
                'datadog-installer-7.85.0-1-amd64.tar',
                'datadog-installer-7.85.0-1-arm64.tar',
                'datadog-fips-installer-7.85.0-1-amd64.tar',
                'datadog-fips-installer-7.85.0-1-arm64.tar',
            ],
            'without_fips': [
                'datadog-agent-ddot-7.85.0-1-amd64.tar',
                'datadog-agent-ddot-7.85.0-1-arm64.tar',
                'datadog-agent-ddot-7.85.0-1-windows-amd64.oci.tar',
            ],
        }
        for name, expected in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                # Argument order is significant: merge preserves it in the OCI index.
                for archive in expected:
                    (Path(directory) / archive).touch()
                result = subprocess.run(
                    ['bash', '-eo', 'pipefail', '-c', 'datadog-package() { printf "%s\\n" "$@"; }\n' + merge_script],
                    env={**os.environ, 'OUTPUT_DIR': directory},
                    text=True,
                    capture_output=True,
                    check=True,
                )
                self.assertEqual(result.stdout.splitlines(), ['merge', *(str(Path(directory) / p) for p in expected)])
