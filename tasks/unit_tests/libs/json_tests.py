import unittest

from tasks.libs.json import JSONWithCommentsDecoder


class TestJSONWithCommentsDecoder(unittest.TestCase):
    def setUp(self):
        self.decoder = JSONWithCommentsDecoder()

    def test_decode(self):
        self.assertEqual(
            self.decoder.decode('''
                {
                    "key1": "value1",
                    "key2": "value2"
                }
                '''),
            {"key1": "value1", "key2": "value2"},
        )

    def test_decode_with_comments(self):
        self.assertEqual(
            self.decoder.decode('''
                {
                    "key1": "value1",
                    // This is a comment
                    "key2": "value2"
                }
            '''),
            {"key1": "value1", "key2": "value2"},
        )
