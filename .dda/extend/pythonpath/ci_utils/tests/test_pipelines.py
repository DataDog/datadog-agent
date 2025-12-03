# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Unit tests for the pipeline trigger matching logic.
"""

from __future__ import annotations

import tempfile
from pathlib import Path

from ci.pipelines import (
    ChangesTrigger,
    Pipeline,
    PipelinesConfig,
    _matches_pattern,
)


class TestMatchesPattern:
    """Tests for the _matches_pattern function."""

    def test_exact_match(self):
        """Test exact file path matching."""
        assert _matches_pattern("pkg/main.go", "pkg/main.go")
        assert not _matches_pattern("pkg/main.go", "pkg/other.go")

    def test_directory_prefix(self):
        """Test directory prefix matching."""
        assert _matches_pattern("standalone-repo/main.go", "standalone-repo")
        assert _matches_pattern("standalone-repo/sub/file.go", "standalone-repo")
        assert not _matches_pattern("pkg/main.go", "standalone-repo")

    def test_glob_star(self):
        """Test single star glob patterns."""
        assert _matches_pattern("pkg/main.go", "pkg/*.go")
        assert _matches_pattern("pkg/test.go", "pkg/*.go")
        assert not _matches_pattern("pkg/sub/main.go", "pkg/*.go")

    def test_glob_double_star(self):
        """Test double star glob patterns (recursive)."""
        assert _matches_pattern("standalone-repo/main.go", "standalone-repo/**/*")
        assert _matches_pattern("standalone-repo/sub/main.go", "standalone-repo/**/*")
        assert _matches_pattern("standalone-repo/a/b/c/main.go", "standalone-repo/**/*")
        assert not _matches_pattern("pkg/main.go", "standalone-repo/**/*")

    def test_glob_double_star_extension(self):
        """Test double star with file extension."""
        assert _matches_pattern("pkg/main.go", "**/*.go")
        assert _matches_pattern("pkg/sub/main.go", "**/*.go")
        assert not _matches_pattern("pkg/main.py", "**/*.go")

    def test_question_mark_glob(self):
        """Test question mark glob pattern."""
        assert _matches_pattern("file1.go", "file?.go")
        assert _matches_pattern("fileA.go", "file?.go")
        assert not _matches_pattern("file12.go", "file?.go")


class TestChangesTrigger:
    """Tests for the ChangesTrigger class."""

    def test_no_patterns_matches_everything(self):
        """Trigger with no patterns matches everything."""
        trigger = ChangesTrigger()
        assert trigger.matches("any/file.go")
        assert trigger.matches("standalone-repo/main.go")

    def test_include_only(self):
        """Trigger with only include patterns - filter mode."""
        trigger = ChangesTrigger(include=["standalone-repo/**/*"])
        assert trigger.matches("standalone-repo/main.go")
        assert trigger.matches("standalone-repo/sub/file.go")
        assert not trigger.matches("pkg/main.go")

    def test_all_except_only(self):
        """Trigger with only all_except patterns - match everything except these."""
        trigger = ChangesTrigger(all_except=["standalone-repo/**/*"])
        assert not trigger.matches("standalone-repo/main.go")
        assert not trigger.matches("standalone-repo/sub/file.go")
        assert trigger.matches("pkg/main.go")
        assert trigger.matches("cmd/agent/main.go")

    def test_include_overrides_all_except(self):
        """Include patterns should override all_except patterns."""
        trigger = ChangesTrigger(
            include=["standalone-repo/shared/**/*"],
            all_except=["standalone-repo/**/*"],
        )
        # File in shared/ matches include, should be included despite all_except
        assert trigger.matches("standalone-repo/shared/util.go")
        # File not in shared/ matches all_except, should be excluded
        assert not trigger.matches("standalone-repo/other/main.go")
        # File outside standalone-repo doesn't match include or all_except
        # Files not matching any pattern are included by default
        assert trigger.matches("pkg/main.go")

    def test_all_except_with_override_and_general_match(self):
        """Test the main pipeline use case: all_except folder but include subfolder."""
        # For main pipeline: match everything except standalone-repo, but include standalone-repo/shared
        trigger = ChangesTrigger(
            include=["standalone-repo/shared/**/*"],
            all_except=["standalone-repo/**/*"],
        )

        # standalone-repo/shared/util.go -> matches include -> True ✓
        assert trigger.matches("standalone-repo/shared/util.go")

        # standalone-repo/other/main.go -> doesn't match include -> matches all_except -> False ✓
        assert not trigger.matches("standalone-repo/other/main.go")

        # pkg/main.go -> doesn't match include -> doesn't match all_except -> True ✓
        assert trigger.matches("pkg/main.go")

    def test_multiple_include_patterns(self):
        """Test multiple include patterns."""
        trigger = ChangesTrigger(include=["pkg/**/*", "cmd/**/*"])
        assert trigger.matches("pkg/main.go")
        assert trigger.matches("cmd/agent/main.go")
        assert not trigger.matches("test/main.go")

    def test_multiple_all_except_patterns(self):
        """Test multiple all_except patterns."""
        trigger = ChangesTrigger(all_except=["vendor/**/*", "test/**/*"])
        assert not trigger.matches("vendor/lib/main.go")
        assert not trigger.matches("test/unit/main_test.go")
        assert trigger.matches("pkg/main.go")


class TestPipeline:
    """Tests for the Pipeline class."""

    def test_no_triggers_always_runs(self):
        """Pipeline with no triggers always runs."""
        pipeline = Pipeline(name="always")
        assert pipeline.should_trigger([])
        assert pipeline.should_trigger(["any/file.go"])

    def test_single_trigger(self):
        """Pipeline with a single trigger."""
        pipeline = Pipeline(
            name="standalone",
            triggers=[ChangesTrigger(include=["standalone-repo/**/*"])],
        )
        assert pipeline.should_trigger(["standalone-repo/main.go"])
        assert not pipeline.should_trigger(["pkg/main.go"])

    def test_all_except_trigger(self):
        """Pipeline with all_except trigger."""
        pipeline = Pipeline(
            name="main",
            triggers=[ChangesTrigger(all_except=["standalone-repo/**/*"])],
        )
        assert pipeline.should_trigger(["pkg/main.go"])
        assert not pipeline.should_trigger(["standalone-repo/main.go"])

    def test_mixed_files(self):
        """Pipeline triggers if any file matches."""
        pipeline = Pipeline(
            name="main",
            triggers=[ChangesTrigger(all_except=["standalone-repo/**/*"])],
        )
        # If at least one file matches, pipeline triggers
        assert pipeline.should_trigger(["pkg/main.go", "standalone-repo/main.go"])

    def test_no_matching_files(self):
        """Pipeline doesn't trigger if no files match."""
        pipeline = Pipeline(
            name="standalone",
            triggers=[ChangesTrigger(include=["standalone-repo/**/*"])],
        )
        assert not pipeline.should_trigger(["pkg/main.go", "cmd/main.go"])

    def test_empty_changed_files(self):
        """Pipeline with triggers but no changed files doesn't trigger."""
        pipeline = Pipeline(
            name="test",
            triggers=[ChangesTrigger(include=["test/**/*"])],
        )
        assert not pipeline.should_trigger([])


