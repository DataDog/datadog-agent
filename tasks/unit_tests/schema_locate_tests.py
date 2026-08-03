import os
import tempfile
import textwrap
import unittest

import tasks.schema.locate as locate


def _write(path, content):
    with open(path, "w") as f:
        f.write(textwrap.dedent(content))


class _SchemaFixture(unittest.TestCase):
    """Builds a synthetic split schema in a tempdir.

    Mirrors the real layout: a top file (``core.yaml``) with an inline section
    (``proxy``) and a ``$ref`` split section (``apm_config``) whose body lives in
    a sibling file (``apm_config.yaml``). A second standalone schema
    (``sysprobe.yaml``) shares the ``log_level`` setting so cross-schema matching
    can be exercised.
    """

    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name

        # core top file: api_key (setting), log_level (setting), proxy (inline
        # section), apm_config (split section via $ref).
        _write(
            self._path("core.yaml"),
            """\
            properties:
              api_key:
                node_type: setting
                type: string
                default: ''
                description: The API key.
              log_level:
                node_type: setting
                type: string
                default: info
                description: Core log level.
              proxy:
                node_type: section
                type: object
                description: Proxy settings.
                properties:
                  https:
                    node_type: setting
                    type: string
                    default: ''
                    description: HTTPS proxy.
              apm_config:
                $ref: apm_config.yaml
            """,
        )
        _write(
            self._path("apm_config.yaml"),
            """\
            node_type: section
            type: object
            description: APM settings.
            properties:
              enabled:
                node_type: setting
                type: boolean
                default: false
                description: Enable APM.
              env:
                node_type: setting
                type: string
                default: ''
                description: APM env.
            """,
        )
        _write(
            self._path("sysprobe.yaml"),
            """\
            properties:
              log_level:
                node_type: setting
                type: string
                default: info
                description: System-probe log level.
            """,
        )

    def tearDown(self):
        self._tempdir.cleanup()

    def _path(self, name):
        return os.path.join(self.dir, name)


class TestLocatePhysical(_SchemaFixture):
    def _line_of(self, filename, needle):
        """Return the 1-based line number of the first line containing *needle*."""
        with open(self._path(filename)) as f:
            for i, line in enumerate(f, start=1):
                if needle in line:
                    return i
        raise AssertionError(f"{needle!r} not found in {filename}")

    def test_top_level_setting(self):
        result = locate._locate_physical(self._path("core.yaml"), ["api_key"])
        self.assertEqual(result, (self._path("core.yaml"), self._line_of("core.yaml", "api_key:")))

    def test_nested_setting_in_inline_section(self):
        result = locate._locate_physical(self._path("core.yaml"), ["proxy", "https"])
        self.assertEqual(result, (self._path("core.yaml"), self._line_of("core.yaml", "https:")))

    def test_bare_split_section_reports_ref_line_in_top_file(self):
        # apm_config's value is a $ref node; per Q4 we report the $ref line in the top file.
        result = locate._locate_physical(self._path("core.yaml"), ["apm_config"])
        self.assertEqual(result, (self._path("core.yaml"), self._line_of("core.yaml", "$ref:")))

    def test_setting_inside_split_section_reports_subfile(self):
        result = locate._locate_physical(self._path("core.yaml"), ["apm_config", "enabled"])
        self.assertEqual(result, (self._path("apm_config.yaml"), self._line_of("apm_config.yaml", "enabled:")))

    def test_unknown_top_level_returns_none(self):
        self.assertIsNone(locate._locate_physical(self._path("core.yaml"), ["nope"]))

    def test_extra_part_under_leaf_returns_none(self):
        self.assertIsNone(locate._locate_physical(self._path("core.yaml"), ["api_key", "sub"]))


class TestNavigateMerged(_SchemaFixture):
    def _merged(self):
        from tasks.schema.merge_schema import resolve_schema

        return resolve_schema(self._path("core.yaml"))

    def test_finds_top_level_setting(self):
        node = locate._navigate_merged(self._merged(), ["api_key"])
        self.assertEqual(node["type"], "string")
        self.assertEqual(node["description"], "The API key.")

    def test_finds_setting_inside_split_section(self):
        # $ref must be resolved for the content to be reachable.
        node = locate._navigate_merged(self._merged(), ["apm_config", "enabled"])
        self.assertEqual(node["type"], "boolean")
        self.assertEqual(node["default"], False)

    def test_finds_split_section_itself(self):
        node = locate._navigate_merged(self._merged(), ["apm_config"])
        self.assertEqual(node["node_type"], "section")
        self.assertIn("enabled", node["properties"])

    def test_missing_returns_none(self):
        self.assertIsNone(locate._navigate_merged(self._merged(), ["nope"]))

    def test_extra_part_under_leaf_returns_none(self):
        self.assertIsNone(locate._navigate_merged(self._merged(), ["api_key", "sub"]))


