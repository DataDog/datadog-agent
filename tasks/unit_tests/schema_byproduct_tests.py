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

    def test_banner_is_dropped_from_every_output(self):
        """``banner`` only drives the config example: neither the binary nor external tools use it."""
        from tasks.schema.produce_byproduct import embedded, json_schema

        doc = {
            "type": "object",
            "properties": {
                "api_key": {"node_type": "setting", "banner": "Basic Configuration", "type": "string", "default": ""}
            },
        }
        for output in (embedded, json_schema):
            self.assertNotIn("banner", output(doc)["properties"]["api_key"], output.__name__)

    def test_agent_formats_become_patterns_in_every_output(self):
        """The Agent and external tools validate the same way: every Agent format is a pattern in every output."""
        from tasks.schema.produce_byproduct import AGENT_FORMATS, embedded, json_schema

        for name, pattern in AGENT_FORMATS.items():
            doc = {"type": "object", "properties": {"setting": {"type": "string", "format": name}}}
            for output in (embedded, json_schema):
                self.assertEqual(
                    output(doc)["properties"]["setting"],
                    {"type": "string", "pattern": pattern},
                    f"{name} in {output.__name__}",
                )

    def test_unknown_format_is_kept(self):
        from tasks.schema.produce_byproduct import embedded, json_schema

        doc = {"type": "string", "format": "hostname"}
        for output in (embedded, json_schema):
            self.assertEqual(output(doc), doc, output.__name__)

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


def _schema_patterns():
    """Yield (setting path, pattern) for every ``pattern`` in the published Agent JSON schemas."""
    for setting, node in _schema_nodes_with_pattern():
        yield setting, node["pattern"]


def _schema_nodes_with_pattern():
    """Yield (setting path, schema node) for every node with a ``pattern`` in the published Agent JSON schemas."""
    from tasks.schema.generate import JSON_SCHEMA_ENTRY_POINTS
    from tasks.schema.merge_schema import resolve_schema
    from tasks.schema.produce_byproduct import json_schema

    def walk(node, path):
        if isinstance(node, dict):
            if isinstance(node.get("pattern"), str):
                yield path, node
            for key, value in node.items():
                if key not in ("default", "enum", "const", "examples"):
                    yield from walk(value, f"{path}/{key}")
        elif isinstance(node, list):
            for i, value in enumerate(node):
                yield from walk(value, f"{path}/{i}")

    for name, path in JSON_SCHEMA_ENTRY_POINTS.items():
        yield from walk(json_schema(resolve_schema(path)), name)


class TestSchemaPatterns(unittest.TestCase):
    def test_patterns_reject_long_invalid_input_quickly(self):
        """Guard against catastrophic backtracking: editors validate with backtracking regex engines
        (like Python's ``re``), so a pattern with ambiguous nested quantifiers freezes them on invalid
        input. The Agent itself uses Go's linear-time engine, so it would not catch this.

        A slow pattern never returns in practice (the former dd_url pattern needed hours on a
        40-character value), so matching runs in a subprocess that is killed after a timeout.
        """
        import subprocess
        import sys

        from tasks.schema.produce_byproduct import AGENT_FORMATS

        # Values that almost match common shapes, then fail on the last character.
        inputs = [
            "a" * 40 + "!",
            "https://intake.example.com/" + "a" * 40 + "!",
            "https://intake.example.com:8080/" + "a/" * 40 + "!",
            "com.datadoghq." + "a." * 40 + "!",
            "1h30m" * 40 + "!",
        ]
        matcher = "import re, sys\np = re.compile(sys.argv[1])\nfor value in sys.argv[2:]:\n    p.match(value)\n"
        # every pattern of the schemas, plus every Agent format, used or not yet
        patterns = list(_schema_patterns()) + [(f"AGENT_FORMATS[{n!r}]", p) for n, p in AGENT_FORMATS.items()]
        self.assertTrue(patterns)
        for setting, pattern in patterns:
            try:
                subprocess.run([sys.executable, "-I", "-c", matcher, pattern, *inputs], check=True, timeout=10)
            except subprocess.TimeoutExpired:
                self.fail(f"{setting}: {pattern!r} backtracks catastrophically on one of {inputs!r}")

    def test_dd_url_pattern(self):
        import re

        patterns = {setting: pattern for setting, pattern in _schema_patterns() if setting.endswith("dd_url")}
        self.assertEqual(
            sorted(patterns),
            [
                "datadog/properties/apm_config/properties/apm_dd_url",
                "datadog/properties/dd_url",
                "datadog/properties/logs_config/properties/logs_dd_url",
            ],
        )
        valid = [
            "intake.datadoghq.com",
            "https://app.datadoghq.com",
            "https://app.datadoghq.com/",
            "http://proxy:3128",
            "https://proxy.example.com:443/logs",
            "localhost:8080",
        ]
        # An empty value means "unset" for all three: the Agent then builds the endpoint from `site`
        # (GetMainEndpoint in pkg/config/utils/endpoints.go for dd_url and apm_dd_url,
        # LogsConfigKeys.logsDDURL in comp/logs/agent/config/config_keys.go for logs_dd_url). It is
        # also the default of logs_dd_url and apm_dd_url.
        valid.append("")
        invalid = [
            "ftp://x.com",
            "https://x.com/?q=1",
            "https://user@x.com",
            "https://",
            # URLs can't contain spaces
            "goog- le.com",
            "https://x.com/a b",
        ]
        for setting, pattern in patterns.items():
            for value in valid:
                self.assertRegex(value, re.compile(pattern), setting)
            for value in invalid:
                self.assertNotRegex(value, re.compile(pattern), setting)

    def test_defaults_match_their_pattern(self):
        """A default must be a valid value of its own setting, or IDEs flag the default config as invalid."""
        import re

        nodes = list(_schema_nodes_with_pattern())
        self.assertTrue(nodes)
        for setting, node in nodes:
            if isinstance(node.get("default"), str):
                self.assertRegex(node["default"], re.compile(node["pattern"]), setting)