class TestPipelinesConfig:
    """Tests for loading PipelinesConfig from YAML."""

    def test_load_simple_config(self):
        """Test loading a simple pipeline config from legacy format."""
        yaml_content = """
pipelines:
  - name: main
    entrypoint: .gitlab-ci-main.yml
    on:
      - changes:
        - all_except:
          - standalone-repo/**/*

  - name: standalone-repo
    entrypoint: standalone-repo/.gitlab-ci.yml
    on:
      - changes:
        - include:
          - standalone-repo/**/*
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            config = PipelinesConfig.load(Path(f.name))

        assert len(config.pipelines) == 2

        main = config.pipelines[0]
        assert main.name == "main"
        assert main.entrypoint == ".gitlab-ci-main.yml"
        assert len(main.triggers) == 1
        assert main.triggers[0].all_except == ["standalone-repo/**/*"]
        assert main.triggers[0].include == []

        standalone = config.pipelines[1]
        assert standalone.name == "standalone-repo"
        assert standalone.entrypoint == "standalone-repo/.gitlab-ci.yml"
        assert len(standalone.triggers) == 1
        assert standalone.triggers[0].include == ["standalone-repo/**/*"]
        assert standalone.triggers[0].all_except == []

    def test_load_from_folder(self):
        """Test loading pipeline configs from a folder of individual files."""
        with tempfile.TemporaryDirectory() as tmpdir:
            folder = Path(tmpdir)

            # Create main.yml
            (folder / "main.yml").write_text("""
name: main
entrypoint: .gitlab-ci-main.yml
on:
  - changes:
    - all_except:
      - standalone-repo/**/*
""")

            # Create standalone-repo.yml
            (folder / "standalone-repo.yml").write_text("""
name: standalone-repo
entrypoint: standalone-repo/.gitlab-ci.yml
on:
  - changes:
    - include:
      - standalone-repo/**/*
