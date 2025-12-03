# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Unit tests for the CI merge utilities.
"""

from __future__ import annotations

from pathlib import Path
from textwrap import dedent

import pytest

from ci_utils.merge import deep_merge, extends_merge, resolve_extends, resolve_includes, resolve_references
from ci_utils.yaml import GitLabReference


class TestDeepMerge:
    """Tests for the deep_merge function (used for pipeline merging)."""

    def test_simple_merge(self):
        """Test simple dictionary merge."""
        base = {"a": 1, "b": 2}
        override = {"b": 3, "c": 4}
        result = deep_merge(base, override)
        assert result == {"a": 1, "b": 3, "c": 4}

    def test_nested_merge(self):
        """Test nested dictionary merge."""
        base = {"a": {"x": 1, "y": 2}}
        override = {"a": {"y": 3, "z": 4}}
        result = deep_merge(base, override)
        assert result == {"a": {"x": 1, "y": 3, "z": 4}}

    def test_list_concatenation(self):
        """Test list concatenation during merge (useful for stages)."""
        base = {"a": [1, 2]}
        override = {"a": [3, 4]}
        result = deep_merge(base, override)
        assert result == {"a": [1, 2, 3, 4]}

    def test_no_override(self):
        """Test that an error is raised if a key is present in both dictionaries and allow_override is False."""
        base = {"a": 1, "b": 2}
        override = {"b": 3, "c": 4}
        with pytest.raises(ValueError, match="Key b is present in both dictionaries, cannot merge"):
            deep_merge(base, override, allow_override=False)


class TestExtendsMerge:
    """Tests for extends_merge function (GitLab CI extends behavior)."""

    def test_simple_merge(self):
        """Test simple dictionary merge."""
        base = {"a": 1, "b": 2}
        override = {"b": 3, "c": 4}
        result = extends_merge(base, override)
        assert result == {"a": 1, "b": 3, "c": 4}

    def test_nested_dict_merge(self):
        """Test nested dictionary merge (e.g., variables)."""
        base = {"variables": {"VAR1": "a", "VAR2": "b"}}
        override = {"variables": {"VAR2": "c", "VAR3": "d"}}
        result = extends_merge(base, override)
        assert result == {"variables": {"VAR1": "a", "VAR2": "c", "VAR3": "d"}}

    def test_list_override(self):
        """Test that lists are overridden, not concatenated."""
        base = {"script": ["echo base1", "echo base2"]}
        override = {"script": ["echo override"]}
        result = extends_merge(base, override)
        # Job's script completely overrides template's script
        assert result == {"script": ["echo override"]}

    def test_before_script_override(self):
        """Test before_script is overridden."""
        base = {"before_script": ["setup1", "setup2"]}
        override = {"before_script": ["custom_setup"]}
        result = extends_merge(base, override)
        assert result == {"before_script": ["custom_setup"]}

    def test_rules_override(self):
        """Test rules are overridden."""
        base = {"rules": [{"if": "$CI"}]}
        override = {"rules": [{"if": "$CUSTOM", "when": "manual"}]}
        result = extends_merge(base, override)
        assert result == {"rules": [{"if": "$CUSTOM", "when": "manual"}]}

    def test_template_list_preserved_when_not_overridden(self):
        """Test that template lists are preserved when job doesn't override."""
        base = {"before_script": ["setup1", "setup2"]}
        override = {"script": ["echo hello"]}
        result = extends_merge(base, override)
        assert result["before_script"] == ["setup1", "setup2"]
        assert result["script"] == ["echo hello"]


