import unittest


class TestJsonSchemaOutput(unittest.TestCase):
    def test_keeps_only_standard_keywords(self):
        """Agent extensions are dropped, and so is `default`, see json_schema."""
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
                    },
                },
            },
        )

    def test_object_default_is_preserved_verbatim(self):
        """Regression: instance data under ``default`` must not be walked as a
        schema subtree, or its non-keyword keys are dropped to ``{}``. Only the
        embedded schema keeps defaults."""
        from tasks.schema.produce_byproduct import embedded

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
        result = embedded(doc)
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
    def test_duration_format_becomes_ref(self):
        from tasks.schema.produce_byproduct import AGENT_FORMATS, GO_DURATION_PATTERN, json_schema

        doc = {
            "type": "object",
            "properties": {
                "timeout": {"node_type": "setting", "type": "string", "default": "15s", "format": "duration"},
                "intervals": {"type": "array", "items": {"type": "string", "format": "duration"}},
                "name": {"type": "string", "format": "hostname"},
            },
        }
        result = json_schema(doc)
        props = result["properties"]
        self.assertEqual(props["timeout"], {"type": "string", "$ref": "#/$defs/duration"})
        self.assertEqual(props["intervals"]["items"], {"type": "string", "$ref": "#/$defs/duration"})
        self.assertEqual(props["name"], {"type": "string", "format": "hostname"})
        # the regexp is written once, in $defs, with the message IDEs show instead of it
        self.assertEqual(
            result["$defs"],
            {
                "duration": {
                    "type": "string",
                    "pattern": GO_DURATION_PATTERN,
                    "patternErrorMessage": AGENT_FORMATS["duration"].error_message,
                }
            },
        )

    def test_existing_pattern_and_ref_are_kept(self):
        from tasks.schema.produce_byproduct import json_schema

        # the setting's own pattern and the format's both apply: they're in different schemas
        doc = {"type": "string", "format": "duration", "pattern": "^[0-9]+s$"}
        self.assertEqual(
            {k: v for k, v in json_schema(doc).items() if k != "$defs"},
            {"type": "string", "pattern": "^[0-9]+s$", "$ref": "#/$defs/duration"},
        )
        # a schema holds one $ref: an existing one is kept, and both are required
        doc = {"type": "string", "format": "url", "$ref": "#/$defs/other"}
        self.assertEqual(
            {k: v for k, v in json_schema(doc).items() if k != "$defs"},
            {"type": "string", "$ref": "#/$defs/other", "allOf": [{"$ref": "#/$defs/url"}]},
        )

    def test_no_defs_without_formats(self):
        from tasks.schema.produce_byproduct import embedded, json_schema

        doc = {"type": "object", "properties": {"x": {"type": "string"}}}
        for output in (embedded, json_schema):
            self.assertNotIn("$defs", output(doc), output.__name__)

    def test_instance_data_is_not_rewritten(self):
        from tasks.schema.produce_byproduct import embedded, json_schema

        doc = {"type": "object", "default": {"format": "duration"}, "enum": [{"format": "duration"}]}
        self.assertEqual(embedded(doc), doc)
        self.assertEqual(json_schema(doc), {"type": "object", "enum": [{"format": "duration"}]})

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

    def test_agent_formats_become_refs_in_every_output(self):
        """The Agent and external tools validate the same way: every Agent format refers to its regexp in every
        output, written once in $defs."""
        from tasks.schema.produce_byproduct import AGENT_FORMATS, embedded, json_schema

        for name, agent_format in AGENT_FORMATS.items():
            self.assertTrue(agent_format.error_message, f"{name} has no error message")
            doc = {"type": "object", "properties": {"setting": {"type": "string", "format": name}}}
            # only the published schema gets the message: the Agent's validator doesn't read it
            for output, extra in (
                (embedded, {}),
                (json_schema, {"patternErrorMessage": agent_format.error_message}),
            ):
                result = output(doc)
                label = f"{name} in {output.__name__}"
                self.assertEqual(result["properties"]["setting"], {"type": "string", "$ref": f"#/$defs/{name}"}, label)
                self.assertEqual(
                    result["$defs"], {name: {"type": "string", "pattern": agent_format.pattern, **extra}}, label
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

    def test_agent_format_settings_use_their_ref(self):
        """Every setting of the Agent schemas using an Agent format refers to its definition, with no `format` left,
        and each regexp is written once. Defaults are checked by test_defaults_match_their_schema."""
        import json

        from tasks.schema.generate import JSON_SCHEMA_ENTRY_POINTS
        from tasks.schema.merge_schema import resolve_schema
        from tasks.schema.produce_byproduct import AGENT_FORMATS, embedded, json_schema

        for name, path in JSON_SCHEMA_ENTRY_POINTS.items():
            source = resolve_schema(path)
            used = {fmt: json.dumps(source).count(f'"format": "{fmt}"') for fmt in AGENT_FORMATS}
            self.assertGreater(used["duration"], 0, name)
            for output in (embedded, json_schema):
                text = json.dumps(output(source))
                label = f"{name} {output.__name__}"
                self.assertNotIn('"format": ', text, label)
                for fmt, count in used.items():
                    self.assertEqual(text.count(f'"$ref": "#/$defs/{fmt}"'), count, f"{label}: {fmt}")
                    self.assertEqual(
                        text.count(json.dumps(AGENT_FORMATS[fmt].pattern)), 1 if count else 0, f"{label}: {fmt}"
                    )


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
        patterns = list(_schema_patterns()) + [(f"AGENT_FORMATS[{n!r}]", f.pattern) for n, f in AGENT_FORMATS.items()]
        self.assertTrue(patterns)
        for setting, pattern in patterns:
            try:
                subprocess.run([sys.executable, "-I", "-c", matcher, pattern, *inputs], check=True, timeout=10)
            except subprocess.TimeoutExpired:
                self.fail(f"{setting}: {pattern!r} backtracks catastrophically on one of {inputs!r}")

    def test_url_settings(self):
        from tasks.schema.lint import walk_nodes
        from tasks.schema.merge_schema import resolve_schema

        url_settings = sorted(
            path
            for path, node in walk_nodes(resolve_schema("pkg/config/schema/yaml/core_schema.yaml"))
            if node.get("format") == "url"
        )
        self.assertEqual(url_settings, ["apm_config.apm_dd_url", "dd_url", "logs_config.logs_dd_url"])

    def test_url_pattern(self):
        """The URL pattern is deliberately lax: rejecting a working URL is worse than accepting a bad one."""
        import re

        from tasks.schema.produce_byproduct import URL_PATTERN

        pattern = re.compile(URL_PATTERN)
        valid = [
            # An empty value means "unset" for the URL settings: the Agent then builds the endpoint from
            # `site` (GetMainEndpoint in pkg/config/utils/endpoints.go for dd_url and apm_dd_url,
            # LogsConfigKeys.logsDDURL in comp/logs/agent/config/config_keys.go for logs_dd_url).
            "",
            "intake.datadoghq.com",
            "https://app.datadoghq.com",
            "https://app.datadoghq.com/",
            "http://proxy:3128",
            "https://proxy.example.com:443/logs",
            "localhost:8080",
            "https://[::1]:8443",
            "https://[2001:db8::1]/path",
            "https://[fe80::1%25eth0]:443",
            "http://user:pass@proxy:3128",
            "https://x.com/path?a=b&c=d#frag",
            "my_host.internal:8080",
        ]
        invalid = ["ftp://x.com", "https://", "https://[]", "http://@x.com", "https://x.com:123456"]
        invalid += ["goog- le.com", "https://x.com/a b", "https:// x.com"]  # URLs can't contain spaces
        for value in valid:
            self.assertRegex(value, pattern)
        for value in invalid:
            self.assertNotRegex(value, pattern)


class TestSchemaDefaults(unittest.TestCase):
    def test_defaults_match_their_schema(self):
        """Every default, including every per-platform value of `platform_default`, must be a valid value of its
        setting: IDEs and CI tools flag the default configuration otherwise. The Go test validating the default
        configuration only covers the platforms the unit tests run on.
        """
        from tasks.schema.generate import JSON_SCHEMA_ENTRY_POINTS
        from tasks.schema.lint import walk_nodes
        from tasks.schema.merge_schema import resolve_schema
        from tasks.schema.produce_byproduct import json_schema

        checked = 0
        for name, path in JSON_SCHEMA_ENTRY_POINTS.items():
            source = resolve_schema(path)
            exported = json_schema(source)
            for setting, node in walk_nodes(source):
                if node.get("node_type") != "setting":
                    continue
                candidates = {"default": node["default"]} if "default" in node else {}
                for platform, value in (node.get("platform_default") or {}).items():
                    candidates[f"platform_default.{platform}"] = value
                value_schema = exported
                for part in setting.split("."):
                    value_schema = value_schema["properties"][part]
                for label, value in candidates.items():
                    checked += 1
                    errors = _violations(value, value_schema, root=exported)
                    self.assertEqual(errors, [], f"{name}: {setting} {label} = {value!r}")
        self.assertGreater(checked, 2000)

    def test_validator_catches_invalid_values(self):
        """Guard the helper validating defaults: it must reject what a JSON Schema validator rejects."""
        schema = {
            "type": "object",
            "properties": {"port": {"type": "integer", "minimum": 1}, "mode": {"type": "string", "enum": ["a"]}},
            "additionalProperties": {"type": "array", "items": {"type": "string", "pattern": "^[a-z]+$"}},
        }
        self.assertEqual(_violations({"port": 1, "mode": "a", "x": ["ok"]}, schema), [])
        for invalid in ({"port": 0}, {"port": True}, {"port": "1"}, {"mode": "b"}, {"x": ["NO"]}, {"x": "ok"}):
            self.assertNotEqual(_violations(invalid, schema), [], invalid)
        with self.assertRaises(AssertionError):
            _violations("x", {"type": "string", "multipleOf": 2})
        root = {"$defs": {"duration": {"type": "string", "pattern": "^[0-9]+s$"}}}
        self.assertEqual(_violations("15s", {"$ref": "#/$defs/duration"}, root=root), [])
        self.assertNotEqual(_violations("15m", {"$ref": "#/$defs/duration"}, root=root), [])


# Keywords that only annotate a schema: they never reject a value.
_ANNOTATIONS = frozenset(
    {"default", "description", "title", "deprecated", "examples", "$schema", "$id", "$comment", "patternErrorMessage"}
)


def _violations(value, schema, location="#", root=None):
    """Return why ``value`` doesn't validate against ``schema``, a JSON schema using the keywords of Agent settings.

    The tasks don't depend on a JSON Schema library, so this covers the keywords the exported schemas use, and
    fails on any other keyword instead of silently ignoring it. A local ``$ref`` (``#/$defs/...``) is resolved
    against ``root``, the document ``schema`` comes from.
    """
    import json
    import re

    types = {
        "string": lambda v: isinstance(v, str),
        "integer": lambda v: isinstance(v, int) and not isinstance(v, bool),
        "number": lambda v: isinstance(v, int | float) and not isinstance(v, bool),
        "boolean": lambda v: isinstance(v, bool),
        "array": lambda v: isinstance(v, list),
        "object": lambda v: isinstance(v, dict),
        "null": lambda v: v is None,
    }
    errors = []
    for key, rule in schema.items():
        if key in _ANNOTATIONS:
            continue
        if key == "type":
            allowed = rule if isinstance(rule, list) else [rule]
            if not any(types[t](value) for t in allowed):
                errors.append(f"{location}: {value!r} is not of type {rule}")
        elif key == "enum":
            if json.dumps(value, sort_keys=True) not in {json.dumps(v, sort_keys=True) for v in rule}:
                errors.append(f"{location}: {value!r} is not one of {rule}")
        elif key == "const":
            if json.dumps(value, sort_keys=True) != json.dumps(rule, sort_keys=True):
                errors.append(f"{location}: {value!r} is not {rule!r}")
        elif key == "pattern":
            if isinstance(value, str) and not re.search(rule, value):
                errors.append(f"{location}: {value!r} does not match {rule!r}")
        elif key in ("minLength", "maxLength"):
            if isinstance(value, str) and (len(value) < rule if key == "minLength" else len(value) > rule):
                errors.append(f"{location}: {value!r} breaks {key} {rule}")
        elif key in ("minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum"):
            if types["number"](value):
                broken = {
                    "minimum": value < rule,
                    "maximum": value > rule,
                    "exclusiveMinimum": value <= rule,
                    "exclusiveMaximum": value >= rule,
                }[key]
                if broken:
                    errors.append(f"{location}: {value!r} breaks {key} {rule}")
        elif key == "items":
            if isinstance(value, list):
                for i, item in enumerate(value):
                    errors += _violations(item, rule, f"{location}/{i}", root=root)
        elif key == "properties":
            if isinstance(value, dict):
                for name, sub in rule.items():
                    if name in value:
                        errors += _violations(value[name], sub, f"{location}/{name}", root=root)
        elif key == "required":
            if isinstance(value, dict):
                errors += [f"{location}: missing {name!r}" for name in rule if name not in value]
        elif key in ("additionalProperties", "unevaluatedProperties"):
            # without applicators next to them, both apply to the keys not listed in `properties`
            assert not {"allOf", "anyOf", "oneOf", "patternProperties"} & set(schema), f"{location}: unsupported {key}"
            if isinstance(value, dict):
                for name, item in value.items():
                    if name in schema.get("properties", {}):
                        continue
                    if rule is False:
                        errors.append(f"{location}: unexpected property {name!r}")
                    elif isinstance(rule, dict):
                        errors += _violations(item, rule, f"{location}/{name}", root=root)
        elif key == "$ref":
            assert rule.startswith("#/"), f"{location}: only local $ref are supported, got {rule!r}"
            target = root
            for part in rule[2:].split("/"):
                target = target[part]
            errors += _violations(value, target, location, root=root)
        elif key == "allOf":
            for sub in rule:
                errors += _violations(value, sub, location, root=root)
        elif key in ("anyOf", "oneOf"):
            matching = sum(1 for sub in rule if not _violations(value, sub, location, root=root))
            if matching == 0 or (key == "oneOf" and matching > 1):
                errors.append(f"{location}: {value!r} doesn't match {key} ({matching} matching)")
        else:
            raise AssertionError(f"{location}: keyword {key!r} isn't supported, add it to _violations")
    return errors


class TestPlatformDefaultsDescription(unittest.TestCase):
    def test_sentence(self):
        from tasks.schema.produce_byproduct import platform_defaults_sentence

        for platform_default, expected in (
            ({"darwin": 5002, "windows": 5002, "other": -1}, "Default: 5002 on Windows and macOS, -1 elsewhere."),
            (
                {"linux": True, "container": False, "other": False},
                "Default: true on Linux, false in containers and elsewhere.",
            ),
            (
                # strings are quoted as they are: no JSON escaping of Windows paths
                {"linux": "/a", "windows": "C:\\a", "darwin": "/a", "aix": "/a"},
                'Default: "/a" on Linux, macOS and AIX, "C:\\a" on Windows.',
            ),
            ({"linux": "/run/x.sock", "other": ""}, 'Default: "/run/x.sock" on Linux, "" elsewhere.'),
            ({"other": [1, "a"]}, 'Default: [1, "a"] on every platform.'),
        ):
            self.assertEqual(platform_defaults_sentence(platform_default), expected)

    def test_exported_description(self):
        """The export drops every default, and describes the default of public settings only."""
        from tasks.schema.produce_byproduct import json_schema

        def setting(public, description=None, **default):
            node = {"node_type": "setting", "type": "integer", **default}
            if public:
                node["visibility"] = "public"
            if description:
                node["description"] = description
            return node

        doc = {
            "properties": {
                "public_default": setting(True, "The port.", default=5000),
                "public_platform_default": setting(True, "The port.", platform_default={"windows": 1, "other": 2}),
                "public_without_description": setting(True, platform_default={"windows": 1, "other": 2}),
                "private_default": setting(False, "Internal.", default=5000),
                "private_platform_default": setting(False, platform_default={"windows": 1, "other": 2}),
            }
        }
        props = json_schema(doc)["properties"]
        self.assertEqual(props["public_default"]["description"], "The port.\n\nDefault: 5000.")
        self.assertEqual(
            props["public_platform_default"]["description"], "The port.\n\nDefault: 1 on Windows, 2 elsewhere."
        )
        self.assertEqual(props["public_without_description"]["description"], "Default: 1 on Windows, 2 elsewhere.")
        self.assertEqual(props["private_default"]["description"], "Internal.")
        self.assertNotIn("description", props["private_platform_default"])
        for name, node in props.items():
            self.assertNotIn("default", node, name)


class TestJsonSchemaId(unittest.TestCase):
    PREFIX = "https://raw.githubusercontent.com/DataDog/schema/main/datadog-agent/"

    def test_latest_id(self):
        from tasks.schema.generate import json_schema_id

        self.assertEqual(json_schema_id("datadog"), self.PREFIX + "datadog.schema.json")
        self.assertEqual(json_schema_id("system-probe"), self.PREFIX + "system-probe.schema.json")

    def test_versioned_id(self):
        from tasks.schema.generate import json_schema_id

        for version in ("7.85.1", "v7.85.1"):
            self.assertEqual(json_schema_id("datadog", version), self.PREFIX + "v7.85.1/datadog.schema.json")

    def test_prerelease_versioned_id(self):
        """Release candidates and dev builds (`dda inv agent.version --url-safe`) get their own folder too."""
        from tasks.schema.generate import json_schema_id

        for version in ("7.87.1-rc.2", "v7.87.1-rc.2"):
            self.assertEqual(json_schema_id("datadog", version), self.PREFIX + "v7.87.1-rc.2/datadog.schema.json")
        self.assertEqual(
            json_schema_id("system-probe", "7.87.0-devel.git.969.6db8b91"),
            self.PREFIX + "v7.87.0-devel.git.969.6db8b91/system-probe.schema.json",
        )

    def test_build_suffix_versioned_id(self):
        """`agent.version --url-safe` appends `.git.<commits>.<sha>` (and `.pipeline.<id>` on nightlies) after a
        dot for a commit past a tag, including on a stable base version with no pre-release part."""
        from tasks.schema.generate import json_schema_id

        for version in (
            "7.87.1.git.1.6db8b91",
            "7.87.1.git.0.6db8b91.pipeline.12345",
            "7.87.0-devel.git.969.6db8b91.pipeline.12345",
        ):
            self.assertEqual(json_schema_id("datadog", version), self.PREFIX + f"v{version}/datadog.schema.json")

    def test_any_version_is_used_as_given(self):
        """The version isn't parsed: whatever `agent.version` prints, or a maintainer passes, becomes the folder."""
        from tasks.schema.generate import json_schema_id

        for version in ("7.85", "main", "7.85.1+git.1", "7.87.1-rc.2.git.3.abc", "8.0.0-beta"):
            self.assertEqual(json_schema_id("datadog", version), self.PREFIX + f"v{version}/datadog.schema.json")

    def test_invalid_versions_are_rejected(self):
        """The version is a folder of the published URL: it can't be empty, or split or escape that folder."""
        from tasks.schema.generate import json_schema_id

        for version in (
            "",
            "v",
            "7.85.1/x",
            "../7.85.1",
            "7.85.1\\x",
            "7.85.1 rc",
            "7.85.1\t",
            "7.85.1?x",
            "7.85.1#x",
            "7.85.1%2F",
        ):
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
                produce_jsonschema(Context(), out, version="7.85.1/x")
            self.assertEqual(os.listdir(out), [], "nothing must be written for an invalid version")


if __name__ == "__main__":
    unittest.main()
