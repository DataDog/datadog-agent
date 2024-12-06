import unittest

from tasks.libs.pipeline.data import get_infra_failure_info
from tasks.libs.types.types import FailedJobReason


class TestGetInfraFailuresJob(unittest.TestCase):
    def test_without_logs(self):
        self.assertEqual(get_infra_failure_info(''), FailedJobReason.GITLAB)
        self.assertEqual(get_infra_failure_info(None), FailedJobReason.GITLAB)

    def test_kitchen(self):
        self.assertEqual(
            get_infra_failure_info(
                'something ERROR: The kitchen tests failed due to infrastructure failures. something'
            ),
            FailedJobReason.KITCHEN,
        )

    def test_gitlab_5xx(self):
        self.assertEqual(
            get_infra_failure_info(
                'something fatal: unable to access \'.*\': The requested URL returned error: 5.. something'
            ),
            FailedJobReason.GITLAB,
        )

    def test_ec2_spot(self):
        self.assertEqual(
            get_infra_failure_info(
                'something Failed to allocate end to end test EC2 Spot instance after 1 attempts something'
            ),
            FailedJobReason.EC2_SPOT,
        )
        self.assertEqual(
            get_infra_failure_info('something Connection to 192.168.0.1 closed by remote host. something'),
            FailedJobReason.EC2_SPOT,
        )

    def test_e2e_infra_failure(self):
        self.assertEqual(
            get_infra_failure_info('something E2E INTERNAL ERROR something'), FailedJobReason.E2E_INFRA_FAILURE
        )

    def test_no_match(self):
        self.assertIsNone(get_infra_failure_info('something no match something'))

    def test_runner(self):
        self.assertEqual(
            get_infra_failure_info('something Docker runner job start script failed something'),
            FailedJobReason.RUNNER,
        )
        self.assertEqual(
            get_infra_failure_info('something net/http: TLS handshake timeout (test) something'),
            FailedJobReason.RUNNER,
        )
        self.assertEqual(
            get_infra_failure_info('something no basic auth credentials (test) something'),
            FailedJobReason.RUNNER,
        )
