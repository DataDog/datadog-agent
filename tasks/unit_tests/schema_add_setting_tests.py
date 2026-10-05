import os
import tempfile
import textwrap
import unittest
from unittest.mock import patch

import yaml
from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.schema.add_setting import _resolve_target, add_setting


def _write(path, content):
    with open(path, "w") as f:
        f.write(textwrap.dedent(content))


class TestResolveTarget(unittest.TestCase):
    """_resolve_target must follow $refs, however deeply the splits nest."""

    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name
        self.top = self._path("core_schema.yaml")
        self.sysprobe = self._path("system-probe_schema.yaml")
        patcher = patch.multiple("tasks.schema.add_setting", CORE_SCHEMA=self.top, SYSPROBE_SCHEMA=self.sysprobe)
        patcher.start()
        self.addCleanup(patcher.stop)

    def tearDown(self):
        self._tempdir.cleanup()

    def _path(self, name):
        return os.path.join(self.dir, name)

    def _write_top(self, content):
        _write(self.top, content)

    def test_unsplit_path_targets_the_top_file(self):
        self._write_top(
            """\
            properties:
              proxy:
                node_type: section
                properties:
                  https:
                    node_type: setting
                    type: string
                    default: ''
            """
        )

        self.assertEqual(_resolve_target(["proxy", "http"], "core"), (self.top, ["proxy", "http"], []))

    def test_missing_intermediate_sections_stay_in_the_current_file(self):
        self._write_top(
            """\
            properties: {}
            """
        )

        self.assertEqual(_resolve_target(["brand", "new", "one"], "core"), (self.top, ["brand", "new", "one"], []))

    def test_single_ref_is_followed(self):
        self._write_top(
            """\
            properties:
              apm_config:
                $ref: apm_config.yaml
            """
        )
        sub = self._path("apm_config.yaml")
        _write(
            sub,
            """\
            node_type: section
            properties: {}
            """,
        )

        self.assertEqual(
            _resolve_target(["apm_config", "enabled"], "core"), (sub, ["enabled"], [("apm_config", sub, [])])
        )

    def test_nested_refs_are_followed(self):
        self._write_top(
            """\
            properties:
              outer:
                $ref: outer.yaml
            """
        )
        outer = self._path("outer.yaml")
        _write(
            outer,
            """\
            node_type: section
            properties:
              inner:
                $ref: inner.yaml
            """,
        )
        inner = self._path("inner.yaml")
        _write(
            inner,
            """\
            node_type: section
            properties: {}
            """,
        )

        self.assertEqual(
            _resolve_target(["outer", "inner", "deep", "leaf"], "core"),
            (inner, ["deep", "leaf"], [("outer", outer, []), ("outer.inner", inner, [])]),
        )

    def test_ref_nested_below_a_plain_section_is_followed(self):
        self._write_top(
            """\
            properties:
              network_devices:
                node_type: section
                properties:
                  snmp_traps:
                    $ref: snmp_traps.yaml
            """
        )
        sub = self._path("snmp_traps.yaml")
        _write(
            sub,
            """\
            node_type: section
            properties: {}
            """,
        )

        self.assertEqual(
            _resolve_target(["network_devices", "snmp_traps", "enabled"], "core"),
            (
                sub,
                ["enabled"],
                [("network_devices", self.top, ["network_devices"]), ("network_devices.snmp_traps", sub, [])],
            ),
        )

    def test_every_plain_section_walked_before_a_ref_is_reported(self):
        self._write_top(
            """\
            properties:
              network_devices:
                node_type: section
                properties:
                  snmp:
                    node_type: section
                    properties:
                      traps:
                        $ref: traps.yaml
            """
        )
        sub = self._path("traps.yaml")
        _write(
            sub,
            """\
            node_type: section
            properties: {}
            """,
        )

        self.assertEqual(
            _resolve_target(["network_devices", "snmp", "traps", "enabled"], "core"),
            (
                sub,
                ["enabled"],
                [
                    ("network_devices", self.top, ["network_devices"]),
                    ("network_devices.snmp", self.top, ["network_devices", "snmp"]),
                    ("network_devices.snmp.traps", sub, []),
                ],
            ),
        )

    def test_plain_sections_in_the_target_file_are_left_to_insert(self):
        # 'apm_config.obfuscation' lives in the target file, so it is part of
        # path_within_file and _insert reports it; only the file root is outer.
        self._write_top(
            """\
            properties:
              apm_config:
                $ref: apm_config.yaml
            """
        )
        sub = self._path("apm_config.yaml")
        _write(
            sub,
            """\
            node_type: section
            properties:
              obfuscation:
                node_type: section
                properties: {}
            """,
        )

        self.assertEqual(
            _resolve_target(["apm_config", "obfuscation", "enabled"], "core"),
            (sub, ["obfuscation", "enabled"], [("apm_config", sub, [])]),
        )

    def test_split_section_as_leaf_is_rejected(self):
        self._write_top(
            """\
            properties:
              apm_config:
                $ref: apm_config.yaml
            """
        )
        _write(
            self._path("apm_config.yaml"),
            """\
            node_type: section
            properties: {}
            """,
        )

        with self.assertRaises(Exit) as ctx:
            _resolve_target(["apm_config"], "core")
        self.assertIn("'apm_config' is a section", str(ctx.exception))

    def test_nested_split_section_as_leaf_is_rejected_with_its_full_path(self):
        self._write_top(
            """\
            properties:
              outer:
                $ref: outer.yaml
            """
        )
        _write(
            self._path("outer.yaml"),
            """\
            node_type: section
            properties:
              inner:
                $ref: inner.yaml
            """,
        )
        _write(
            self._path("inner.yaml"),
            """\
            node_type: section
            properties: {}
            """,
        )

        with self.assertRaises(Exit) as ctx:
            _resolve_target(["outer", "inner"], "core")
        self.assertIn("'outer.inner' is a section", str(ctx.exception))

    def test_missing_ref_target_is_reported(self):
        self._write_top(
            """\
            properties:
              apm_config:
                $ref: does_not_exist.yaml
            """
        )

        with self.assertRaises(Exit) as ctx:
            _resolve_target(["apm_config", "enabled"], "core")
        self.assertIn("does_not_exist.yaml", str(ctx.exception))

    def test_ref_cycle_is_reported(self):
        self._write_top(
            """\
            properties:
              outer:
                $ref: outer.yaml
            """
        )
        _write(
            self._path("outer.yaml"),
            """\
            node_type: section
            properties:
              inner:
                $ref: outer.yaml
            """,
        )

        with self.assertRaises(Exit) as ctx:
            _resolve_target(["outer", "inner", "leaf"], "core")
        self.assertIn("cycle", str(ctx.exception))

    def test_ref_with_siblings_is_not_a_split_section(self):
        # merge_schema only inlines a lone $ref, so neither does the walk: the
        # node is treated as a regular (malformed) child of the current file.
        self._write_top(
            """\
            properties:
              apm_config:
                $ref: apm_config.yaml
                node_type: section
            """
        )

        self.assertEqual(_resolve_target(["apm_config", "enabled"], "core"), (self.top, ["apm_config", "enabled"], []))

    def test_system_probe_schema_refs_are_followed(self):
        _write(
            self.sysprobe,
            """\
            properties:
              service_monitoring_config:
                $ref: system-probe-usm.yaml
            """,
        )
        sub = self._path("system-probe-usm.yaml")
        _write(
            sub,
            """\
            node_type: section
            properties: {}
            """,
        )

        self.assertEqual(
            _resolve_target(["service_monitoring_config", "enabled"], "system-probe"),
            (sub, ["enabled"], [("service_monitoring_config", sub, [])]),
        )


