import unittest

from tasks.gitlab_helpers import (
    CI_VISIBILITY_URL,
    create_gitlab_annotations_report,
)


class TestCreateGitlabAnnotations(unittest.TestCase):
    def test_create_gitlab_annotations_report_basic_name(self):
        job_id = "123"
        job_name = "test-job"
        result = create_gitlab_annotations_report(job_id, job_name)
        expected = {
            "CI Visibility": [
                {
                    "external_link": {
                        "label": "CI Visibility: This job instance",
                        "url": f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.id%3A123%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent",
                    }
                },
                {
                    "external_link": {
                        "label": "CI Visibility: This job on main",
                        "url": f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.name%3A%22test-job%22%20%40git.branch%3Amain%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent",
                    }
                },
            ]
        }
        self.assertEqual(
            result,
            expected,
        )

    def test_create_gitlab_annotations_report_name_with_spaces(self):
        job_id = "123"
        job_name = "test job"
        result = create_gitlab_annotations_report(job_id, job_name)
        expected = {
            "CI Visibility": [
                {
                    "external_link": {
                        "label": "CI Visibility: This job instance",
                        "url": f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.id%3A123%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent",
                    }
                },
                {
                    "external_link": {
                        "label": "CI Visibility: This job on main",
                        "url": f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.name%3A%22test%20job%22%20%40git.branch%3Amain%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent",
                    }
                },
            ]
        }
        self.assertEqual(
            result,
            expected,
        )

    def test_create_gitlab_annotations_report_name_with_weird_chars(self):
        job_id = "123"
        job_name = "test job [One|Two|Three] --skip Four"
        result = create_gitlab_annotations_report(job_id, job_name)
        expected = {
            "CI Visibility": [
                {
                    "external_link": {
                        "label": "CI Visibility: This job instance",
                        "url": f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.id%3A123%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent",
                    }
                },
                {
                    "external_link": {
                        "label": "CI Visibility: This job on main",
                        "url": f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.name%3A%22test%20job%20%5BOne%7CTwo%7CThree%5D%20--skip%20Four%22%20%40git.branch%3Amain%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent",
                    }
                },
            ]
        }
        self.assertEqual(
            result,
            expected,
        )
