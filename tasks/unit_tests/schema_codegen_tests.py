import filecmp
import os
import shutil
import tempfile
import unittest

import yaml

import tasks.schema.codegen_init_settings as codegen
from tasks.schema.codegen_init_settings import as_go_value

TESTDATA = os.path.join(os.path.dirname(__file__), "testdata", "schema_codegen")


def fixture(name):
    return os.path.join(TESTDATA, name)


def filter_not_sysprobe(filename):
    return filename != 'system_probe_settings.go'


class TestCodegenInitSettings(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.tmpdir)

    def validate_generated_code(self, golden_file):
        ents = os.listdir(self.tmpdir)
        self.assertEqual(len(ents), 1, 'expected only 1 generated source file')
        actual_file = os.path.join(self.tmpdir, ents[0])
        self.assertTrue(filecmp.cmp(actual_file, golden_file))

    def test_basic_codegen(self):
        with open(fixture('basic_schema.yaml')) as f:
            schema = yaml.safe_load(f)
        codegen.run_codegen(schema, filter_not_sysprobe, None, False, self.tmpdir)
        self.validate_generated_code(fixture('basic_settings.gen'))

    def test_codegen_full_agent_setting(self):
        with open(fixture('basic_full_agent_schema.yaml')) as f:
            schema = yaml.safe_load(f)
        codegen.run_codegen(schema, filter_not_sysprobe, None, False, self.tmpdir)
        self.validate_generated_code(fixture('basic_full_agent_settings.gen'))

    def test_as_go_value(self):
        cases = [
            {
                "describe": "list conversion",
                "split_lines": False,
                "input":  "['a', 'b', 'c']",
                "expect": "{\"a\", \"b\", \"c\"}"
            },
            {
                "describe": "quoted string",
                "split_lines": False,
                "input":  "['datadoghq.com']",
                "expect": "{\"datadoghq.com\"}",
            },
            {
                "describe": "map with bool values",
                "split_lines": False,
                "input": "{'linux': True, 'other': False}",
                "expect": "{\"linux\": true, \"other\": false}",
            },
            {
                "describe": "string containing comma",
                "split_lines": False,
                "input":  "['a,b']",
                "expect": "{\"a,b\"}"
            },
            {
                "describe": "map conversion",
                "split_lines": False,
                "input":  "{'a': 'apple', 'b': 'banana'}",
                "expect": "{\"a\": \"apple\", \"b\": \"banana\"}"
            },
            {
                "describe": "regex",
                "split_lines": False,
                "input": "['^ad\\.datadoghq\\.com\\/([[:alnum:]]+\\.)$']",
                "expect": "{`^ad\\.datadoghq\\.com\\/([[:alnum:]]+\\.)$`}",
            },
            {
                "describe": "map with split lines",
                "split_lines": True,
                "input":  "{'a': 'apple', 'b': 'banana'}",
                "expect": "{\n\"a\": \"apple\",\n \"b\": \"banana\",\n}"
            },
        ]
        for c in cases:
            with self.subTest(service=c['describe']):
                actual = as_go_value(c['input'], split_lines=c['split_lines'])
                self.assertEqual(c['expect'], actual, c['describe'])


if __name__ == "__main__":
    unittest.main()
