import shutil
import tempfile
import unittest
from pathlib import Path

from tasks.debugging.symbols import SymbolStore


class TestSymbolStore(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix='test-store-'))
        self.symbol_store = SymbolStore(Path(self.root, 'symbols'))

    def tearDown(self):
        shutil.rmtree(self.root)

    def test_add_symbols(self):
        """
        Test that we can add symbols to the store
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            # pretend symbols in directory
            Path(tmpdir, 'agent.dbg').write_text('fake symbols')
            # add to store
            self.symbol_store.add('1.0', 'linux', 'x86_64', tmpdir)
        # symbols organized by version/platform/arch
        symbol_path = Path(self.root, 'symbols', '1.0', 'linux', 'x86_64', 'symbols', 'agent.dbg')
        self.assertTrue(symbol_path.exists())
        self.assertEqual('fake symbols', symbol_path.read_text())
        # get symbols from store
        symbols = self.symbol_store.get('1.0', 'linux', 'x86_64')
        assert symbols is not None
        self.assertTrue(symbols.exists())
        self.assertTrue(Path(symbols, 'agent.dbg').exists())
