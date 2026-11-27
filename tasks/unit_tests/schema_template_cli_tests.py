import os
import tempfile
import unittest

import yaml


def _sample_core_schema():
    """Minimal enriched-schema shape sufficient to exercise generate_template.

    Mirrors the shape from the schema for the core
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

        rc = cli_main(["template.py", schema_path, "datadog-agent", "linux", out_path])

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

        rc = cli_main(["template.py", schema_path, "datadog-agent", "bogus-os", out_path])

        self.assertNotEqual(rc, 0)

    def test_cli_main_rejects_missing_schema(self):
        from tasks.schema.template import main as cli_main

        out_path = os.path.join(self.dir, "out.yaml")
        missing = os.path.join(self.dir, "does-not-exist.yaml")

        rc = cli_main(["template.py", missing, "datadog-agent", "linux", out_path])

        self.assertNotEqual(rc, 0)

    def test_cli_main_rejects_wrong_argv_count(self):
        from tasks.schema.template import main as cli_main

        rc = cli_main(["template.py", "only-one-arg"])

        self.assertNotEqual(rc, 0)


class TestTemplateBanners(unittest.TestCase):
    @staticmethod
    def _setting(**extra):
        return {
            "node_type": "setting",
            "type": "string",
            "default": "",
            "description": "A setting",
            "visibility": "public",
            **extra,
        }

    def _render(self, properties):
        from tasks.schema.template import generate_template

        with tempfile.TemporaryDirectory() as tmp:
            schema_path = os.path.join(tmp, "schema.yaml")
            with open(schema_path, "w") as f:
                yaml.safe_dump({"properties": properties}, f, sort_keys=False)
            out_path = os.path.join(tmp, "out.yaml")
            generate_template(schema_path, out_path, "datadog-agent", "linux")
            with open(out_path) as f:
                return f.read()

    @staticmethod
    def _banner(text):
        outline = "#" * (len(text) + 6)
        return f"{outline}\n## {text} ##\n{outline}\n\n"

    def test_setting_banner_is_rendered_before_the_setting(self):
        content = self._render({"api_key": self._setting(banner="Basic Configuration")})
        self.assertIn(self._banner("Basic Configuration") + "## @param api_key", content)

    def test_section_title_is_rendered_as_banner(self):
        section = {
            "node_type": "section",
            "title": "Trace Collection Configuration",
            "type": "object",
            "visibility": "public",
            "description": "A section",
            "properties": {"enabled": self._setting()},
        }
        content = self._render({"apm_config": section})
        self.assertIn(self._banner("Trace Collection Configuration") + "## @param apm_config", content)

    def test_section_banner_takes_precedence_over_title(self):
        section = {
            "node_type": "section",
            "title": "Label",
            "banner": "Banner",
            "type": "object",
            "visibility": "public",
            "description": "A section",
            "properties": {"enabled": self._setting()},
        }
        content = self._render({"apm_config": section})
        self.assertIn(self._banner("Banner"), content)
        self.assertNotIn("## Label ##", content)

    def test_setting_title_is_not_a_banner(self):
        content = self._render({"api_key": self._setting(title="Basic Configuration")})
        self.assertNotIn("## Basic Configuration ##", content)


if __name__ == "__main__":
    unittest.main()
