# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Unit tests for the CI YAML utilities.
"""

from __future__ import annotations

from pathlib import Path

import yaml

from ci_utils.yaml import GitLabReference, NoAnchorDumper, dump_yaml, load_yaml


class TestGitLabReference:
    """Tests for the GitLabReference class."""

    def test_repr(self):
        """Test string representation."""
        ref = GitLabReference([".template", "script"])
        assert repr(ref) == "!reference ['.template', 'script']"

    def test_value_stored(self):
        """Test that value is stored correctly."""
        ref = GitLabReference([".template", "script", "0"])
        assert ref.value == [".template", "script", "0"]


class TestLoadYaml:
    """Tests for the load_yaml function."""

    def test_load_simple_yaml(self, tmp_path: Path):
        """Test loading a simple YAML file."""
        yaml_file = tmp_path / "test.yml"
        yaml_file.write_text("key: value\nlist:\n  - item1\n  - item2\n")

        result = load_yaml(yaml_file)

        assert result == {"key": "value", "list": ["item1", "item2"]}

    def test_load_yaml_with_reference(self, tmp_path: Path):
        """Test loading a YAML file with !reference tag."""
        yaml_file = tmp_path / "test.yml"
        yaml_file.write_text(
            """.template:
  script:
    - echo hello

my-job:
  script:
    - !reference [.template, script]
"""
        )

        result = load_yaml(yaml_file)

        assert ".template" in result
        assert "my-job" in result
        # The !reference should be loaded as GitLabReference
        assert isinstance(result["my-job"]["script"][0], GitLabReference)
        assert result["my-job"]["script"][0].value == [".template", "script"]

    def test_load_empty_yaml(self, tmp_path: Path):
        """Test loading an empty YAML file returns empty dict."""
        yaml_file = tmp_path / "empty.yml"
        yaml_file.write_text("")

        result = load_yaml(yaml_file)

        assert result == {}


class TestNoAnchorDumper:
    """Tests for the NoAnchorDumper class (removes YAML anchors)."""

    def test_no_anchors_for_shared_dict(self):
        """Test that shared dictionaries don't create anchors."""
        shared_dict = {"key": "value"}
        content = {
            "job1": {"config": shared_dict},
            "job2": {"config": shared_dict},  # Same object reference
        }

        output = yaml.dump(content, Dumper=NoAnchorDumper)

        # Should not contain any anchors (&) or aliases (*)
        assert "&" not in output
        assert "*" not in output
        # Both jobs should have the full config inlined
        assert output.count("key: value") == 2

    def test_no_anchors_for_shared_list(self):
        """Test that shared lists don't create anchors."""
        shared_list = ["item1", "item2"]
        content = {
            "job1": {"items": shared_list},
            "job2": {"items": shared_list},
        }

        output = yaml.dump(content, Dumper=NoAnchorDumper)

        assert "&" not in output
        assert "*" not in output
        # Both jobs should have the full list inlined
        assert output.count("- item1") == 2
        assert output.count("- item2") == 2

    def test_no_anchors_for_nested_shared_objects(self):
        """Test that nested shared objects don't create anchors."""
        shared_nested = {"nested": {"deep": "value"}}
        content = {
            "job1": {"config": shared_nested},
            "job2": {"config": shared_nested},
            "job3": {"other": shared_nested["nested"]},  # Reference to nested part
        }

        output = yaml.dump(content, Dumper=NoAnchorDumper)

        assert "&" not in output
        assert "*" not in output

    def test_gitlab_reference_preserved(self):
        """Test that GitLabReference is preserved when dumping."""
        content = {
            "my-job": {
                "script": [GitLabReference([".template", "script"])],
            },
        }

        output = yaml.dump(content, Dumper=NoAnchorDumper)

        assert "!reference" in output
        assert ".template" in output


