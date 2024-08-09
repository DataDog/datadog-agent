from collections import OrderedDict
from unittest import TestCase
from urllib.parse import quote

from tasks.libs.notify.utils import get_ci_visibility_job_url


class TestGetCiVisibilityJobUrl(TestCase):
    def test_simple_prefix(self):
        url = get_ci_visibility_job_url("my-job")
        query = quote(r"@ci.job.name:my\-job*")
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count'

        self.assertEqual(url, expected)

    def test_simple_full(self):
        url = get_ci_visibility_job_url("my-job", prefix=False)
        query = quote('@ci.job.name:"my-job"')
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count'

        self.assertEqual(url, expected)

    def test_prefix(self):
        url = get_ci_visibility_job_url('my-job: ["hello"]')
        query = quote(r'@ci.job.name:my\-job\:\ \[\"hello\"\]*')
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count'

        self.assertEqual(url, expected)

    def test_full(self):
        url = get_ci_visibility_job_url('my-job: ["hello"]', prefix=False)
        query = quote(r'@ci.job.name:my\-job\:\ \[\"hello\"\]')
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count'

        self.assertEqual(url, expected)

    def test_full_flags(self):
        url = get_ci_visibility_job_url("my-job", prefix=False, extra_flags=["status:error", "-@error.domain:provider"])
        query = quote('@ci.job.name:"my-job" status:error -@error.domain:provider')
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count'

        self.assertEqual(url, expected)

    def test_full_args(self):
        url = get_ci_visibility_job_url(
            "my-job", prefix=False, extra_args=OrderedDict([("start", "123"), ("paused", "true")])
        )
        query = quote('@ci.job.name:"my-job"')
        suffix = '&start=123&paused=true'
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count{suffix}'

        self.assertEqual(url, expected)

    def test_full_flags_args(self):
        url = get_ci_visibility_job_url(
            "my-job",
            prefix=False,
            extra_flags=["status:error", "-@error.domain:provider"],
            extra_args=OrderedDict([("start", "123"), ("paused", "true")]),
        )
        query = quote('@ci.job.name:"my-job" status:error -@error.domain:provider')
        suffix = '&start=123&paused=true'
        expected = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20{query}&agg_m=count{suffix}'

        self.assertEqual(url, expected)
