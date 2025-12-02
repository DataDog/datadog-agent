# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Unit tests for the CI merge utilities.
"""

from __future__ import annotations

import pytest

from ci.merge import deep_merge, extends_merge, resolve_extends


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
        result = resolve_extends(content)

        # Job should still exist with its own config
        assert result["my-job"]["script"] == ["echo hello"]
        assert "extends" not in result["my-job"]

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
