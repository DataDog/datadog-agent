import unittest

from tasks.libs.pipeline.generation import remove_fields, update_child_job_variables, update_needs_parent


class TestGeneration(unittest.TestCase):
    def test_update_child_jobs_variable(self):
        kept_jobs = {
            "job1": {
                "variables": {
                    "E2E_PIPELINE_ID": "$CI_PIPELINE_ID",
                    "E2E_COMMIT_SHA": "$CI_COMMIT_SHA",
                    "E2E_COMMIT_SHORT_SHA": "$CI_COMMIT_SHORT_SHA",
                    "IMAGE_TAG": "latest-$CI_COMMIT_SHORT_SHA-$CI_PIPELINE_ID",
                }
            },
        }
        updated_jobs = update_child_job_variables(kept_jobs)

        self.assertEqual(
            updated_jobs,
            {
                "job1": {
                    "variables": {
                        "E2E_PIPELINE_ID": "$PARENT_PIPELINE_ID",
                        "E2E_COMMIT_SHA": "$PARENT_COMMIT_SHA",
                        "E2E_COMMIT_SHORT_SHA": "$PARENT_COMMIT_SHORT_SHA",
                        "IMAGE_TAG": "latest-$PARENT_COMMIT_SHORT_SHA-$PARENT_PIPELINE_ID",
                    }
                }
            },
        )

    def test_update_needs_parent(self):
        needs = [
            {"optional": "true", "job": "job1"},
            "job2",
            ["job3", "job4"],
            "job5",
        ]
        deps_to_keep = ["job1", "job3", "job5"]
        new_needs = update_needs_parent(needs, deps_to_keep)

        self.assertEqual(
            new_needs,
            [
                {"optional": "true", "pipeline": "$PARENT_PIPELINE_ID", "job": "job1"},
                {"pipeline": "$PARENT_PIPELINE_ID", "job": "job3"},
                {"pipeline": "$PARENT_PIPELINE_ID", "job": "job5"},
            ],
        )

    def test_remove_fields(self):
        job = {
            "needs": ["toto"],
            "rules": [],
            "extends": [],
            "retry": 2,
        }
        remove_fields(job)

        self.assertEqual(
            job,
            {"needs": ["toto"]},
        )
