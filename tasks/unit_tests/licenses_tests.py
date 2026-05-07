import unittest

from tasks.licenses import is_valid_quote


class TestLicensesMethod(unittest.TestCase):
    def test_valid_quotes(self):
        self.assertTrue(is_valid_quote('"\'hello\'"'))

    def test_invalid_quotes(self):
        self.assertFalse(is_valid_quote('""hello' '"""'))
