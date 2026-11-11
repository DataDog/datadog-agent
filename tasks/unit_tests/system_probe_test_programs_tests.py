import io
import unittest
from unittest.mock import MagicMock, patch

from invoke.exceptions import Exit

from tasks.libs.build.ninja import NinjaWriter
from tasks.system_probe import ninja_add_dyninst_test_programs


class TestDyninstTestProgramFilters(unittest.TestCase):
    def generate(self, existing=False, **filters):
        ctx = MagicMock()
        prefix = 'github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/'
        ctx.run.return_value.return_code = 0
        ctx.run.return_value.stdout = f'{prefix}simple main: fmt\n{prefix}sample main: fmt\n'
        output = io.StringIO()
        with patch('tasks.system_probe.glob.glob', return_value=['/tmp/unused-fixture'] if existing else []):
            ninja_add_dyninst_test_programs(ctx, NinjaWriter(output), '.', 'go', **filters)
        return output.getvalue()

    def test_defaults_preserved(self):
        output = self.generate()
        for toolchain in ['go1.23.11', 'go1.24.3', 'go1.25.0', 'go1.26rc1', 'go1.27.3']:
            self.assertIn(f'GOTOOLCHAIN={toolchain}', output)
        self.assertEqual(output.count('build '), 20)

    def test_filtered_matrix(self):
        output = self.generate(toolchains='go1.27.2,go1.27.3', programs='simple')
        self.assertIn('GOTOOLCHAIN=go1.27.2', output)
        self.assertIn('GOTOOLCHAIN=go1.27.3', output)
        self.assertIn('GOARCH=amd64', output)
        self.assertIn('GOARCH=arm64', output)
        self.assertNotIn('/sample', output)
        self.assertEqual(output.count('build '), 4)

    def test_unknown_program(self):
        with self.assertRaises(Exit) as error:
            self.generate(programs='unknown')
        self.assertIn('Unknown dyninst test programs', error.exception.message)

    @patch('tasks.system_probe.os.path.isfile', return_value=True)
    @patch('tasks.system_probe.os.remove')
    def test_filtered_build_preserves_other_fixtures(self, remove, _isfile):
        self.generate(existing=True, programs='simple')
        remove.assert_not_called()
