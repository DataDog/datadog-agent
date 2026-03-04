"""
Tests for the GitLab CI parser.

Run with Bazel:
    bazel test //tools/pipeline_analysis/tests:test_gitlab_ci_parser
"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

from pipeline_analysis.parsers.gitlab_ci import GitLabCIParser, Job, _deep_merge

_HERE = Path(__file__).parent
FIXTURES = _HERE / "fixtures"


# ---------------------------------------------------------------------------
# _deep_merge unit tests
# ---------------------------------------------------------------------------


class TestDeepMerge:
    def test_scalar_override(self):
        base = {"a": 1, "b": 2}
        override = {"b": 99, "c": 3}
        result = _deep_merge(base, override)
        assert result == {"a": 1, "b": 99, "c": 3}

    def test_script_override_not_merge(self):
        base = {"script": ["echo base"]}
        override = {"script": ["echo override"]}
        result = _deep_merge(base, override)
        assert result["script"] == ["echo override"]

    def test_variables_deep_merge(self):
        base = {"variables": {"A": "1", "B": "2"}}
        override = {"variables": {"B": "99", "C": "3"}}
        result = _deep_merge(base, override)
        assert result["variables"] == {"A": "1", "B": "99", "C": "3"}

    def test_rules_override(self):
        base = {"rules": [{"when": "always"}]}
        override = {"rules": [{"when": "never"}]}
        result = _deep_merge(base, override)
        assert result["rules"] == [{"when": "never"}]

    def test_nested_dict_merge(self):
        base = {"cache": {"key": "base", "paths": ["/cache"]}}
        override = {"cache": {"policy": "pull"}}
        result = _deep_merge(base, override)
        assert result["cache"]["key"] == "base"
        assert result["cache"]["policy"] == "pull"


# ---------------------------------------------------------------------------
# Parser tests using fixture files
# ---------------------------------------------------------------------------


class TestSimpleFixture:
    """Tests using fixtures/simple.yml"""

    def setup_method(self):
        # Create a minimal "repo" with a .gitlab-ci.yml pointing to simple.yml
        # We'll use a temporary directory trick: just parse fixture directly
        # by monkey-patching the root
        self.parser = _FixtureParser(FIXTURES / "simple.yml")

    def test_stages(self):
        self.parser.parse()
        assert self.parser.stages == ["build", "test", "deploy"]

    def test_job_count(self):
        jobs = self.parser.parse()
        # 3 real jobs + .base_job hidden (hidden jobs filtered out)
        job_names = {j.name for j in jobs}
        assert "build-linux" in job_names
        assert "test-unit" in job_names
        assert "deploy-package" in job_names
        assert ".base_job" not in job_names

    def test_extends_resolution(self):
        jobs = self.parser.parse()
        build = next(j for j in jobs if j.name == "build-linux")
        # .base_job has tags: [runner:main]; build-linux overrides with [arch:amd64, os:linux]
        assert "arch:amd64" in build.tags
        assert "os:linux" in build.tags

    def test_needs(self):
        jobs = self.parser.parse()
        test = next(j for j in jobs if j.name == "test-unit")
        assert test.needs == ["build-linux"]

    def test_needs_dict_form(self):
        jobs = self.parser.parse()
        deploy = next(j for j in jobs if j.name == "deploy-package")
        assert set(deploy.needs) == {"test-unit", "build-linux"}

    def test_trigger(self):
        jobs = self.parser.parse()
        deploy = next(j for j in jobs if j.name == "deploy-package")
        assert deploy.trigger is not None
        assert deploy.job_type == "trigger"

    def test_s3_produces(self):
        jobs = self.parser.parse()
        build = next(j for j in jobs if j.name == "build-linux")
        assert len(build.s3_produces) > 0

    def test_s3_consumes(self):
        jobs = self.parser.parse()
        deploy = next(j for j in jobs if j.name == "deploy-package")
        assert len(deploy.s3_consumes) > 0


class TestReferenceFixture:
    """Tests using fixtures/reference.yml"""

    def setup_method(self):
        self.parser = _FixtureParser(FIXTURES / "reference.yml")

    def test_reference_tag_in_rules(self):
        jobs = self.parser.parse()
        build = next(j for j in jobs if j.name == "build-job")
        # rules should have been resolved from .shared_rules
        assert isinstance(build.rules, list)
        assert len(build.rules) > 0

    def test_reference_tag_in_script(self):
        jobs = self.parser.parse()
        build = next(j for j in jobs if j.name == "build-job")
        # script should contain resolved shared steps + extra step
        assert any("shared" in s for s in build.script)
        assert any("extra" in s for s in build.script)


# ---------------------------------------------------------------------------
# Job dataclass tests
# ---------------------------------------------------------------------------


class TestJobPlatform:
    def _make_job(self, tags):
        return Job(name="x", stage="build", tags=tags)

    def test_linux(self):
        assert self._make_job(["arch:amd64"]).platform == "linux"

    def test_windows(self):
        assert self._make_job(["runner:windows-medium"]).platform == "windows"

    def test_mac(self):
        assert self._make_job(["os:darwin"]).platform == "mac"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


class _FixtureParser(GitLabCIParser):
    """Parser variant that loads a single fixture file instead of a whole repo."""

    def __init__(self, fixture_path: Path):
        # Use fixture's parent as "repo root" but override _load_all
        super().__init__(fixture_path.parent)
        self._fixture_path = fixture_path

    def _load_all(self):
        data = self._load_file(self._fixture_path)
        self._raw = data
        self.stages = data.get("stages", [])
        self.variables = {k: str(v) for k, v in data.get("variables", {}).items()}


if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-v"]))
