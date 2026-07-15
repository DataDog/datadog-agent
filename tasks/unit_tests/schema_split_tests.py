import copy
import os
import tempfile
import unittest


def _sample_schema():
    return {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://example.com/datadog.yaml.schema.json",
        "title": "DataDog Agent configuration schema",
        "description": "schema description",
        "properties": {
            "api_key": {
                "node_type": "setting",
                "type": "string",
                "default": "",
            },
            "logs_config": {
                "node_type": "section",
                "type": "object",
                "description": "Logs settings",
                "properties": {
                    "enabled": {
                        "node_type": "setting",
                        "type": "boolean",
                        "default": False,
                    },
                    "compression_level": {
                        "node_type": "setting",
                        "type": "number",
                        "default": 6,
                    },
                },
            },
            "proxy": {
                "node_type": "section",
                "type": "object",
                "description": "Proxy settings",
                "properties": {
                    "http": {"node_type": "setting", "type": "string", "default": ""},
                },
            },
        },
    }


class TestSplitAndWriteSchema(unittest.TestCase):
    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name

    def tearDown(self):
        self._tempdir.cleanup()

    def test_split_section_becomes_ref_and_subfile_is_written(self):
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        split_and_write_schema(schema, self.dir, ["logs_config"], "core_schema")

        # In-memory schema entry was replaced by a $ref.
        self.assertEqual(schema["properties"]["logs_config"], {"$ref": "logs_config.yaml"})

        # Sub-file was written with the original section's content.
        sub_path = os.path.join(self.dir, "logs_config.yaml")
        self.assertTrue(os.path.isfile(sub_path), f"expected {sub_path} to exist")

        import yaml

        with open(sub_path) as f:
            sub = yaml.safe_load(f)
        original_section = _sample_schema()["properties"]["logs_config"]
        self.assertEqual(sub.pop("$schema"), "https://json-schema.org/draft/2020-12/schema")
        self.assertEqual(
            sub.pop("$id"),
            "https://raw.githubusercontent.com/DataDog/schema/main/agent/logs_config.yaml.schema.json",
        )
        self.assertEqual(sub, original_section)

    def test_top_file_is_written(self):
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        split_and_write_schema(schema, self.dir, ["logs_config"], "core_schema")

        top_path = os.path.join(self.dir, "core_schema.yaml")
        self.assertTrue(os.path.isfile(top_path), f"expected {top_path} to exist")

        import yaml

        with open(top_path) as f:
            top = yaml.safe_load(f)
        self.assertEqual(top["properties"]["logs_config"], {"$ref": "logs_config.yaml"})

    def test_subfile_has_header_keys_before_section_content(self):
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        split_and_write_schema(schema, self.dir, ["logs_config"], "core_schema")

        import yaml

        with open(os.path.join(self.dir, "logs_config.yaml")) as f:
            sub = yaml.safe_load(f)
        keys = list(sub.keys())
        # $schema, $id come first so the file reads as a proper JSON-schema document.
        self.assertEqual(keys[0], "$schema")
        self.assertEqual(keys[1], "$id")

    def test_section_not_in_split_list_stays_inline(self):
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        split_and_write_schema(schema, self.dir, ["logs_config"], "core_schema")

        self.assertEqual(schema["properties"]["proxy"], _sample_schema()["properties"]["proxy"])
        self.assertFalse(os.path.isfile(os.path.join(self.dir, "proxy.yaml")))

    def test_top_level_metadata_is_preserved(self):
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        split_and_write_schema(schema, self.dir, ["logs_config"], "core_schema")

        for key in ("$schema", "$id", "title", "description"):
            self.assertEqual(schema[key], _sample_schema()[key])

    def test_round_trip_with_resolver_returns_original(self):
        """split + resolve_schema(top file written to disk) ≡ original dict."""
        from tasks.schema.generate import split_and_write_schema
        from tasks.schema.merge_schema import resolve_schema

        original = _sample_schema()
        schema = copy.deepcopy(original)
        # split_and_write_schema writes the top file as part of its job.
        split_and_write_schema(schema, self.dir, ["logs_config", "proxy"], "core_schema")

        merged = resolve_schema(os.path.join(self.dir, "core_schema.yaml"))
        self.assertEqual(merged, original)

    def test_missing_section_is_silently_skipped(self):
        """Sections in the split list that don't exist in the schema are skipped, not errors.

        This makes the splitter robust to the schema temporarily losing a
        section (e.g. during refactoring), without forcing a code update.
        """
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        # 'nonexistent_section' is not in the schema.
        split_and_write_schema(schema, self.dir, ["logs_config", "nonexistent_section"], "core_schema")
        self.assertEqual(schema["properties"]["logs_config"], {"$ref": "logs_config.yaml"})
        self.assertFalse(os.path.isfile(os.path.join(self.dir, "nonexistent_section.yaml")))

    def test_no_sections_writes_only_top_file(self):
        """When sections is None (system-probe case), no splitting happens —
        the schema is written as-is to <name>.yaml."""
        from tasks.schema.generate import split_and_write_schema

        schema = _sample_schema()
        split_and_write_schema(schema, self.dir, None, "system-probe_schema")

        top_path = os.path.join(self.dir, "system-probe_schema.yaml")
        self.assertTrue(os.path.isfile(top_path))

        import yaml

        with open(top_path) as f:
            written = yaml.safe_load(f)
        self.assertEqual(written, _sample_schema())  # untouched

        # No sub-files were created.
        self.assertFalse(os.path.isfile(os.path.join(self.dir, "logs_config.yaml")))


if __name__ == "__main__":
    unittest.main()
