import os
import tempfile
import textwrap
import unittest

from invoke.exceptions import Exit

import tasks.schema.validation as validation


def _write(path, content):
    with open(path, "w") as f:
        f.write(textwrap.dedent(content))


class TestParseParams(unittest.TestCase):
    def test_range_single(self):
        self.assertEqual(validation.parse_params(">= 2", "number"), {"minimum": 2.0})
        self.assertEqual(validation.parse_params("> 2", "integer"), {"exclusiveMinimum": 2})

    def test_range_double(self):
        self.assertEqual(
            validation.parse_params("> 0 and <= 10000", "integer"),
            {"exclusiveMinimum": 0, "maximum": 10000},
        )
        # Lower bound is emitted first, whatever the expression order.
        self.assertEqual(
            validation.parse_params("< 100 AND >= 10", "integer"),
            {"minimum": 10, "exclusiveMaximum": 100},
        )

    def test_range_fractional_bound_on_integer(self):
        self.assertEqual(validation.parse_params("> 0.5", "integer"), {"exclusiveMinimum": 0.5})

    def test_range_negative(self):
        self.assertEqual(validation.parse_params(">= -1", "integer"), {"minimum": -1})

    def test_range_errors(self):
        for params, setting_type in [
            (">= 2", "string"),
            (">= 2 and > 3", "integer"),
            ("< 2 and <= 3", "integer"),
            ("> 5 and < 5", "integer"),
            (">= 10 and <= 1", "integer"),
            (">= abc", "integer"),
            ("=> 2", "integer"),
        ]:
            with self.subTest(params=params), self.assertRaises(Exit):
                validation.parse_params(params, setting_type)

    def test_enum(self):
        self.assertEqual(
            validation.parse_params("disabled, dry_run, enabled", "string"),
            {"enum": ["disabled", "dry_run", "enabled"]},
        )
        self.assertEqual(validation.parse_params("1,2,3", "integer"), {"enum": [1, 2, 3]})

    def test_enum_errors(self):
        for params, setting_type in [
            ("one, two-three", "string"),
            ("one,,two", "string"),
            ("one, one", "string"),
            ("one, two", "integer"),
        ]:
            with self.subTest(params=params), self.assertRaises(Exit):
                validation.parse_params(params, setting_type)

    def test_json(self):
        self.assertEqual(
            validation.parse_params('{"minimum": 10, "exclusiveMaximum": 100}', "integer"),
            {"minimum": 10, "exclusiveMaximum": 100},
        )

    def test_json_errors(self):
        for params in ["{not json", "{}"]:
            with self.subTest(params=params), self.assertRaises(Exit):
                validation.parse_params(params, "integer")

    def test_non_empty(self):
        self.assertEqual(validation.parse_params("non-empty", "string"), {"minLength": 1})
        with self.assertRaises(Exit):
            validation.parse_params("non-empty", "integer")


class TestAddValidation(unittest.TestCase):
    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name
        _write(
            self._path("core.yaml"),
            """\
            properties:
              backoff:
                node_type: setting
                type: number
                default: 2
                tags:
                  - golang_type:float64
              mode:
                node_type: setting
                type: string
                default: on
                description: |-
                  The mode.

                  More text.

              name:
                node_type: setting
                type: string
                default: ''
              proxy:
                node_type: section
                type: object
                properties:
                  port:
                    node_type: setting
                    type: integer
                    default: 0
                    tags:
                    - foo
              apm_config:
                $ref: apm_config.yaml
            """,
        )
        _write(
            self._path("apm_config.yaml"),
            """\
            node_type: section
            type: object
            properties:
              max:
                node_type: setting
                type: integer
                default: 10
                minimum: 0
            """,
        )
        _write(
            self._path("sysprobe.yaml"),
            """\
            properties:
              name:
                node_type: setting
                type: string
                default: ''
            """,
        )
        self.schemas = [("core", self._path("core.yaml"))]

    def tearDown(self):
        self._tempdir.cleanup()

    def _path(self, name):
        return os.path.join(self.dir, name)

    def _read(self, name):
        with open(self._path(name)) as f:
            return f.read()

    def test_after_indented_sequence(self):
        validation.add_validation_to_schema("backoff", ">= 2", self.schemas)
        self.assertIn(
            "      - golang_type:float64\n    minimum: 2.0\n  mode:\n",
            self._read("core.yaml"),
        )

    def test_after_block_scalar_keeps_blank_line(self):
        validation.add_validation_to_schema("mode", "on, off", self.schemas)
        self.assertIn(
            "      More text.\n    enum:\n    - 'on'\n    - 'off'\n\n  name:\n",
            self._read("core.yaml"),
        )

    def test_nested_after_unindented_sequence(self):
        validation.add_validation_to_schema("proxy.port", "> 0 and < 65536", self.schemas)
        self.assertIn(
            "        - foo\n        exclusiveMinimum: 0\n        exclusiveMaximum: 65536\n  apm_config:\n",
            self._read("core.yaml"),
        )

    def test_follows_ref(self):
        validation.add_validation_to_schema("apm_config.max", "<= 100", self.schemas)
        self.assertTrue(self._read("apm_config.yaml").endswith("    minimum: 0\n    maximum: 100\n"))

    def test_last_setting_in_file(self):
        validation.add_validation_to_schema("name", "non-empty", [("sp", self._path("sysprobe.yaml"))])
        self.assertTrue(self._read("sysprobe.yaml").endswith("    default: ''\n    minLength: 1\n"))

    def test_rejects_existing_field(self):
        with self.assertRaises(Exit):
            validation.add_validation_to_schema("apm_config.max", ">= 1", self.schemas)

    def test_rejects_section(self):
        with self.assertRaises(Exit):
            validation.add_validation_to_schema("proxy", "non-empty", self.schemas)
        with self.assertRaises(Exit):
            validation.add_validation_to_schema("apm_config", "non-empty", self.schemas)

    def test_rejects_missing(self):
        with self.assertRaises(Exit):
            validation.add_validation_to_schema("nope", "non-empty", self.schemas)

    def test_rejects_ambiguous(self):
        schemas = [*self.schemas, ("sp", self._path("sysprobe.yaml"))]
        with self.assertRaises(Exit):
            validation.add_validation_to_schema("name", "non-empty", schemas)