""")

            config = PipelinesConfig.load_from_folder(folder)

        assert len(config.pipelines) == 2

        # Pipelines are sorted by filename
        main = next(p for p in config.pipelines if p.name == "main")
        assert main.entrypoint == ".gitlab-ci-main.yml"
        assert main.triggers[0].all_except == ["standalone-repo/**/*"]

        standalone = next(p for p in config.pipelines if p.name == "standalone-repo")
        assert standalone.entrypoint == "standalone-repo/.gitlab-ci.yml"
        assert standalone.triggers[0].include == ["standalone-repo/**/*"]

    def test_load_from_folder_uses_filename_as_name(self):
        """Test that filename is used as pipeline name if not specified."""
        with tempfile.TemporaryDirectory() as tmpdir:
            folder = Path(tmpdir)

            (folder / "my-pipeline.yml").write_text("""
entrypoint: .gitlab-ci.yml
""")

            config = PipelinesConfig.load_from_folder(folder)

        assert len(config.pipelines) == 1
        assert config.pipelines[0].name == "my-pipeline"

    def test_load_config_with_include_override(self):
        """Test loading config with include overriding all_except."""
        yaml_content = """
pipelines:
  - name: main
    on:
      - changes:
        - all_except:
          - standalone-repo/**/*
        - include:
          - standalone-repo/shared/**/*
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            config = PipelinesConfig.load(Path(f.name))

        assert len(config.pipelines) == 1
        main = config.pipelines[0]
        assert main.triggers[0].all_except == ["standalone-repo/**/*"]
        assert main.triggers[0].include == ["standalone-repo/shared/**/*"]

    def test_load_nonexistent_file(self):
        """Test loading from nonexistent file returns empty config."""
        config = PipelinesConfig.load(Path("/nonexistent/path.yml"))
        assert config.pipelines == []

    def test_load_from_nonexistent_folder(self):
        """Test loading from nonexistent folder returns empty config."""
        config = PipelinesConfig.load_from_folder(Path("/nonexistent/folder"))
        assert config.pipelines == []

    def test_get_triggered_pipelines(self):
        """Test getting triggered pipelines based on changed files."""
        with tempfile.TemporaryDirectory() as tmpdir:
            folder = Path(tmpdir)

            (folder / "main.yml").write_text("""
name: main
entrypoint: .gitlab-ci-main.yml
on:
  - changes:
    - all_except:
      - standalone-repo/**/*
""")

            (folder / "standalone-repo.yml").write_text("""
name: standalone-repo
entrypoint: standalone-repo/.gitlab-ci.yml
on:
  - changes:
    - include:
      - standalone-repo/**/*
""")

            config = PipelinesConfig.load_from_folder(folder)

        # Only standalone-repo files changed
        triggered = config.get_triggered_pipelines(["standalone-repo/main.go"])
        assert len(triggered) == 1
        assert triggered[0].name == "standalone-repo"

        # Only non-standalone-repo files changed
        triggered = config.get_triggered_pipelines(["pkg/main.go"])
        assert len(triggered) == 1
        assert triggered[0].name == "main"

        # Both changed
        triggered = config.get_triggered_pipelines(["pkg/main.go", "standalone-repo/main.go"])
        assert len(triggered) == 2

        # No changes - returns all pipelines
        triggered = config.get_triggered_pipelines([])
        assert len(triggered) == 2


class TestMainPipelineUseCase:
    """
    Integration tests for the main use case:
    - main pipeline triggers on everything except standalone-repo
    - standalone-repo pipeline triggers only on standalone-repo changes
    """

    def setup_method(self):
        """Set up test pipelines."""
        self.main_pipeline = Pipeline(
            name="main",
            triggers=[ChangesTrigger(all_except=["standalone-repo/**/*"])],
        )
        self.standalone_pipeline = Pipeline(
            name="standalone-repo",
            triggers=[ChangesTrigger(include=["standalone-repo/**/*"])],
        )

    def test_only_standalone_repo_changed(self):
        """Only standalone-repo triggers when only those files change."""
        changed = ["standalone-repo/main.go", "standalone-repo/sub/util.go"]

        assert not self.main_pipeline.should_trigger(changed)
        assert self.standalone_pipeline.should_trigger(changed)

    def test_only_other_files_changed(self):
        """Only main triggers when non-standalone-repo files change."""
        changed = ["pkg/main.go", "cmd/agent/main.go"]

        assert self.main_pipeline.should_trigger(changed)
        assert not self.standalone_pipeline.should_trigger(changed)

    def test_both_changed(self):
        """Both pipelines trigger when files in both areas change."""
        changed = ["pkg/main.go", "standalone-repo/main.go"]

        assert self.main_pipeline.should_trigger(changed)
        assert self.standalone_pipeline.should_trigger(changed)

    def test_root_files_trigger_main(self):
        """Root level files trigger main pipeline."""
        changed = ["README.md", ".gitlab-ci.yml"]

        assert self.main_pipeline.should_trigger(changed)
        assert not self.standalone_pipeline.should_trigger(changed)
