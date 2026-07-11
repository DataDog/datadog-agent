import os
import tempfile
import textwrap
import unittest


def _write(path, content):
    with open(path, "w") as f:
        f.write(textwrap.dedent(content))


class TestResolveSchema(unittest.TestCase):
    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name

    def tearDown(self):
        self._tempdir.cleanup()

    def _path(self, name):
        return os.path.join(self.dir, name)

    def test_simple_ref_is_inlined(self):
        from tasks.schema.merge_schema import resolve_schema

        _write(
            self._path("logs_config.yaml"),
            """\
            node_type: section
            description: Logs settings
            properties:
              enabled:
                node_type: setting
                type: boolean
                default: false
            """,
        )
        _write(
            self._path("top.yaml"),
            """\
            properties:
              logs_config:
                $ref: logs_config.yaml
            """,
        )

        result = resolve_schema(self._path("top.yaml"))
        self.assertEqual(
            result["properties"]["logs_config"],
            {
                "node_type": "section",
                "description": "Logs settings",
                "properties": {
                    "enabled": {
                        "node_type": "setting",
                        "type": "boolean",
                        "default": False,
                    },
                },
            },
        )

    def test_multiple_refs_resolve(self):
        from tasks.schema.merge_schema import resolve_schema

        _write(self._path("a.yaml"), "x: 1\n")
        _write(self._path("b.yaml"), "y: 2\n")
        _write(
            self._path("top.yaml"),
            """\
            properties:
              a:
                $ref: a.yaml
              b:
                $ref: b.yaml
            """,
        )

        result = resolve_schema(self._path("top.yaml"))
        self.assertEqual(result["properties"]["a"], {"x": 1})
        self.assertEqual(result["properties"]["b"], {"y": 2})

    def test_non_ref_top_level_fields_are_preserved(self):
        from tasks.schema.merge_schema import resolve_schema

        _write(self._path("sub.yaml"), "k: v\n")
        _write(
            self._path("top.yaml"),
            """\
            $schema: https://example.com/schema
            $id: https://example.com/myschema
            title: My Schema
            description: top-level description
            properties:
              inline_section:
                node_type: setting
                type: string
              referenced_section:
                $ref: sub.yaml
            """,
        )

        result = resolve_schema(self._path("top.yaml"))
        self.assertEqual(result["$schema"], "https://example.com/schema")
        self.assertEqual(result["$id"], "https://example.com/myschema")
        self.assertEqual(result["title"], "My Schema")
        self.assertEqual(result["description"], "top-level description")
        self.assertEqual(
            result["properties"]["inline_section"],
            {"node_type": "setting", "type": "string"},
        )
        self.assertEqual(result["properties"]["referenced_section"], {"k": "v"})

    def test_nested_ref_resolves_recursively(self):
        from tasks.schema.merge_schema import resolve_schema

        _write(self._path("inner.yaml"), "deep: value\n")
        _write(
            self._path("mid.yaml"),
            """\
            properties:
              nested:
                $ref: inner.yaml
            """,
        )
        _write(
            self._path("top.yaml"),
            """\
            properties:
              mid:
                $ref: mid.yaml
            """,
        )

        result = resolve_schema(self._path("top.yaml"))
        self.assertEqual(
            result["properties"]["mid"]["properties"]["nested"],
            {"deep": "value"},
        )

    def test_missing_ref_target_raises(self):
        from tasks.schema.merge_schema import resolve_schema

        _write(
            self._path("top.yaml"),
            """\
            properties:
              missing:
                $ref: does_not_exist.yaml
            """,
        )

        with self.assertRaises(FileNotFoundError):
            resolve_schema(self._path("top.yaml"))

    def test_ref_resolved_against_parent_directory(self):
        from tasks.schema.merge_schema import resolve_schema

        subdir = os.path.join(self.dir, "sub")
        os.makedirs(subdir)
        _write(os.path.join(subdir, "x.yaml"), "hello: world\n")
        _write(
            self._path("top.yaml"),
            """\
            properties:
              x:
                $ref: sub/x.yaml
            """,
        )

        result = resolve_schema(self._path("top.yaml"))
        self.assertEqual(result["properties"]["x"], {"hello": "world"})

    def test_inlined_subfile_strips_schema_and_id_headers(self):
        """`$schema` and `$id` belong to the sub-file as a standalone doc; they
        should not leak into the merged parent."""
        from tasks.schema.merge_schema import resolve_schema

        _write(
            self._path("sub.yaml"),
            """\
            $schema: https://json-schema.org/draft/2020-12/schema
            $id: https://example.com/sub.yaml.schema.json
            node_type: section
            properties:
              k:
                type: string
            """,
        )
        _write(
            self._path("top.yaml"),
            """\
            properties:
              referenced:
                $ref: sub.yaml
            """,
        )

        result = resolve_schema(self._path("top.yaml"))
        inlined = result["properties"]["referenced"]
        self.assertNotIn("$schema", inlined)
        self.assertNotIn("$id", inlined)
        self.assertEqual(inlined["node_type"], "section")
        self.assertEqual(inlined["properties"]["k"], {"type": "string"})

    def test_absolute_url_ref_is_rejected(self):
        from tasks.schema.merge_schema import resolve_schema

        _write(
            self._path("top.yaml"),
            """\
            properties:
              external:
                $ref: https://example.com/schema.json
            """,
        )

        with self.assertRaises(ValueError):
            resolve_schema(self._path("top.yaml"))


if __name__ == "__main__":
    unittest.main()
