from __future__ import annotations

import json
import sys
import unittest
from unittest.mock import MagicMock, patch

from gitlab.v4.objects import ProjectJob
from invoke import MockContext

from tasks import notify
from tasks.libs.pipeline.notifications import load_and_validate


def get_fake_jobs() -> list[ProjectJob]:
    with open("tasks/unit_tests/testdata/jobs.json") as f:
        jobs = json.load(f)

    return [ProjectJob(MagicMock(), attrs=job) for job in jobs]


def get_github_slack_map():
    return load_and_validate(
        "tasks/unit_tests/testdata/github_slack_map.yaml",
        "DEFAULT_SLACK_CHANNEL",
        '#channel-everything',
        relpath=False,
    )


class TestSendStats(unittest.TestCase):
    @patch('tasks.libs.ciproviders.gitlab_api.get_gitlab_api')
    @patch("tasks.libs.notify.alerts.create_count", new=MagicMock())
    def test_nominal(self, api_mock):
        repo_mock = api_mock.return_value.projects.get.return_value
        trace_mock = repo_mock.jobs.get.return_value.trace
        pipeline_mock = repo_mock.pipelines.get

        trace_mock.return_value = b"E2E INTERNAL ERROR"
        attrs = {"jobs.list.return_value": get_fake_jobs(), "created_at": "2024-03-12T10:00:00.000Z"}
        pipeline_mock.return_value = MagicMock(**attrs)

        notify.send_stats(MockContext(), dry_run=True)

        trace_mock.assert_called()
        pipeline_mock.assert_called()
        self.assertEqual(pipeline_mock.call_count, 1)


class TestJobOwners(unittest.TestCase):
    def test_partition(self):
        from tasks.owners import make_partition

        jobs = [
            'tests_team_a_42',
            'tests_team_a_618',
            'this_is_a_test',
            'tests_team_b_1',
            'tests_letters_0',
            'hello_world',
        ]

        partition = make_partition(jobs, "tasks/unit_tests/testdata/jobowners.txt")
        partition = sorted(partition.items())

        self.assertEqual(
            partition,
            [
                ('@DataDog/team-a', {'tests_team_a_42', 'tests_team_a_618', 'tests_letters_0'}),
                ('@DataDog/team-b', {'tests_team_b_1', 'tests_letters_0'}),
                ('@DataDog/team-everything', {'this_is_a_test', 'hello_world'}),
            ],
        )


class TestFailureSummarySendNotifications(unittest.TestCase):
    @patch.dict('os.environ', {'CI_PIPELINE_CREATED_AT': '2025-02-07T11:11:11.111Z'})
    @patch('builtins.print')
    def test_ignore_non_scheduled_conductor(self, print_mock):
        notify.failure_summary_send_notifications(MockContext(), daily_summary=True)
        print_mock.assert_called_with(
            "Failure summary notifications are only sent during the conductor scheduled pipeline, skipping",
            file=sys.stderr,
        )

    @patch.dict('os.environ', {'CI_PIPELINE_CREATED_AT': '2025-02-07T06:11:11.111Z'})
    @patch('tasks.notify.failure_summary', new=MagicMock())
    @patch('builtins.print')
    def test_sheduled_conductor(self, print_mock):
        notify.failure_summary_send_notifications(MockContext(), daily_summary=True)
        print_mock.assert_not_called()

    @patch.dict('os.environ', {'CI_PIPELINE_CREATED_AT': '2025-02-07T05:22:22.222Z'})
    @patch('tasks.notify.failure_summary', new=MagicMock())
    @patch('builtins.print')
    def test_sheduled_conductor_dst(self, print_mock):
        notify.failure_summary_send_notifications(MockContext(), daily_summary=True)
        print_mock.assert_not_called()
