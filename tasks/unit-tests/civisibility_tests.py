import unittest

from tasks.libs.civisibility import (
    CI_VISIBILITY_URL,
    TEST_VISIBILITY_URL,
    get_pipeline_link_to_job_id,
    get_pipeline_link_to_job_on_main,
    get_test_link_to_job_id,
    get_test_link_to_job_on_main,
    get_test_link_to_test_on_main,
)


class TestCIVisibilityLinks(unittest.TestCase):
    def test_get_pipeline_link_to_job_id(self):
        job_id = "123"
        result = get_pipeline_link_to_job_id(job_id)
        expected = f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.id%3A123%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent"

        self.assertEqual(
            result,
            expected,
        )

    def test_get_pipeline_link_to_job_on_main(self):
        job_name = "test job -- totoro"
        result = get_pipeline_link_to_job_on_main(job_name)
        expected = f"{CI_VISIBILITY_URL}?query=ci_level%3Ajob%20%40ci.job.name%3A%22test%20job%20--%20totoro%22%20%40git.branch%3Amain%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent"

        self.assertEqual(
            result,
            expected,
        )

    def test_get_test_link_to_job_id(self):
        job_id = "123"
        result = get_test_link_to_job_id(job_id)
        expected = f"{TEST_VISIBILITY_URL}?query=test_level%3Atest%20%40ci.job.url%3A%22https%3A%2F%2Fgitlab.ddbuild.io%2FDataDog%2Fdatadog-agent%2F-%2Fjobs%2F123%22%20%40test.service%3Adatadog-agent"
        self.assertEqual(
            result,
            expected,
        )

    def test_get_test_link_to_job_on_main(self):
        job_name = "Test Name -- Totoro"
        result = get_test_link_to_job_on_main(job_name)
        expected = f"{TEST_VISIBILITY_URL}?query=test_level%3Atest%20%40ci.job.name%3A%22Test%20Name%20--%20Totoro%22%20%40git.branch%3Amain%20%40test.service%3Adatadog-agent"
        self.assertEqual(
            result,
            expected,
        )

    def test_get_test_link_to_test_on_main(self):
        suite_name = "github.com/DataDog/datadog-agent/totoro"
        test_name = "TestTotoro/WhenTheyWakeUp"
        result = get_test_link_to_test_on_main(suite_name, test_name)
        expected = f"{TEST_VISIBILITY_URL}?query=test_level%3Atest%20%40ci.test.name%3A%22TestTotoro%2FWhenTheyWakeUp%22%20%40ci.suite.name%3A%22github.com%2FDataDog%2Fdatadog-agent%2Ftotoro%22%20%40git.branch%3Amain%20%40test.service%3Adatadog-agent"
        self.assertEqual(
            result,
            expected,
        )