class TestJsonSchemaId(unittest.TestCase):
    PREFIX = "https://raw.githubusercontent.com/DataDog/schema/main/agent/"

    def test_latest_id(self):
        from tasks.schema.generate import json_schema_id

        self.assertEqual(json_schema_id("datadog"), self.PREFIX + "datadog.schema.json")
        self.assertEqual(json_schema_id("system-probe"), self.PREFIX + "system-probe.schema.json")

    def test_versioned_id(self):
        from tasks.schema.generate import json_schema_id

        for version in ("7.85.1", "v7.85.1"):
            self.assertEqual(json_schema_id("datadog", version), self.PREFIX + "v7.85.1/datadog.schema.json")

    def test_invalid_versions_are_rejected(self):
        from tasks.schema.generate import json_schema_id

        for version in ("", "7.85", "7.85.0-rc.1", "7.85.1-devel", "main", "../7.85.1", "7.85.1/x"):
            with self.assertRaises(ValueError, msg=version):
                json_schema_id("datadog", version)

    def test_schema_id_replaces_source_id_and_keeps_header_first(self):
        from tasks.schema.produce_byproduct import _with_schema_id

        doc = {"$schema": "s", "$id": "old", "title": "t", "properties": {}}
        result = _with_schema_id(doc, "new")
        self.assertEqual(list(result), ["$schema", "$id", "title", "properties"])
        self.assertEqual(result["$id"], "new")
        result = _with_schema_id({"$schema": "s", "properties": {}}, "new")
        self.assertEqual(list(result), ["$schema", "$id", "properties"])

    def test_produce_jsonschema_writes_published_ids(self):
        import json
        import os
        import tempfile

        from invoke import Context
        from invoke.exceptions import Exit

        from tasks.schema.generate import produce_jsonschema

        with tempfile.TemporaryDirectory() as out:
            produce_jsonschema(Context(), out, version="7.85.1")
            self.assertEqual(sorted(os.listdir(out)), ["datadog.schema.json", "system-probe.schema.json"])
            for file_name in os.listdir(out):
                with open(os.path.join(out, file_name), encoding="utf-8") as f:
                    schema = json.load(f)
                # the file can be copied as-is to the URL its $id names
                self.assertEqual(schema["$id"], f"{self.PREFIX}v7.85.1/{file_name}")
                self.assertEqual(list(schema)[:2], ["$schema", "$id"])
                self.assertEqual(schema["type"], "object")
        with tempfile.TemporaryDirectory() as out:
            with self.assertRaises(Exit):
                produce_jsonschema(Context(), out, version="7.85.0-rc.1")
            self.assertEqual(os.listdir(out), [], "nothing must be written for an invalid version")


if __name__ == "__main__":
    unittest.main()
