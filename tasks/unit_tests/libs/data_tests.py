import unittest

from tasks.libs.pipeline.data import get_infra_failure_info
from tasks.libs.types.types import FailedJobReason


class TestGetInfraFailuresJob(unittest.TestCase):
    def test_without_logs(self):
        self.assertEqual(get_infra_failure_info(''), FailedJobReason.GITLAB)
        self.assertEqual(get_infra_failure_info(None), FailedJobReason.GITLAB)

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

    def test_fakeintake_timeout_infra_failure(self):
        # Verbatim from the WINA-3079 job log.
        self.assertEqual(
            get_infra_failure_info(
                'Get "http://10.255.119.230:80/fakeintake/rc/stats": dial tcp 10.255.119.230:80: i/o timeout'
            ),
            FailedJobReason.E2E_INFRA_FAILURE,
        )
        self.assertEqual(
            get_infra_failure_info(
                'Post "http://10.255.119.230:80/fakeintake/rc/config": dial tcp 10.255.119.230:80: i/o timeout'
            ),
            FailedJobReason.E2E_INFRA_FAILURE,
        )

    def test_fakeintake_connection_refused_infra_failure(self):
        # Verbatim from the pipeline-3070 job log: fakeintake not listening, rather than the
        # host being unreachable. Same root cause class, and that job's retry loop also
        # produced two i/o timeouts, so which form ends up reported is a timing coin-flip.
        self.assertEqual(
            get_infra_failure_info(
                'Get "http://10.255.98.252:80/fakeintake/payloads?endpoint=/api/intake/metrics/v3/series": '
                'dial tcp 10.255.98.252:80: connect: connection refused'
            ),
            FailedJobReason.E2E_INFRA_FAILURE,
        )

    def test_fakeintake_url_with_api_endpoint_query_is_infra_failure(self):
        # The ?endpoint= query holds an /api/... path, so the classifier must key off the
        # /fakeintake/ path segment rather than treating any /api/ URL as product traffic.
        self.assertEqual(
            get_infra_failure_info(
                'Get "http://10.255.98.252:80/fakeintake/payloads?endpoint=/api/v2/series": '
                'dial tcp 10.255.98.252:80: i/o timeout'
            ),
            FailedJobReason.E2E_INFRA_FAILURE,
        )

    def test_fakeintake_timeout_panic_infra_failure(self):
        # client.go's c.get() panics with this marker instead of returning the *url.Error.
        self.assertEqual(
            get_infra_failure_info('panic: fakeintake call timed out: dial tcp 10.0.0.1:80: i/o timeout'),
            FailedJobReason.E2E_INFRA_FAILURE,
        )

    def test_non_fakeintake_network_error_is_not_infra_failure(self):
        # Same dial errors on a route the fakeintake client never calls: those may be a real
        # product or networking regression, so widening the error side must not sweep them in.
        self.assertIsNone(
            get_infra_failure_info(
                'Get "http://10.255.119.230:80/api/v2/series": dial tcp 10.255.119.230:80: i/o timeout'
            )
        )
        self.assertIsNone(
            get_infra_failure_info(
                'Get "http://10.255.119.230:80/api/v2/series": '
                'dial tcp 10.255.119.230:80: connect: connection refused'
            )
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
