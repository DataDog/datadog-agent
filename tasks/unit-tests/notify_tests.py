import pathlib
import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result
from invoke.exceptions import UnexpectedExit

from tasks import notify


class TestRetrieveJobExecutionsCreated(unittest.TestCase):
    job_executions = None

    def setUp(self) -> None:
        self.job_executions = notify.create_initial_job_executions()

    def tearDown(self) -> None:
        pathlib.Path(notify.JOB_FAILURES_FILE).unlink(missing_ok=True)

    def test_retrieved(self):
        ctx = MockContext(run=Result("test"))
        j = notify.retrieve_job_executions(ctx)
        self.assertEqual(j, self.job_executions)


class TestRetrieveJobExecutions(unittest.TestCase):
    def test_not_found(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(stderr="This is a 404 not found"))
        j = notify.retrieve_job_executions(ctx)
        self.assertEqual(j, {"pipeline_id": 0, "jobs": {}})

    def test_other_error(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(stderr="This is another error"))
        with self.assertRaises(UnexpectedExit):
            notify.retrieve_job_executions(ctx)


class TestUpdateStatistics(unittest.TestCase):
    @patch('tasks.notify.get_failed_jobs')
    def test_nominal(self, mock_get_failed):
        failed_jobs = mock_get_failed.return_value
        failed_jobs.all_failures.return_value = [{"name": "nifnif"}, {"name": "nafnaf"}]
        j = {"jobs": {"nafnaf": {"cumulative_failures": 2}, "noufnouf": {"cumulative_failures": 2}}}
        a, j = notify.update_statistics(j)
        self.assertEqual(j["jobs"]["nifnif"]["cumulative_failures"], 1)
        self.assertEqual(j["jobs"]["nafnaf"]["cumulative_failures"], 3)
        self.assertEqual(j["jobs"]["noufnouf"]["cumulative_failures"], 0)
        self.assertEqual(len(a), 1)
        self.assertIn("nafnaf", a)
        mock_get_failed.assert_called()

    @patch('tasks.notify.get_failed_jobs')
    def test_multiple_failures(self, mock_get_failed):
        failed_jobs = mock_get_failed.return_value
        failed_jobs.all_failures.return_value = [{"name": "poulidor"}, {"name": "virenque"}, {"name": "bardet"}]
        j = {
            "jobs": {
                "poulidor": {"cumulative_failures": 8},
                "virenque": {"cumulative_failures": 2},
                "bardet": {"cumulative_failures": 2},
            }
        }
        a, j = notify.update_statistics(j)
        self.assertEqual(j["jobs"]["poulidor"]["cumulative_failures"], 9)
        self.assertEqual(j["jobs"]["virenque"]["cumulative_failures"], 3)
        self.assertEqual(j["jobs"]["bardet"]["cumulative_failures"], 3)
        self.assertEqual(len(a), 2)
        self.assertIn("virenque", a)
        self.assertIn("bardet", a)
        mock_get_failed.assert_called()
