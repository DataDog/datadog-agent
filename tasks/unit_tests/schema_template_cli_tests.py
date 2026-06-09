import os
import tempfile
import unittest

import yaml


def _sample_core_schema():
    """Minimal enriched-schema shape sufficient to exercise generate_template.

    Mirrors the shape produced by `dda inv schema.generate` for the core
    agent: a top-level `properties` dict whose entries each carry the
    enrichment fields (visibility, description, tags, default, type).
    Two settings is enough to verify both the per-entry rendering path
    and the ordering driven by `properties` iteration.
    """
    return {
        "properties": {
            "api_key": {
                "node_type": "setting",
                "type": "string",
                "default": "",
                "description": "Your Datadog API key",
                "visibility": "public",
                "tags": [],
            },
            "site": {
                "node_type": "setting",
                "type": "string",
                "default": "datadoghq.com",
                "description": "The Datadog site URL",
                "visibility": "public",
                "tags": [],
            },
        }
    }


def _sample_sysprobe_schema():
    return {
        "properties": {
            "network_config": {
                "node_type": "section",
                "type": "object",
                "description": "Network monitoring configuration",
                "visibility": "public",
                "tags": [],
                "properties": {
                    "enabled": {
                        "node_type": "setting",
                        "type": "boolean",
                        "default": False,
                        "description": "Enable network monitoring",
                        "visibility": "public",
                        "tags": [],
                    },
                },
            },
        }
    }


class TestSchemaTemplateCLI(unittest.TestCase):
    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name

    def tearDown(self):
        self._tempdir.cleanup()

    def _write_schema(self, schema, name="schema.yaml"):
        path = os.path.join(self.dir, name)
        with open(path, "w") as f:
            yaml.safe_dump(schema, f, sort_keys=False)
        return path

    def test_cli_main_writes_output(self):
        from tasks.schema.template import main as cli_main

        schema_path = self._write_schema(_sample_core_schema())
        out_path = os.path.join(self.dir, "out.yaml")

        rc = cli_main(["template.py", schema_path, "agent-py3", "linux", out_path])

        self.assertEqual(rc, 0)
        self.assertTrue(os.path.isfile(out_path))
        with open(out_path) as f:
            content = f.read()
        self.assertIn("api_key", content)
        self.assertIn("site", content)
        self.assertIn("datadoghq.com", content)

    def test_cli_main_system_probe_build_type(self):
        from tasks.schema.template import main as cli_main

        schema_path = self._write_schema(_sample_sysprobe_schema())
        out_path = os.path.join(self.dir, "out.yaml")

        rc = cli_main(["template.py", schema_path, "system-probe", "linux", out_path])

        self.assertEqual(rc, 0)
        with open(out_path) as f:
            content = f.read()
        self.assertIn("network_config", content)
        self.assertIn("enabled", content)

    def test_cli_main_rejects_invalid_build_type(self):
        from tasks.schema.template import main as cli_main

        schema_path = self._write_schema(_sample_core_schema())
        out_path = os.path.join(self.dir, "out.yaml")

        rc = cli_main(["template.py", schema_path, "bogus-build", "linux", out_path])

        self.assertNotEqual(rc, 0)

    def test_cli_main_rejects_invalid_os_target(self):
        from tasks.schema.template import main as cli_main

        schema_path = self._write_schema(_sample_core_schema())
        out_path = os.path.join(self.dir, "out.yaml")

        rc = cli_main(["template.py", schema_path, "agent-py3", "bogus-os", out_path])

        self.assertNotEqual(rc, 0)

    def test_cli_main_rejects_missing_schema(self):
        from tasks.schema.template import main as cli_main

        out_path = os.path.join(self.dir, "out.yaml")
        missing = os.path.join(self.dir, "does-not-exist.yaml")

        rc = cli_main(["template.py", missing, "agent-py3", "linux", out_path])

        self.assertNotEqual(rc, 0)

    def test_cli_main_rejects_wrong_argv_count(self):
        from tasks.schema.template import main as cli_main

        rc = cli_main(["template.py", "only-one-arg"])

        self.assertNotEqual(rc, 0)


if __name__ == "__main__":
    unittest.main()
