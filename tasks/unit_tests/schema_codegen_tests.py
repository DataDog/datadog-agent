import filecmp
import os
import re
import shutil
import tempfile
import unittest

import yaml

import tasks.schema.codegen_init_settings as codegen
from tasks.schema.codegen_init_settings import as_go_value, override_stubs, try_parse_duration

TESTDATA = os.path.join(os.path.dirname(__file__), "testdata", "schema_codegen")


def fixture(name):
    return os.path.join(TESTDATA, name)


def filter_not_sysprobe(filename):
    return filename != 'system_probe_settings.go'


class TestCodegenInitSettings(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        override_stubs('', '')

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
                "input": "['a', 'b', 'c']",
                "expect": "{\"a\", \"b\", \"c\"}",
            },
            {
                "describe": "quoted string",
                "split_lines": False,
                "input": "['datadoghq.com']",
                "expect": "{\"datadoghq.com\"}",
            },
            {
                "describe": "map with bool values",
                "split_lines": False,
                "input": "{'linux': True, 'other': False}",
                "expect": "{\"linux\": true, \"other\": false}",
            },
            {"describe": "string containing comma", "split_lines": False, "input": "['a,b']", "expect": "{\"a,b\"}"},
            {
                "describe": "map conversion",
                "split_lines": False,
                "input": "{'a': 'apple', 'b': 'banana'}",
                "expect": "{\"a\": \"apple\", \"b\": \"banana\"}",
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
                "input": "{'a': 'apple', 'b': 'banana'}",
                "expect": "{\n\t\t\"a\": \"apple\",\n\t\t\"b\": \"banana\",\n\t}",
            },
        ]
        for c in cases:
            with self.subTest(service=c['describe']):
                actual = as_go_value(c['input'], split_lines=c['split_lines'])
                self.assertEqual(c['expect'], actual, c['describe'])

    def test_try_parse_duration(self):
        cases = [
            {
                "describe": "10 seconds",
                "input": "10s",
                "expect": "10*time.Second",
            },
            {
                "describe": "4 minutes, 30 seconds",
                "input": "4m30s",
                "expect": "4*time.Minute + 30*time.Second",
            },
            {
                "describe": "100 microseconds",
                "input": "100µs",
                "expect": "100*time.Microsecond",
            },
            {
                "describe": "4 nanoseconds",
                "input": "4ns",
                "expect": "4*time.Nanosecond",
            },
            {
                "describe": "-1 nanoseconds",
                "input": "-1ns",
                "expect": "-1*time.Nanosecond",
            },
            {
                "describe": "-2 hours and 400 milliseconds",
                "input": "-2h400ms",
                "expect": "-(2*time.Hour + 400*time.Millisecond)",
            },
        ]
        for c in cases:
            with self.subTest(service=c['describe']):
                actual = try_parse_duration(c['input'])
                self.assertEqual(c['expect'], actual, c['describe'])


class TestGenerateConst(unittest.TestCase):
    @staticmethod
    def _setting(default, const, type_='string'):
        return {'node_type': 'setting', 'type': type_, 'default': default, 'tags': [f'generate_const:{const}']}

    def test_dedup_agreeing_refs_across_schemas(self):
        # DefaultSite is referenced by two core settings and one system-probe setting, all agreeing.
        core = {
            'properties': {
                'site': self._setting('datadoghq.com', 'DefaultSite'),
                'security_agent': {
                    'node_type': 'section',
                    'properties': {
                        'internal_profiling': {
                            'node_type': 'section',
                            'properties': {'site': self._setting('datadoghq.com', 'DefaultSite')},
                        },
                        'cmd_port': self._setting(5010, 'DefaultSecurityAgentCmdPort', 'integer'),
                    },
                },
            }
        }
        sysprobe = {
            'properties': {
                'system_probe_config': {
                    'node_type': 'section',
                    'properties': {'internal_profiling': {'node_type': 'section', 'properties': {}}},
                }
            }
        }
        core_out, sysprobe_out = [], []
        codegen.gen_generate_const(core, sysprobe, core_out, sysprobe_out)

        src = '\n'.join(core_out)
        # Remove white space added by codegen's formatter
        contents = re.sub(' +', ' ', src)
        # DefaultSite is emitted exactly once despite three references, and the block is valid Go.
        self.assertEqual(contents.count('DefaultSite ='), 1)
        self.assertIn('DefaultSecurityAgentCmdPort = 5010', contents)
        self.assertIn('DefaultSite = "datadoghq.com"', contents)
        self.assertEqual(sysprobe_out, [])

    def test_conflicting_defaults_raise(self):
        # Same constant tagged on two settings with different defaults must fail codegen.
        core = {
            'properties': {
                'a': self._setting(23, 'DefaultAuditorTTL', 'integer'),
                'b': self._setting(22, 'DefaultAuditorTTL', 'integer'),
            }
        }
        with self.assertRaises(RuntimeError) as ctx:
            codegen.gen_generate_const(core, {'properties': {}}, [], [])
        self.assertIn('DefaultAuditorTTL', str(ctx.exception))

    def test_no_tags_emits_nothing(self):
        core = {'properties': {'a': {'node_type': 'setting', 'type': 'string', 'default': 'x'}}}
        core_out = []
        codegen.gen_generate_const(core, {'properties': {}}, core_out, [])
        self.assertEqual(core_out, [])


if __name__ == "__main__":
    unittest.main()