class TestDumpYaml:
    """Tests for the dump_yaml function."""

    def test_dump_simple_yaml(self, tmp_path: Path):
        """Test dumping simple content."""
        output_file = tmp_path / "output.yml"
        content = {"key": "value", "list": ["item1", "item2"]}

        dump_yaml(content, output_file)

        result = output_file.read_text()
        assert "key: value" in result
        assert "- item1" in result
        assert "- item2" in result

    def test_dump_with_header(self, tmp_path: Path):
        """Test dumping with a header comment."""
        output_file = tmp_path / "output.yml"
        content = {"key": "value"}

        dump_yaml(content, output_file, header="Generated file\nDo not edit")

        result = output_file.read_text()
        assert "# Generated file" in result
        assert "# Do not edit" in result
        assert "---" in result  # YAML document separator after header

    def test_dump_removes_anchors(self, tmp_path: Path):
        """Test that dump_yaml removes anchors."""
        output_file = tmp_path / "output.yml"
        shared_dict = {"key": "value"}
        content = {
            "job1": {"config": shared_dict},
            "job2": {"config": shared_dict},
        }

        dump_yaml(content, output_file)

        result = output_file.read_text()
        assert "&" not in result
        assert "*" not in result
        assert result.count("key: value") == 2

    def test_dump_preserves_order(self, tmp_path: Path):
        """Test that key order is preserved."""
        output_file = tmp_path / "output.yml"
        content = {"z_key": "z", "a_key": "a", "m_key": "m"}

        dump_yaml(content, output_file)

        result = output_file.read_text()
        lines = [line for line in result.split("\n") if line and not line.startswith("#") and line != "---"]
        # Keys should be in insertion order, not sorted
        assert lines[0] == "z_key: z"
        assert lines[1] == "a_key: a"
        assert lines[2] == "m_key: m"

    def test_dump_gitlab_reference(self, tmp_path: Path):
        """Test that GitLabReference is dumped correctly."""
        output_file = tmp_path / "output.yml"
        content = {
            "my-job": {
                "script": [GitLabReference([".template", "script"])],
            },
        }

        dump_yaml(content, output_file)

        result = output_file.read_text()
        assert "!reference" in result
        # Reload and verify
        loaded = load_yaml(output_file)
        assert isinstance(loaded["my-job"]["script"][0], GitLabReference)


class TestAnchorRemovalIntegration:
    """Integration tests for anchor removal in realistic scenarios."""

    def test_gitlab_ci_style_templates(self, tmp_path: Path):
        """Test anchor removal with GitLab CI style templates."""
        output_file = tmp_path / "output.yml"

        # Simulate common GitLab CI pattern where templates are reused
        template_vars = {"VAR1": "value1", "VAR2": "value2"}
        template_rules = [{"if": "$CI_COMMIT_BRANCH == 'main'"}]

        content = {
            ".template": {
                "variables": template_vars,
                "rules": template_rules,
            },
            "job1": {
                "extends": ".template",
                "variables": template_vars,  # Same reference
                "rules": template_rules,  # Same reference
                "script": ["echo job1"],
            },
            "job2": {
                "extends": ".template",
                "variables": template_vars,  # Same reference
                "rules": template_rules,  # Same reference
                "script": ["echo job2"],
            },
        }

        dump_yaml(content, output_file)

        result = output_file.read_text()

        # No anchors or aliases
        assert "&" not in result
        assert "*" not in result

        # All values should be inlined
        assert result.count("VAR1: value1") == 3
        assert result.count("VAR2: value2") == 3

    def test_extends_merge_result_no_anchors(self, tmp_path: Path):
        """Test that the result of extends merge has no anchors when dumped."""
        from ci_utils.merge import resolve_extends

        output_file = tmp_path / "output.yml"

        content = {
            ".template": {
                "image": "alpine:latest",
                "variables": {"VAR1": "value1"},
            },
            "job1": {
                "extends": ".template",
                "script": ["echo job1"],
            },
            "job2": {
                "extends": ".template",
                "script": ["echo job2"],
            },
        }

        # Resolve extends (this may create shared references)
        resolved = resolve_extends(content)

        # Dump and verify no anchors
        dump_yaml(resolved, output_file)

        result = output_file.read_text()
        assert "&" not in result
        assert "*" not in result