class TestResolveExtends:
    """Tests for the resolve_extends function."""

    def test_no_extends(self):
        """Test that content without extends is unchanged."""
        content = {
            "variables": {"VAR1": "value1"},
            "stages": ["build", "test"],
            "my-job": {
                "stage": "build",
                "script": ["echo hello"],
            },
        }
        result = resolve_extends(content)
        assert result == content

    def test_simple_extends(self):
        """Test resolving a simple extends."""
        content = {
            ".template": {
                "image": "alpine:latest",
                "before_script": ["echo setup"],
            },
            "my-job": {
                "extends": ".template",
                "script": ["echo hello"],
            },
        }
        result = resolve_extends(content)

        assert result["my-job"]["image"] == "alpine:latest"
        assert result["my-job"]["before_script"] == ["echo setup"]
        assert result["my-job"]["script"] == ["echo hello"]
        assert "extends" not in result["my-job"]

    def test_extends_with_override(self):
        """Test that job config overrides template config."""
        content = {
            ".template": {
                "image": "alpine:latest",
                "stage": "build",
            },
            "my-job": {
                "extends": ".template",
                "image": "ubuntu:latest",  # Override
            },
        }
        result = resolve_extends(content)

        assert result["my-job"]["image"] == "ubuntu:latest"
        assert result["my-job"]["stage"] == "build"

    def test_multiple_extends(self):
        """Test extending multiple templates."""
        content = {
            ".base": {
                "image": "alpine:latest",
            },
            ".with-cache": {
                "cache": {"key": "test"},
            },
            "my-job": {
                "extends": [".base", ".with-cache"],
                "script": ["echo hello"],
            },
        }
        result = resolve_extends(content)

        assert result["my-job"]["image"] == "alpine:latest"
        assert result["my-job"]["cache"] == {"key": "test"}
        assert result["my-job"]["script"] == ["echo hello"]

    def test_nested_extends(self):
        """Test resolving nested extends (template extends another template)."""
        content = {
            ".base": {
                "image": "alpine:latest",
            },
            ".extended-base": {
                "extends": ".base",
                "before_script": ["echo setup"],
            },
            "my-job": {
                "extends": ".extended-base",
                "script": ["echo hello"],
            },
        }
        result = resolve_extends(content)

        assert result["my-job"]["image"] == "alpine:latest"
        assert result["my-job"]["before_script"] == ["echo setup"]
        assert result["my-job"]["script"] == ["echo hello"]

    def test_preserves_non_job_content(self):
        """Test that non-job content (variables, stages) is preserved."""
        content = {
            "variables": {"VAR1": "value1"},
            "stages": ["build", "test"],
            ".template": {
                "image": "alpine:latest",
            },
            "my-job": {
                "extends": ".template",
                "script": ["echo hello"],
            },
        }
        result = resolve_extends(content)

        assert result["variables"] == {"VAR1": "value1"}
        assert result["stages"] == ["build", "test"]

    def test_circular_extends_detection(self):
        """Test that circular extends are detected."""
        content = {
            ".a": {
                "extends": ".b",
            },
            ".b": {
                "extends": ".a",
            },
        }
        with pytest.raises(ValueError, match="Circular extends"):
            resolve_extends(content)

    def test_missing_template_ignored(self):
        """Test that missing templates are silently ignored."""
        content = {
            "my-job": {
                "extends": ".nonexistent",
                "script": ["echo hello"],
            },
        }
        with pytest.raises(ValueError, match="Template .nonexistent not found, cannot merge"):
            resolve_extends(content)

    def test_extends_with_variables(self):
        """Test that template variables are merged into job."""
        content = {
            ".template": {
                "variables": {
                    "VAR1": "from_template",
                    "VAR2": "from_template",
                },
            },
            "my-job": {
                "extends": ".template",
                "variables": {
                    "VAR2": "from_job",  # Override
                    "VAR3": "from_job",
                },
                "script": ["echo $VAR1 $VAR2 $VAR3"],
            },
        }
        result = resolve_extends(content)

        assert result["my-job"]["variables"]["VAR1"] == "from_template"
        assert result["my-job"]["variables"]["VAR2"] == "from_job"
        assert result["my-job"]["variables"]["VAR3"] == "from_job"

    def test_extends_script_override(self):
        """Test that job's script overrides template's script (not concatenated)."""
        content = {
            ".template": {
                "image": "alpine:latest",
                "script": ["echo template1", "echo template2"],
            },
            "my-job": {
                "extends": ".template",
                "script": ["echo job"],  # Should completely override
            },
        }
        result = resolve_extends(content)

        # Job's script should completely override template's script
        assert result["my-job"]["script"] == ["echo job"]
        assert result["my-job"]["image"] == "alpine:latest"

    def test_extends_before_script_inherited(self):
        """Test that before_script is inherited when not overridden."""
        content = {
            ".template": {
                "before_script": ["echo setup"],
            },
            "my-job": {
                "extends": ".template",
                "script": ["echo job"],
            },
        }
        result = resolve_extends(content)

        # before_script should be inherited from template
        assert result["my-job"]["before_script"] == ["echo setup"]
        assert result["my-job"]["script"] == ["echo job"]

    def test_extends_before_script_override(self):
        """Test that job's before_script overrides template's."""
        content = {
            ".template": {
                "before_script": ["echo template_setup1", "echo template_setup2"],
            },
            "my-job": {
                "extends": ".template",
                "before_script": ["echo job_setup"],  # Override
                "script": ["echo job"],
            },
        }
        result = resolve_extends(content)

        # Job's before_script should completely override template's
        assert result["my-job"]["before_script"] == ["echo job_setup"]


