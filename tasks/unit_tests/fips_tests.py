import unittest
from contextlib import chdir
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

import yaml
from invoke import MockContext

from tasks.fips import generate_fips_e2e_pipeline


class TestFipsPipeline(unittest.TestCase):
    def test_preserves_job_retry_policy(self):
        infra_retry = {
            "max": 2,
            "when": ["runner_system_failure", "stuck_or_timeout_failure", "scheduler_failure"],
            "exit_codes": [42],
        }
        for generate_config in (False, True):
            for retry in (infra_retry, 0):
                with (
                    self.subTest(generate_config=generate_config, retry=retry),
                    TemporaryDirectory() as directory,
                    chdir(directory),
                ):
                    config = {
                        "variables": {},
                        "default": {"retry": {"max": 1, "when": "always"}},
                        "new-e2e-test": {
                            "stage": "e2e",
                            "variables": {"ON_NIGHTLY_FIPS": "true"},
                            "script": ["run-tests"],
                            "needs": ["go_e2e_deps"],
                            "rules": [{"when": "always"}],
                            "extends": ".new_e2e_template",
                            "retry": retry,
                        },
                    }
                    if not generate_config:
                        Path("artifacts").mkdir()
                        Path("artifacts/after.gitlab-ci.yml").write_text(yaml.safe_dump({".gitlab-ci.yml": config}))

                    with patch("tasks.fips.resolve_gitlab_ci_configuration", return_value=config):
                        generate_fips_e2e_pipeline.body(MockContext(), generate_config=generate_config)

                    pipeline = yaml.safe_load(Path("fips-e2e-gitlab-ci.yml").read_text())
                    job = pipeline["new-e2e-test-fips"]
                    # A test failure must not regain the inherited retry-on-any-failure policy.
                    effective_retry = job.get("retry", pipeline["default"]["retry"])
                    self.assertEqual(effective_retry, retry)
                    self.assertNotIn("rules", job)
                    self.assertNotIn("extends", job)