class TestDisplayNode(unittest.TestCase):
    def test_setting_is_returned_in_full(self):
        node = {"node_type": "setting", "type": "string", "default": "", "description": "x"}
        self.assertEqual(locate._display_node(node), node)

    def test_section_drops_property_bodies_to_sorted_key_list(self):
        node = {
            "node_type": "section",
            "type": "object",
            "description": "APM settings.",
            "properties": {
                "env": {"node_type": "setting", "type": "string"},
                "enabled": {"node_type": "setting", "type": "boolean"},
            },
        }
        result = locate._display_node(node)
        self.assertEqual(result["description"], "APM settings.")
        self.assertEqual(result["properties"], ["enabled", "env"])

    def test_node_with_properties_but_no_node_type_treated_as_section(self):
        node = {"properties": {"b": {"type": "string"}, "a": {"type": "string"}}}
        self.assertEqual(locate._display_node(node)["properties"], ["a", "b"])


class TestLocateSetting(_SchemaFixture):
    def _schemas(self):
        return [("core", self._path("core.yaml")), ("system-probe", self._path("sysprobe.yaml"))]

    def test_match_only_in_core(self):
        matches = locate.locate_setting("api_key", self._schemas())
        self.assertEqual(len(matches), 1)
        m = matches[0]
        self.assertEqual(m["schema"], "core")
        self.assertEqual(m["path"], "api_key")
        self.assertEqual(m["file"], self._path("core.yaml"))
        self.assertEqual(m["node"]["description"], "The API key.")
        self.assertIsInstance(m["line"], int)

    def test_match_in_both_schemas(self):
        matches = locate.locate_setting("log_level", self._schemas())
        self.assertEqual([m["schema"] for m in matches], ["core", "system-probe"])
        self.assertEqual(matches[0]["file"], self._path("core.yaml"))
        self.assertEqual(matches[1]["file"], self._path("sysprobe.yaml"))
        self.assertEqual(matches[1]["node"]["description"], "System-probe log level.")

    def test_split_section_match_shapes_node(self):
        matches = locate.locate_setting("apm_config", self._schemas())
        self.assertEqual(len(matches), 1)
        # Section node: properties reduced to a sorted key list, located at the $ref site.
        self.assertEqual(matches[0]["node"]["properties"], ["enabled", "env"])
        self.assertEqual(matches[0]["file"], self._path("core.yaml"))

    def test_missing_returns_empty(self):
        self.assertEqual(locate.locate_setting("does.not.exist", self._schemas()), [])


class TestIsPattern(unittest.TestCase):
    def test_dotted_path_is_not_a_pattern(self):
        self.assertFalse(locate.is_pattern("api_key"))
        self.assertFalse(locate.is_pattern("apm_config.enabled"))
        # Keys may contain dots internally (e.g. patternProperties leaves).
        self.assertFalse(locate.is_pattern("_dd.origin"))

    def test_glob_and_regex_metacharacters_are_patterns(self):
        for s in ("*enabled", "enabled$", r"apm_config\..*", "proxy.[a-z]+", "^api"):
            self.assertTrue(locate.is_pattern(s), s)