class TestResolveIncludes:
    """Tests for the resolve_includes function."""

    def test_no_includes(self, tmp_path: Path):
        """Test that content without includes is unchanged."""
        content = {
            "variables": {"VAR1": "value1"},
            "my-job": {"script": ["echo hello"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)
        assert result == content

    def test_simple_include(self, tmp_path: Path):
        """Test resolving a simple include."""
        # Create included file
        included_file = tmp_path / "common.yml"
        included_file.write_text(
            dedent("""
            .template:
              image: alpine:latest
        """)
        )

        content = {
            "include": ["common.yml"],
            "my-job": {"script": ["echo hello"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        assert ".template" in result
        assert result[".template"]["image"] == "alpine:latest"
        assert result["my-job"]["script"] == ["echo hello"]
        assert "include" not in result

    def test_include_with_local(self, tmp_path: Path):
        """Test resolving include with local: syntax."""
        included_file = tmp_path / "common.yml"
        included_file.write_text(
            dedent("""
            .template:
              image: alpine:latest
        """)
        )

        content = {
            "include": [{"local": "/common.yml"}],
            "my-job": {"script": ["echo hello"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        assert ".template" in result
        assert result[".template"]["image"] == "alpine:latest"

    def test_duplicate_includes_skipped(self, tmp_path: Path):
        """Test that the same file included multiple times is only processed once."""
        # Create common file with a needs list
        common_dir = tmp_path / "common"
        common_dir.mkdir()
        common_file = common_dir / "base.yml"
        common_file.write_text(
            dedent("""
            .base_template:
              needs:
                - dep1
                - dep2
        """)
        )

        # Create two files that both include the common file
        file_a = tmp_path / "file_a.yml"
        file_a.write_text(
            dedent("""
            include:
              - common/base.yml
            job_a:
              script: ["echo a"]
        """)
        )

        file_b = tmp_path / "file_b.yml"
        file_b.write_text(
            dedent("""
            include:
              - common/base.yml
            job_b:
              script: ["echo b"]
        """)
        )

        # Main file that includes both
        content = {
            "include": ["file_a.yml", "file_b.yml"],
            "my-job": {"script": ["echo hello"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        # The needs list should NOT be duplicated
        assert result[".base_template"]["needs"] == ["dep1", "dep2"]
        assert "job_a" in result
        assert "job_b" in result

    def test_duplicate_includes_no_duplicated_lists(self, tmp_path: Path):
        """Test that lists from duplicate includes are not concatenated multiple times."""
        # This is the specific bug case from .gitlab/common/macos.yml
        common_dir = tmp_path / "common"
        common_dir.mkdir()
        common_file = common_dir / "macos.yml"
        common_file.write_text(
            dedent("""
            .macos_gitlab:
              needs:
                - go_deps
                - go_tools_deps
              before_script:
                - setup command
        """)
        )

        # lint/macos.yml includes common/macos.yml
        lint_dir = tmp_path / "lint"
        lint_dir.mkdir()
        lint_file = lint_dir / "macos.yml"
        lint_file.write_text(
            dedent("""
            include:
              - common/macos.yml
            .lint_macos_gitlab:
              extends: .macos_gitlab
              stage: lint
        """)
        )

        # source_test/macos.yml also includes common/macos.yml
        source_test_dir = tmp_path / "source_test"
        source_test_dir.mkdir()
        source_test_file = source_test_dir / "macos.yml"
        source_test_file.write_text(
            dedent("""
            include:
              - common/macos.yml
            .source_test_macos_gitlab:
              extends: .macos_gitlab
              stage: test
        """)
        )

        # Main file that includes both
        content = {
            "include": ["lint/macos.yml", "source_test/macos.yml"],
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        # The needs list should NOT be duplicated
        assert result[".macos_gitlab"]["needs"] == ["go_deps", "go_tools_deps"]
        assert len(result[".macos_gitlab"]["needs"]) == 2  # Explicitly check length

    def test_nested_includes(self, tmp_path: Path):
        """Test resolving nested includes (include file that includes another)."""
        # Level 2: deepest file
        level2_file = tmp_path / "level2.yml"
        level2_file.write_text(
            dedent("""
            .deep_template:
              image: alpine:latest
        """)
        )

        # Level 1: includes level 2
        level1_file = tmp_path / "level1.yml"
        level1_file.write_text(
            dedent("""
            include:
              - level2.yml
            .mid_template:
              stage: build
        """)
        )

        # Main content includes level 1
        content = {
            "include": ["level1.yml"],
            "my-job": {"script": ["echo hello"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        assert ".deep_template" in result
        assert ".mid_template" in result
        assert result[".deep_template"]["image"] == "alpine:latest"

    def test_glob_pattern_includes(self, tmp_path: Path):
        """Test resolving includes with glob patterns."""
        # Create multiple files matching a pattern
        config_dir = tmp_path / "configs"
        config_dir.mkdir()

        (config_dir / "job1.yml").write_text(
            dedent("""
            job1:
              script: ["echo job1"]
        """)
        )

        (config_dir / "job2.yml").write_text(
            dedent("""
            job2:
              script: ["echo job2"]
        """)
        )

        content = {
            "include": ["configs/*.yml"],
            "main-job": {"script": ["echo main"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        assert "job1" in result
        assert "job2" in result
        assert "main-job" in result

    def test_main_content_takes_precedence(self, tmp_path: Path):
        """Test that main content overrides included content."""
        included_file = tmp_path / "common.yml"
        included_file.write_text(
            dedent("""
            my-job:
              image: from_include
              stage: build
        """)
        )

        content = {
            "include": ["common.yml"],
            "my-job": {
                "image": "from_main",  # Override
                "script": ["echo hello"],
            },
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        # Main content should override included content
        assert result["my-job"]["image"] == "from_main"
        # But included stage should be merged
        assert result["my-job"]["stage"] == "build"

    def test_remote_includes_skipped(self, tmp_path: Path):
        """Test that remote includes are skipped (not processed)."""
        content = {
            "include": [
                {"project": "other/repo", "file": "template.yml"},
                {"remote": "https://example.com/template.yml"},
            ],
            "my-job": {"script": ["echo hello"]},
        }
        result = resolve_includes(content, tmp_path, tmp_path)

        # Only the main content should be present
        assert result == {"my-job": {"script": ["echo hello"]}}

    def test_missing_include_file_raises_error(self, tmp_path: Path):
        """Test that missing include files raise an error (consistent with GitLab CI)."""
        content = {
            "include": ["nonexistent.yml"],
            "my-job": {"script": ["echo hello"]},
        }
        with pytest.raises(FileNotFoundError, match="nonexistent.yml"):
            resolve_includes(content, tmp_path, tmp_path)


class TestResolveReferences:
    """Tests for the resolve_references function (!reference tag resolution)."""

    def test_no_references(self):
        """Test that content without references is unchanged."""
        content = {
            ".template": {
                "script": ["echo hello"],
            },
            "my-job": {
                "script": ["echo world"],
            },
        }
        result = resolve_references(content)
        assert result == content

    def test_simple_reference(self):
        """Test resolving a simple !reference tag."""
        content = {
            ".template": {
                "script": ["echo hello", "echo world"],
            },
            "my-job": {
                "script": [GitLabReference([".template", "script"])],
            },
        }
        result = resolve_references(content)

        # The reference should be replaced with the actual value
        assert result["my-job"]["script"] == ["echo hello", "echo world"]

    def test_reference_in_list_flattened(self):
        """Test that a reference to a list in a list is flattened."""
        content = {
            ".template": {
                "script": ["echo template1", "echo template2"],
            },
            "my-job": {
                "script": [
                    "echo start",
                    GitLabReference([".template", "script"]),
                    "echo end",
                ],
            },
        }
        result = resolve_references(content)

        # The list from the reference should be flattened into the parent list
        assert result["my-job"]["script"] == [
            "echo start",
            "echo template1",
            "echo template2",
            "echo end",
        ]

    def test_reference_to_scalar(self):
        """Test resolving a reference to a scalar value."""
        content = {
            ".template": {
                "image": "alpine:latest",
            },
            "my-job": {
                "image": GitLabReference([".template", "image"]),
                "script": ["echo hello"],
            },
        }
        result = resolve_references(content)

        assert result["my-job"]["image"] == "alpine:latest"

    def test_reference_to_dict(self):
        """Test resolving a reference to a dictionary."""
        content = {
            ".template": {
                "variables": {
                    "VAR1": "value1",
                    "VAR2": "value2",
                },
            },
            "my-job": {
                "variables": GitLabReference([".template", "variables"]),
                "script": ["echo hello"],
            },
        }
        result = resolve_references(content)

        assert result["my-job"]["variables"] == {"VAR1": "value1", "VAR2": "value2"}

    def test_nested_reference(self):
        """Test resolving a reference inside a nested structure."""
        content = {
            ".template": {
                "cache": {
                    "key": "my-key",
                    "paths": ["node_modules/"],
                },
            },
            "my-job": {
                "cache": {
                    "key": GitLabReference([".template", "cache", "key"]),
                    "paths": GitLabReference([".template", "cache", "paths"]),
                },
                "script": ["echo hello"],
            },
        }
        result = resolve_references(content)

        assert result["my-job"]["cache"]["key"] == "my-key"
        assert result["my-job"]["cache"]["paths"] == ["node_modules/"]

    def test_reference_to_before_script(self):
        """Test resolving a reference to before_script (common GitLab CI pattern)."""
        content = {
            ".setup": {
                "before_script": ["apt-get update", "apt-get install -y curl"],
            },
            "my-job": {
                "before_script": [
                    GitLabReference([".setup", "before_script"]),
                    "echo 'Custom setup'",
                ],
                "script": ["echo hello"],
            },
        }
        result = resolve_references(content)

        # before_script should have the setup commands flattened, then the custom one
        assert result["my-job"]["before_script"] == [
            "apt-get update",
            "apt-get install -y curl",
            "echo 'Custom setup'",
        ]

    def test_multiple_references_in_same_list(self):
        """Test multiple references in the same list."""
        content = {
            ".setup": {
                "commands": ["setup1", "setup2"],
            },
            ".teardown": {
                "commands": ["teardown1", "teardown2"],
            },
            "my-job": {
                "script": [
                    GitLabReference([".setup", "commands"]),
                    "echo 'main'",
                    GitLabReference([".teardown", "commands"]),
                ],
            },
        }
        result = resolve_references(content)

        assert result["my-job"]["script"] == [
            "setup1",
            "setup2",
            "echo 'main'",
            "teardown1",
            "teardown2",
        ]

    def test_reference_not_found_preserved(self):
        """Test that references to non-existent paths are preserved."""
        content = {
            "my-job": {
                "script": [GitLabReference([".nonexistent", "script"])],
            },
        }
        result = resolve_references(content)

        # The reference should be preserved (GitLab will handle/error at runtime)
        assert isinstance(result["my-job"]["script"][0], GitLabReference)

    def test_chained_references(self):
        """Test resolving references that point to other references."""
        content = {
            ".base": {
                "script": ["echo base"],
            },
            ".intermediate": {
                "script": [GitLabReference([".base", "script"])],
            },
            "my-job": {
                "script": [GitLabReference([".intermediate", "script"])],
            },
        }
        result = resolve_references(content)

        # The chained references should all be resolved
        assert result["my-job"]["script"] == ["echo base"]

    def test_reference_in_rules(self):
        """Test resolving references in rules (another common pattern)."""
        content = {
            ".common_rules": {
                "rules": [
                    {"if": "$CI_COMMIT_BRANCH == 'main'"},
                    {"when": "manual"},
                ],
            },
            "my-job": {
                "rules": GitLabReference([".common_rules", "rules"]),
                "script": ["echo hello"],
            },
        }
        result = resolve_references(content)

        assert result["my-job"]["rules"] == [
            {"if": "$CI_COMMIT_BRANCH == 'main'"},
            {"when": "manual"},
        ]
