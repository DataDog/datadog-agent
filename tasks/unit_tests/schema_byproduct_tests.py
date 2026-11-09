import unittest


class TestJsonSchemaOutput(unittest.TestCase):
    def test_keeps_only_standard_keywords(self):
        from tasks.schema.produce_byproduct import json_schema

        doc = {
            "type": "object",
            "node_type": "section",
            "visibility": "public",
            "properties": {
                "enabled": {
                    "type": "boolean",
                    "default": False,
                    "env_vars": ["DD_ENABLED"],
                },
            },
        }
        self.assertEqual(
            json_schema(doc),
            {
                "type": "object",
                "properties": {
                    "enabled": {
                        "type": "boolean",
                        "default": False,
                    },
                },
            },
        )

    def test_object_default_is_preserved_verbatim(self):
        """Regression: instance data under ``default`` must not be walked as a
        schema subtree, or its non-keyword keys are dropped to ``{}``."""
        from tasks.schema.produce_byproduct import json_schema

        doc = {
            "type": "object",
            "properties": {
                "kubernetes_node_annotations_as_tags": {
                    "type": "object",
                    "default": {"cluster.k8s.io/machine": "kube_machine"},
                    "additionalProperties": {"type": "string"},
                },
            },
        }
        result = json_schema(doc)
        self.assertEqual(
            result["properties"]["kubernetes_node_annotations_as_tags"]["default"],
            {"cluster.k8s.io/machine": "kube_machine"},
        )

    def test_enum_const_examples_instance_data_preserved(self):
        from tasks.schema.produce_byproduct import json_schema

        doc = {
            "enum": [{"description": "not a keyword here"}, "plain"],
            "const": {"properties": "instance value"},
            "examples": [{"cluster.k8s.io/machine": "machine"}],
        }
        self.assertEqual(json_schema(doc), doc)


class TestJsonSchemaDuration(unittest.TestCase):
    def test_duration_format_becomes_pattern(self):
        from tasks.schema.produce_byproduct import GO_DURATION_PATTERN, json_schema

        doc = {
            "type": "object",
            "properties": {
                "timeout": {"node_type": "setting", "type": "string", "default": "15s", "format": "duration"},
                "intervals": {"type": "array", "items": {"type": "string", "format": "duration"}},
                "name": {"type": "string", "format": "hostname"},
            },
        }
        props = json_schema(doc)["properties"]
        self.assertEqual(props["timeout"], {"type": "string", "default": "15s", "pattern": GO_DURATION_PATTERN})
        self.assertEqual(props["intervals"]["items"], {"type": "string", "pattern": GO_DURATION_PATTERN})
        self.assertEqual(props["name"], {"type": "string", "format": "hostname"})

    def test_existing_pattern_is_kept(self):
        from tasks.schema.produce_byproduct import GO_DURATION_PATTERN, json_schema

        doc = {"type": "string", "format": "duration", "pattern": "^[0-9]+s$"}
        self.assertEqual(
            json_schema(doc),
            {"type": "string", "pattern": "^[0-9]+s$", "allOf": [{"pattern": GO_DURATION_PATTERN}]},
        )

    def test_instance_data_is_not_rewritten(self):
        from tasks.schema.produce_byproduct import json_schema

        doc = {"type": "object", "default": {"format": "duration"}, "enum": [{"format": "duration"}]}
        self.assertEqual(json_schema(doc), doc)

    def test_embedded_keeps_duration_format(self):
        from tasks.schema.produce_byproduct import embedded

        doc = {"type": "string", "default": "15s", "format": "duration"}
        self.assertEqual(embedded(doc), doc)

    def test_pattern_matches_go_parse_duration(self):
        """Expected results are what Go's time.ParseDuration returns for each value."""
        import re

        from tasks.schema.produce_byproduct import GO_DURATION_PATTERN

        pattern = re.compile(GO_DURATION_PATTERN)
        valid = [
            "0",
            "-0",
            "15s",
            "1m0s",
            "2h30m21s",
            "1s30m",
            "1h1h",
            "1.5h",
            ".5s",
            "5.s",
            "-5m",
            "+5m",
            "500ms",
            "10us",
            "10µs",  # MICRO SIGN
            "10μs",  # GREEK SMALL LETTER MU
            "100ns",
        ]
        invalid = ["", "30", "2h30min21s", "1d", "1 s", " 1s", "1s ", "PT15S", ".s", "s", "-", "1S", "1.2.3s"]
        for value in valid:
            self.assertIsNotNone(pattern.match(value), value)
        for value in invalid:
            self.assertIsNone(pattern.match(value), value)

    def test_pattern_rejects_long_invalid_input_quickly(self):
        """Guard against catastrophic backtracking on invalid input."""
        import re
        import time

        from tasks.schema.produce_byproduct import GO_DURATION_PATTERN

        start = time.monotonic()
        self.assertIsNone(re.match(GO_DURATION_PATTERN, "1h30m" * 5000 + "!"))
        self.assertLess(time.monotonic() - start, 1)

    def test_agent_duration_defaults_match_pattern(self):
        """Every duration default shipped in the Agent schemas must satisfy the exported pattern."""
        import re

        from tasks.schema.generate import JSON_SCHEMA_ENTRY_POINTS
        from tasks.schema.merge_schema import resolve_schema
        from tasks.schema.produce_byproduct import GO_DURATION_PATTERN, json_schema

        def durations(node, path):
            if isinstance(node, dict):
                if node.get("pattern") == GO_DURATION_PATTERN:
                    yield path, node
                for key, value in node.items():
                    if key not in ("default", "enum", "const", "examples"):
                        yield from durations(value, f"{path}/{key}")
            elif isinstance(node, list):
                for i, value in enumerate(node):
                    yield from durations(value, f"{path}/{i}")

        for name, path in JSON_SCHEMA_ENTRY_POINTS.items():
            found = list(durations(json_schema(resolve_schema(path)), name))
            self.assertTrue(found, f"no duration settings found in {name}")
            for setting, node in found:
                self.assertNotIn("format", node, setting)
                if "default" in node:
                    self.assertRegex(node["default"], re.compile(GO_DURATION_PATTERN), setting)


if __name__ == "__main__":
    unittest.main()
