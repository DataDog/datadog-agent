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
        j = {
            "jobs": {
                "nafnaf": {"consecutive_failures": 2, "cumulative_failures": [0, 0, 0, 0, 0, 0, 0, 0, 1, 1]},
                "noufnouf": {"consecutive_failures": 2, "cumulative_failures": [1, 0, 1, 1]},
            }
        }
        a, j = notify.update_statistics(j)
        self.assertEqual(j["jobs"]["nifnif"]["consecutive_failures"], 1)
        self.assertEqual(j["jobs"]["nifnif"]["cumulative_failures"], [1])
        self.assertEqual(j["jobs"]["nafnaf"]["consecutive_failures"], 3)
        self.assertEqual(j["jobs"]["nafnaf"]["cumulative_failures"], [0, 0, 0, 0, 0, 0, 0, 1, 1, 1])
        self.assertEqual(j["jobs"]["noufnouf"]["consecutive_failures"], 0)
        self.assertEqual(j["jobs"]["noufnouf"]["cumulative_failures"], [1, 0, 1, 1, 0])
        self.assertEqual(len(a["consecutive"]), 1)
        self.assertEqual(len(a["cumulative"]), 0)
        self.assertIn("nafnaf", a["consecutive"])
        mock_get_failed.assert_called()

    @patch('tasks.notify.get_failed_jobs')
    def test_multiple_failures(self, mock_get_failed):
        failed_jobs = mock_get_failed.return_value
        failed_jobs.all_failures.return_value = [{"name": "poulidor"}, {"name": "virenque"}, {"name": "bardet"}]
        j = {
            "jobs": {
                "poulidor": {"consecutive_failures": 8, "cumulative_failures": [0, 0, 1, 1, 1, 1, 1, 1, 1, 1]},
                "virenque": {"consecutive_failures": 2, "cumulative_failures": [0, 0, 0, 0, 1, 0, 1, 0, 1, 1]},
                "bardet": {"consecutive_failures": 2, "cumulative_failures": [1, 1]},
            }
        }
        a, j = notify.update_statistics(j)
        self.assertEqual(j["jobs"]["poulidor"]["consecutive_failures"], 9)
        self.assertEqual(j["jobs"]["virenque"]["consecutive_failures"], 3)
        self.assertEqual(j["jobs"]["bardet"]["consecutive_failures"], 3)
        self.assertEqual(len(a["consecutive"]), 2)
        self.assertEqual(len(a["cumulative"]), 1)
        self.assertIn("virenque", a["consecutive"])
        self.assertIn("bardet", a["consecutive"])
        self.assertIn("virenque", a["cumulative"])
        mock_get_failed.assert_called()


class TestSendNotification(unittest.TestCase):
    @patch('tasks.notify.send_slack_message')
    def test_consecutive(self, mock_slack):
        alert_jobs = {"consecutive": ["foo"], "cumulative": []}
        notify.send_notification(alert_jobs)
        mock_slack.assert_called_with(
            "#agent-platform-ops", f"Job(s) `foo` failed {notify.CONSECUTIVE_THRESHOLD} times in a row.\n"
        )

    @patch('tasks.notify.send_slack_message')
    def test_cumulative(self, mock_slack):
        alert_jobs = {"consecutive": [], "cumulative": ["bar", "baz"]}
        notify.send_notification(alert_jobs)
        mock_slack.assert_called_with(
            "#agent-platform-ops",
            f"Job(s) `bar`, `baz` failed {notify.CUMULATIVE_THRESHOLD} times in last {notify.CUMULATIVE_LENGTH} executions.\n",
        )

    @patch('tasks.notify.send_slack_message')
    def test_both(self, mock_slack):
        alert_jobs = {"consecutive": ["foo"], "cumulative": ["bar", "baz"]}
        notify.send_notification(alert_jobs)
        mock_slack.assert_called_with(
            "#agent-platform-ops",
            f"Job(s) `foo` failed {notify.CONSECUTIVE_THRESHOLD} times in a row.\nJob(s) `bar`, `baz` failed {notify.CUMULATIVE_THRESHOLD} times in last {notify.CUMULATIVE_LENGTH} executions.\n",
        )

    @patch('tasks.notify.send_slack_message')
    def test_none(self, mock_slack):
        alert_jobs = {"consecutive": [], "cumulative": []}
        notify.send_notification(alert_jobs)
        mock_slack.assert_not_called()