class TestAddSettingPublicAncestors(unittest.TestCase):
    """A public setting must leave every ancestor section public and described,
    in whichever file that section lives."""

    def setUp(self):
        self._tempdir = tempfile.TemporaryDirectory()
        self.dir = self._tempdir.name
        self.top = os.path.join(self.dir, "core_schema.yaml")
        self.sysprobe = os.path.join(self.dir, "system-probe_schema.yaml")
        patcher = patch.multiple("tasks.schema.add_setting", CORE_SCHEMA=self.top, SYSPROBE_SCHEMA=self.sysprobe)
        patcher.start()
        self.addCleanup(patcher.stop)

    def tearDown(self):
        self._tempdir.cleanup()

    def _load(self, path):
        with open(path) as f:
            return yaml.safe_load(f)

    def _run(self, answers):
        """Run add_setting with *answers* fed to every prompt, in order."""
        # The task lints the schema through ctx.run at the end; the schemas here
        # are stubs living outside SCHEMA_DIR, so stub the lint out as passing.
        ctx = MockContext(run={"dda inv schema.lint": Result("", exited=0)})
        with patch("builtins.input", side_effect=answers):
            add_setting(ctx)

    def test_plain_parent_of_a_split_section_is_made_public(self):
        # 'network_devices' is an ordinary section in the top file whose child
        # 'snmp_traps' is split out; adding a public setting under the split
        # section must make *both* public, and write the top file back.
        _write(
            self.top,
            """\
            properties:
              network_devices:
                node_type: section
                type: object
                properties:
                  snmp_traps:
                    $ref: snmp_traps.yaml
            """,
        )
        sub = os.path.join(self.dir, "snmp_traps.yaml")
        _write(
            sub,
            """\
            node_type: section
            type: object
            properties: {}
            """,
        )

        self._run(
            [
                "network_devices.snmp_traps.enabled",  # setting name
                "boolean",  # type
                "false",  # default
                "public",  # visibility
                "Enable SNMP traps.",  # description, then a blank line to end it
                "",
                "Network Devices Monitoring.",  # description of 'network_devices'
                "",
                "SNMP traps.",  # description of 'network_devices.snmp_traps'
                "",
            ]
        )

        parent = self._load(self.top)["properties"]["network_devices"]
        self.assertEqual(parent["visibility"], "public")
        self.assertEqual(parent["description"], "Network Devices Monitoring.")

        section = self._load(sub)
        self.assertEqual(section["visibility"], "public")
        self.assertEqual(section["description"], "SNMP traps.")
        self.assertEqual(section["properties"]["enabled"]["visibility"], "public")

    def test_already_public_ancestors_leave_their_file_untouched(self):
        _write(
            self.top,
            """\
            properties:
              network_devices:
                node_type: section
                type: object
                visibility: public
                description: Network Devices Monitoring.
                properties:
                  snmp_traps:
                    $ref: snmp_traps.yaml
            """,
        )
        sub = os.path.join(self.dir, "snmp_traps.yaml")
        _write(
            sub,
            """\
            node_type: section
            type: object
            visibility: public
            description: SNMP traps.
            properties: {}
            """,
        )
        before = open(self.top).read()

        self._run(
            [
                "network_devices.snmp_traps.enabled",
                "boolean",
                "false",
                "public",
                "Enable SNMP traps.",
                "",
            ]
        )

        self.assertEqual(open(self.top).read(), before)
        self.assertIn("enabled", self._load(sub)["properties"])


class TestResolveTargetAgainstRealSchema(unittest.TestCase):
    """Sanity checks against the schema files actually shipped in the repo."""

    def test_split_core_section(self):
        file_path, inner, enclosing = _resolve_target(["apm_config", "enabled"], "core")
        self.assertEqual(file_path, os.path.join("pkg", "config", "schema", "yaml", "apm_config.yaml"))
        self.assertEqual(inner, ["enabled"])
        self.assertEqual(enclosing, [("apm_config", file_path, [])])

    def test_unsplit_core_section(self):
        file_path, inner, enclosing = _resolve_target(["api_key"], "core")
        self.assertEqual(file_path, os.path.join("pkg", "config", "schema", "yaml", "core_schema.yaml"))
        self.assertEqual(inner, ["api_key"])
        self.assertEqual(enclosing, [])

    def test_split_system_probe_section(self):
        file_path, inner, enclosing = _resolve_target(["service_monitoring_config", "enabled"], "system-probe")
        self.assertEqual(file_path, os.path.join("pkg", "config", "schema", "yaml", "system-probe-usm.yaml"))
        self.assertEqual(inner, ["enabled"])
        self.assertEqual(enclosing, [("service_monitoring_config", file_path, [])])
