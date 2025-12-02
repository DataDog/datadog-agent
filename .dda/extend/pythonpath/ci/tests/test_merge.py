# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Unit tests for the CI merge utilities.
"""

from __future__ import annotations

import pytest

from ci.merge import apply_job_injections, deep_merge, extends_merge, resolve_extends


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


class TestApplyJobInjections:
    """Tests for the apply_job_injections function."""

    def test_inject_before_script(self):
        """Test injecting before_script to all jobs."""
        content = {
            "variables": {"VAR1": "value"},
            "stages": ["build"],
            "job1": {
                "script": ["echo job1"],
            },
            "job2": {
                "before_script": ["echo existing"],
                "script": ["echo job2"],
            },
        }
        result = apply_job_injections(
            content,
            before_script=["echo global_setup"],
        )

        # Job without before_script gets injected
        assert result["job1"]["before_script"] == ["echo global_setup"]
        # Job with before_script gets injected at the beginning
        assert result["job2"]["before_script"] == ["echo global_setup", "echo existing"]
        # Non-job content unchanged
        assert result["variables"] == {"VAR1": "value"}

    def test_inject_after_script(self):
        """Test injecting after_script to all jobs."""
        content = {
            "job1": {
                "script": ["echo job1"],
            },
            "job2": {
                "after_script": ["echo cleanup"],
                "script": ["echo job2"],
            },
        }
        result = apply_job_injections(
            content,
            after_script=["echo global_cleanup"],
        )

        assert result["job1"]["after_script"] == ["echo global_cleanup"]
        assert result["job2"]["after_script"] == ["echo cleanup", "echo global_cleanup"]

    def test_inject_needs(self):
        """Test injecting needs to all jobs."""
        content = {
            "job1": {
                "script": ["echo job1"],
            },
            "job2": {
                "needs": ["existing-dep"],
                "script": ["echo job2"],
            },
        }
        result = apply_job_injections(
            content,
            needs=["global-setup"],
        )

        assert result["job1"]["needs"] == ["global-setup"]
        assert result["job2"]["needs"] == ["global-setup", "existing-dep"]

    def test_inject_needs_no_duplicates(self):
        """Test that duplicate needs are not added."""
        content = {
            "job1": {
                "needs": ["global-setup", "other-dep"],
                "script": ["echo job1"],
            },
        }
        result = apply_job_injections(
            content,
            needs=["global-setup"],
        )

        # Should not duplicate global-setup
        assert result["job1"]["needs"] == ["global-setup", "other-dep"]

    def test_inject_variables(self):
        """Test injecting variables to all jobs."""
        content = {
            "job1": {
                "script": ["echo job1"],
            },
            "job2": {
                "variables": {
                    "JOB_VAR": "job_value",
                    "OVERRIDE_ME": "job_override",
                },
                "script": ["echo job2"],
            },
        }
        result = apply_job_injections(
            content,
            variables={"GLOBAL_VAR": "global", "OVERRIDE_ME": "global_value"},
        )

        # Job without variables gets injected
        assert result["job1"]["variables"] == {"GLOBAL_VAR": "global", "OVERRIDE_ME": "global_value"}
        # Job with variables: job's own variables take precedence
        assert result["job2"]["variables"]["GLOBAL_VAR"] == "global"
        assert result["job2"]["variables"]["JOB_VAR"] == "job_value"
        assert result["job2"]["variables"]["OVERRIDE_ME"] == "job_override"

    def test_inject_tags(self):
        """Test injecting tags to all jobs."""
        content = {
            "job1": {
                "script": ["echo job1"],
            },
            "job2": {
                "tags": ["runner-a"],
                "script": ["echo job2"],
            },
        }
        result = apply_job_injections(
            content,
            tags=["global-tag"],
        )

        assert result["job1"]["tags"] == ["global-tag"]
        assert result["job2"]["tags"] == ["global-tag", "runner-a"]

    def test_skip_hidden_templates(self):
        """Test that hidden templates (starting with .) are not injected."""
        content = {
            ".template": {
                "script": ["echo template"],
            },
            "job1": {
                "extends": ".template",
                "script": ["echo job1"],
            },
        }
        result = apply_job_injections(
            content,
            before_script=["echo setup"],
        )

        # Template should not have injection
        assert "before_script" not in result[".template"]
        # Job should have injection
        assert result["job1"]["before_script"] == ["echo setup"]

    def test_skip_special_keys(self):
        """Test that special top-level keys are not treated as jobs."""
        content = {
            "variables": {"VAR1": "value"},
            "stages": ["build", "test"],
            "default": {"image": "alpine"},
            "workflow": {"rules": []},
            "job1": {
                "script": ["echo job1"],
            },
        }
        result = apply_job_injections(
            content,
            before_script=["echo setup"],
        )

        # Special keys unchanged
        assert result["variables"] == {"VAR1": "value"}
        assert result["stages"] == ["build", "test"]
        assert result["default"] == {"image": "alpine"}
        # Job gets injection
        assert result["job1"]["before_script"] == ["echo setup"]

    def test_inject_multiple(self):
        """Test injecting multiple things at once."""
        content = {
            "job1": {
                "script": ["echo job1"],
            },
        }
        result = apply_job_injections(
            content,
            before_script=["echo setup"],
            after_script=["echo cleanup"],
            needs=["init-job"],
            variables={"GLOBAL": "value"},
            tags=["runner"],
        )

        assert result["job1"]["before_script"] == ["echo setup"]
        assert result["job1"]["after_script"] == ["echo cleanup"]
        assert result["job1"]["needs"] == ["init-job"]
        assert result["job1"]["variables"] == {"GLOBAL": "value"}
        assert result["job1"]["tags"] == ["runner"]

    def test_skip_trigger_jobs_for_scripts(self):
        """Test that trigger jobs don't get before_script, after_script, or tags."""
        content = {
            "regular-job": {
                "script": ["echo job"],
            },
            "trigger-job": {
                "trigger": {
                    "include": "child-pipeline.yml",
                },
            },
        }
        result = apply_job_injections(
            content,
            before_script=["echo setup"],
            after_script=["echo cleanup"],
            needs=["init-job"],
            variables={"GLOBAL": "value"},
            tags=["runner"],
        )

        # Regular job gets all injections
        assert result["regular-job"]["before_script"] == ["echo setup"]
        assert result["regular-job"]["after_script"] == ["echo cleanup"]
        assert result["regular-job"]["needs"] == ["init-job"]
        assert result["regular-job"]["variables"] == {"GLOBAL": "value"}
        assert result["regular-job"]["tags"] == ["runner"]

        # Trigger job only gets needs and variables (allowed)
        assert "before_script" not in result["trigger-job"]
        assert "after_script" not in result["trigger-job"]
        assert "tags" not in result["trigger-job"]
        assert result["trigger-job"]["needs"] == ["init-job"]
        assert result["trigger-job"]["variables"] == {"GLOBAL": "value"}