class TestLocatePattern(_SchemaFixture):
    def _schemas(self):
        return [("core", self._path("core.yaml")), ("system-probe", self._path("sysprobe.yaml"))]

    def test_glob_suffix_lists_all_paths_ending_in_token(self):
        # '*enabled' is not valid regex -> falls back to fnmatch (ends-with).
        matches = locate.locate_pattern("*enabled", self._schemas())
        self.assertEqual([m["path"] for m in matches], ["apm_config.enabled"])
        # The split-section setting is located in its sub-file.
        self.assertEqual(matches[0]["file"], self._path("apm_config.yaml"))
        self.assertEqual(matches[0]["node"]["type"], "boolean")

    def test_regex_anchor_matches_full_paths(self):
        matches = locate.locate_pattern(r"\.enabled$", self._schemas())
        self.assertEqual([m["path"] for m in matches], ["apm_config.enabled"])

    def test_pattern_matches_across_schemas_and_sorts(self):
        # 'log_level' lives in both schemas; a contains-match finds both, sorted by (path, schema).
        matches = locate.locate_pattern("log.*", self._schemas())
        self.assertEqual(
            [(m["path"], m["schema"]) for m in matches],
            [("log_level", "core"), ("log_level", "system-probe")],
        )

    def test_pattern_matches_sections_too(self):
        matches = locate.locate_pattern("^proxy$", self._schemas())
        self.assertEqual([m["path"] for m in matches], ["proxy"])
        # Section node: properties reduced to a sorted key list.
        self.assertEqual(matches[0]["node"]["properties"], ["https"])

    def test_no_match_returns_empty(self):
        self.assertEqual(locate.locate_pattern("does_not_match_anything", self._schemas()), [])

    def test_run_locate_dispatches_to_pattern_and_renders_compact(self):
        out = locate.run_locate("*enabled", schemas=self._schemas())
        self.assertIn("[core] apm_config.enabled  ->  " + self._path("apm_config.yaml"), out)
        # Compact mode does not dump the node body.
        self.assertNotIn("node_type:", out)

    def test_run_locate_pattern_no_match_raises_exit(self):
        from invoke.exceptions import Exit

        with self.assertRaises(Exit):
            locate.run_locate("*nope_nope", schemas=self._schemas())


class TestRender(unittest.TestCase):
    MATCHES = [
        {
            "schema": "core",
            "path": "log_level",
            "file": "pkg/config/schema/yaml/core_schema.yaml",
            "line": 42,
            "node": {"node_type": "setting", "type": "string", "default": "info"},
        },
        {
            "schema": "system-probe",
            "path": "log_level",
            "file": "pkg/config/schema/yaml/system-probe_schema.yaml",
            "line": 7,
            "node": {"node_type": "setting", "type": "string", "default": "info"},
        },
    ]

    def test_text_render_includes_clickable_location_per_match(self):
        out = locate._render(self.MATCHES, as_json=False)
        self.assertIn("[core] pkg/config/schema/yaml/core_schema.yaml:42", out)
        self.assertIn("[system-probe] pkg/config/schema/yaml/system-probe_schema.yaml:7", out)
        # The node body is dumped too.
        self.assertIn("node_type: setting", out)

    def test_json_render_is_always_an_array(self):
        import json

        single = locate._render(self.MATCHES[:1], as_json=True)
        parsed = json.loads(single)
        self.assertIsInstance(parsed, list)
        self.assertEqual(len(parsed), 1)
        self.assertEqual(parsed[0]["schema"], "core")
        self.assertEqual(parsed[0]["line"], 42)
        self.assertEqual(parsed[0]["node"]["type"], "string")


class TestLocateTask(_SchemaFixture):
    def _schemas(self):
        return [("core", self._path("core.yaml")), ("system-probe", self._path("sysprobe.yaml"))]

    def test_not_found_raises_exit(self):
        from invoke.exceptions import Exit

        with self.assertRaises(Exit):
            locate.run_locate("does.not.exist", schemas=self._schemas())

    def test_run_locate_returns_text_with_location(self):
        out = locate.run_locate("api_key", schemas=self._schemas())
        self.assertIn("[core] " + self._path("core.yaml"), out)

    def test_target_filters_to_one_schema(self):
        matches = locate.locate_setting("log_level", self._schemas())
        self.assertEqual(len(matches), 2)
        filtered = locate._select_schemas(self._schemas(), "system-probe")
        self.assertEqual([label for label, _ in filtered], ["system-probe"])

    def test_run_locate_target_filters(self):
        import json

        out = locate.run_locate("log_level", target="system-probe", as_json=True, schemas=self._schemas())
        parsed = json.loads(out)
        self.assertEqual([m["schema"] for m in parsed], ["system-probe"])

    def test_invalid_target_raises_exit(self):
        from invoke.exceptions import Exit

        with self.assertRaises(Exit):
            locate._select_schemas(self._schemas(), "nope")


if __name__ == "__main__":
    unittest.main()
