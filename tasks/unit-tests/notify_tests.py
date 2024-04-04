import json
import os
import pathlib
import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result
from invoke.exceptions import UnexpectedExit

from tasks import notify


class TestSendMessage(unittest.TestCase):
    @patch("requests.get")
    def test_merge(self, get_mock):
        with open("tasks/unit-tests/testdata/jobs.json") as f:
            jobs = json.load(f)
        job_list = {"json.return_value": jobs}
        no_jobs = {"json.return_value": ""}
        get_mock.side_effect = [MagicMock(status_code=200, **job_list), MagicMock(status_code=200, **no_jobs)]
        notify.send_message(MockContext(), notification_type="merge", print_to_stdout=True)
        get_mock.assert_called()


class TestSendStats(unittest.TestCase):
    @patch("requests.get")
    @patch("tasks.notify.create_count", new=MagicMock())
    def test_nominal(self, get_mock):
        with open("tasks/unit-tests/testdata/jobs.json") as f:
            jobs = json.load(f)
        job_list = {"json.return_value": jobs}
        no_jobs = {"json.return_value": ""}
        get_mock.side_effect = [MagicMock(status_code=200, **job_list), MagicMock(status_code=200, **no_jobs)]
        notify.send_stats(MockContext(), print_to_stdout=True)
        get_mock.assert_called()


class TestCheckConsistentFailures(unittest.TestCase):
    @patch("requests.get")
    def test_nominal(self, get_mock):
        os.environ["CI_PIPELINE_ID"] = "456"
        with open("tasks/unit-tests/testdata/jobs.json") as f:
            jobs = json.load(f)
        job_list = {"json.return_value": jobs}
        no_jobs = {"json.return_value": ""}
        get_mock.side_effect = [MagicMock(status_code=200, **job_list), MagicMock(status_code=200, **no_jobs)]
        notify.check_consistent_failures(
            MockContext(run=Result("test")), "tasks/unit-tests/testdata/job_executions.json"
        )
        get_mock.assert_called()


class TestRetrieveJobExecutionsCreated(unittest.TestCase):
    job_executions = None
    job_file = "job_executions.json"

    def setUp(self) -> None:
        self.job_executions = notify.create_initial_job_executions(self.job_file)

    def tearDown(self) -> None:
        pathlib.Path(self.job_file).unlink(missing_ok=True)

    def test_retrieved(self):
        ctx = MockContext(run=Result("test"))
        j = notify.retrieve_job_executions(ctx, "job_executions.json")
        self.assertEqual(j, self.job_executions)


class TestRetrieveJobExecutions(unittest.TestCase):
    test_json = "tasks/unit-tests/testdata/job_executions.json"

    def test_not_found(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(stderr="This is a 404 not found"))
        j = notify.retrieve_job_executions(ctx, self.test_json)
        self.assertEqual(j, {"pipeline_id": 0, "jobs": {}})

    def test_other_error(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(stderr="This is another error"))
        with self.assertRaises(UnexpectedExit):
            notify.retrieve_job_executions(ctx, self.test_json)


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
