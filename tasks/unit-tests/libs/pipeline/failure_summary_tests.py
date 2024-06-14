import os
import shutil
from contextlib import contextmanager
from datetime import UTC, datetime, timedelta
from unittest import TestCase
from unittest.mock import MagicMock, patch

from gitlab.v4.objects import ProjectManager, ProjectPipeline, ProjectPipelineJob
from invoke.context import Context, MockContext

from tasks.github_tasks import ALL_TEAMS
from tasks.libs.pipeline.notifications import load_and_validate
from tasks.libs.pipeline import failure_summary
from tasks.libs.pipeline.failure_summary import SummaryData, SummaryStats

TEST_DIR = '/tmp/summary'


class FailureSummaryTest(TestCase):
    def __init__(self, methodName: str = "runTest") -> None:
        super().__init__(methodName)

        self.github_slack_map = load_and_validate("tasks/unit-tests/testdata/github_slack_map.yaml", "DEFAULT_SLACK_CHANNEL", '#agent-developer-experience', relpath=False)

    def setUp(self) -> None:
        os.makedirs(TEST_DIR, exist_ok=True)

        self.patches = [
            patch('tasks.libs.pipeline.failure_summary.write_file', self.write_file),
            patch('tasks.libs.pipeline.failure_summary.read_file', self.read_file),
            patch('tasks.libs.pipeline.failure_summary.remove_files', self.remove_files),
            patch('tasks.libs.pipeline.failure_summary.list_files', self.list_files),
            patch('tasks.owners.GITHUB_SLACK_MAP', self.github_slack_map),
        ]
        self.mocks = [patch.start() for patch in self.patches]

    def tearDown(self) -> None:
        shutil.rmtree(TEST_DIR, ignore_errors=True)

        for p in self.patches:
            p.stop()

    def write_file(self, ctx: Context, name: str, data: str):
        with open(f'{TEST_DIR}/{name}', 'w') as f:
            f.write(data)

    def read_file(self, ctx: Context, name: str) -> str:
        with open(f'{TEST_DIR}/{name}') as f:
            return f.read()

    def remove_files(self, ctx: Context, names: list[str]):
        os.system(f'rm -f {TEST_DIR}/{{{",".join(names)}}}')

    def list_files(self, ctx: Context) -> list[str]:
        return os.listdir(TEST_DIR)

    def get_dummy_summary_data(self, jobs: list[dict], id=618) -> SummaryData:
        jobs = [ProjectPipelineJob(manager=MagicMock(), attrs=attr) for attr in jobs]

        return SummaryData(MagicMock(), id, jobs=jobs, pipeline=ProjectPipeline(manager=MagicMock(), attrs={'id': 42}))

    def get_dummy_summary_data_ids(self, job_ids: list[int], id=618) -> SummaryData:
        return self.get_dummy_summary_data([{'id': i} for i in job_ids], id=id)

    @contextmanager
    def patch_fetch_jobs(self, job_ids: list[int]):
        p = patch(
            'tasks.libs.pipeline.failure_summary.fetch_jobs', return_value=self.get_dummy_summary_data_ids(job_ids)
        )
        mock = p.start()

        try:
            yield mock
        finally:
            p.stop()

    def read_result(self):
        files = failure_summary.list_files(None)

        self.assertEqual(len(files), 1)

        return failure_summary.read_file(None, files[0])


class SummaryDataTest(FailureSummaryTest):
    def test_read_write_summaries(self):
        data = self.get_dummy_summary_data_ids([1, 2, 3], id=618)
        data.write()

        data = SummaryData.read(MagicMock(), MagicMock(), 618)
        self.assertEqual(len(data.jobs), 3)
        self.assertEqual(data.jobs[0].id, 1)
        self.assertEqual(data.jobs[1].id, 2)
        self.assertEqual(data.jobs[2].id, 3)

    def test_list_write_summaries(self):
        self.get_dummy_summary_data_ids([1, 2, 3], id=314).write()
        self.get_dummy_summary_data_ids([4, 5, 6], id=618).write()

        summaries = SummaryData.list_summaries(MagicMock())

        self.assertEqual(len(summaries), 2)
        self.assertEqual(summaries[0], 314)
        self.assertEqual(summaries[1], 618)

    def test_list_write_summaries_before(self):
        self.get_dummy_summary_data_ids([1, 2, 3], id=314).write()
        self.get_dummy_summary_data_ids([4, 5, 6], id=618).write()

        summaries = SummaryData.list_summaries(MagicMock(), before=500)

        self.assertEqual(len(summaries), 1)
        self.assertEqual(summaries[0], 314)

    def test_list_write_summaries_after(self):
        self.get_dummy_summary_data_ids([1, 2, 3], id=314).write()
        self.get_dummy_summary_data_ids([4, 5, 6], id=618).write()

        summaries = SummaryData.list_summaries(MagicMock(), after=500)

        self.assertEqual(len(summaries), 1)
        self.assertEqual(summaries[0], 618)

    def test_merge_summaries(self):
        summary = SummaryData.merge(
            [
                self.get_dummy_summary_data_ids([1, 2, 3], id=314),
                self.get_dummy_summary_data_ids([4, 5, 6], id=618),
                self.get_dummy_summary_data_ids([7, 8], id=1618),
            ]
        )
        self.assertEqual(len(summary.jobs), 8)
        self.assertEqual(summary.id, None)


class SummaryStatsTest(FailureSummaryTest):
    def test_make_stats(self):
        data = self.get_dummy_summary_data(
            [
                {'name': 'job1', 'status': 'success', 'allow_failure': False},
                {'name': 'job2', 'status': 'failed', 'allow_failure': False},
                {'name': 'job1', 'status': 'failed', 'allow_failure': False},
                {'name': 'job2', 'status': 'success', 'allow_failure': False},
                {'name': 'job1', 'status': 'failed', 'allow_failure': False},
            ]
        )

        stats = SummaryStats(data, allow_failure=False)
        results = stats.make_stats(max_length=1000, jobowners='tasks/unit-tests/testdata/jobowners.txt')
        results = {channel: sorted(result, key=lambda d: d['name']) for channel, result in results.items()}
        result = results[self.github_slack_map[ALL_TEAMS]]

        self.assertEqual(len(result), 2)
        self.assertEqual(result[0], {'name': 'job1', 'failures': 2, 'runs': 3})
        self.assertEqual(result[1], {'name': 'job2', 'failures': 1, 'runs': 2})

    def test_make_stats_allow_failure(self):
        data = self.get_dummy_summary_data(
            [
                {'name': 'job1', 'status': 'success', 'allow_failure': False},
                {'name': 'job2', 'status': 'failed', 'allow_failure': False},
                {'name': 'job1', 'status': 'failed', 'allow_failure': False},
                {'name': 'job2', 'status': 'success', 'allow_failure': False},
                {'name': 'job1', 'status': 'failed', 'allow_failure': False},
                {'name': 'job3', 'status': 'failed', 'allow_failure': True},
                {'name': 'job4', 'status': 'success', 'allow_failure': True},
                {'name': 'job3', 'status': 'success', 'allow_failure': True},
                {'name': 'job4', 'status': 'success', 'allow_failure': True},
            ]
        )

        stats = SummaryStats(data, allow_failure=False)
        results = stats.make_stats(max_length=1000, jobowners='tasks/unit-tests/testdata/jobowners.txt')
        results = {channel: sorted(result, key=lambda d: d['name']) for channel, result in results.items()}
        result = results[self.github_slack_map[ALL_TEAMS]]

        self.assertEqual(len(result), 2)
        self.assertEqual(result[0], {'name': 'job1', 'failures': 2, 'runs': 3})
        self.assertEqual(result[1], {'name': 'job2', 'failures': 1, 'runs': 2})

        stats = SummaryStats(data, allow_failure=True)
        results = stats.make_stats(max_length=1000, jobowners='tasks/unit-tests/testdata/jobowners.txt')
        results = {channel: sorted(result, key=lambda d: d['name']) for channel, result in results.items()}
        result = results[self.github_slack_map[ALL_TEAMS]]

        self.assertEqual(len(result), 1)
        self.assertEqual(result[0], {'name': 'job3', 'failures': 1, 'runs': 2})

    def test_make_stats_channels(self):
        data = self.get_dummy_summary_data(
            [
                {'name': 'tests_hello', 'status': 'failed', 'allow_failure': False},
                {'name': 'tests_ebpf1', 'status': 'failed', 'allow_failure': False},
                {'name': 'tests_ebpf2', 'status': 'failed', 'allow_failure': False},
                {'name': 'tests_release', 'status': 'failed', 'allow_failure': False},
            ]
        )

        stats = SummaryStats(data, allow_failure=False)
        results = stats.make_stats(max_length=1000, jobowners='tasks/unit-tests/testdata/jobowners.txt')
        results = {channel: sorted(result, key=lambda d: d['name']) for channel, result in results.items()}

        self.assertSetEqual(set(results), {'#agent-developer-experience', '#ebpf-platform-ops', '#agent-build-and-releases', self.github_slack_map[ALL_TEAMS]})
        self.assertEqual(len(results['#agent-developer-experience']), 1)
        self.assertEqual(len(results['#ebpf-platform-ops']), 2)
        self.assertEqual(len(results['#agent-build-and-releases']), 1)
        self.assertEqual(len(results[self.github_slack_map[ALL_TEAMS]]), 4)


class ModuleTest(FailureSummaryTest):
    def make_dummy_summaries(self):
        days = [2, 4, 6, 8, 10]
        summaries = []
        for day in days:
            id = int(datetime(2042, 1, day, tzinfo=UTC).timestamp())
            summary = SummaryData(MagicMock(), id, jobs=[ProjectPipelineJob(manager=MagicMock(), attrs={'id': day})])
            summary.write()
            summaries.append(summary)

        return summaries

    def test_is_valid_job_infra(self):
        repo = MagicMock()
        repo.jobs.get.return_value.trace.return_value = b'Docker runner job start script failed'
        job = MagicMock()
        job.status = 'failed'

        self.assertFalse(failure_summary.is_valid_job(repo, job))

    def test_is_valid_job_failed(self):
        repo = MagicMock()
        repo.jobs.get.return_value.trace.return_value = b'Python error'
        job = MagicMock()
        job.status = 'failed'

        self.assertTrue(failure_summary.is_valid_job(repo, job))

    def test_is_valid_job_not_finished(self):
        repo = MagicMock()
        job = MagicMock()
        job.status = 'running'

        self.assertFalse(failure_summary.is_valid_job(repo, job))

    def test_is_valid_job_success(self):
        repo = MagicMock()
        job = MagicMock()
        job.status = 'success'

        self.assertTrue(failure_summary.is_valid_job(repo, job))

    def test_upload_summary(self):
        # Upload summary then retrieve the summary and expect 2 jobs
        with self.patch_fetch_jobs([1, 2]):
            summary_id = failure_summary.upload_summary(None, None).id
            summary = SummaryData.read(None, MagicMock(), summary_id)

            self.assertEqual(len(summary.jobs), 2)
            self.assertEqual(summary.jobs[0].id, 1)
            self.assertEqual(summary.jobs[1].id, 2)

    @patch('tasks.libs.pipeline.failure_summary.datetime')
    def test_clean_summaries(self, mock):
        mock.now.return_value = datetime(2042, 1, 16, tzinfo=UTC)

        summaries = self.make_dummy_summaries()
        ids = [s.id for s in summaries]

        failure_summary.clean_summaries(MagicMock(), period=timedelta(days=10))
        new_ids = failure_summary.list_files(MagicMock())
        new_ids = sorted(SummaryData.get_id(name) for name in new_ids)

        # 6, 8, 10
        self.assertEqual(new_ids, ids[2:])

    @patch("os.environ", new=MagicMock())
    @patch("tasks.libs.pipeline.failure_summary.send_summary_slack_message")
    def test_send_summary_messages(self, mock_slack: MagicMock):
        # Verify that we send the right number of jobs per channel
        expected_team_njobs = {
            '#agent-build-and-releases': 2,
            '#agent-developer-experience': 4,
            '#security-and-compliance-agent-ops': 1,
            self.github_slack_map[ALL_TEAMS]: 5,
        }

        summary = SummaryData(MagicMock(), 42, jobs=[
            # agent-ci-experience
            *[ProjectPipelineJob(manager=MagicMock(), attrs={'name': 'hello', 'status': 'failed', 'allow_failure': False}) for _ in range(20)],
            # agent-ci-experience
            *[ProjectPipelineJob(manager=MagicMock(), attrs={'name': 'world', 'status': 'failed', 'allow_failure': False}) for _ in range(12)],
            # agent-security
            *[ProjectPipelineJob(manager=MagicMock(), attrs={'name': 'security_go_generate_check', 'status': 'failed', 'allow_failure': False}) for _ in range(10)],
            # agent-ci-experience, agent-build-and-releases
            *[ProjectPipelineJob(manager=MagicMock(), attrs={'name': 'tests_release', 'status': 'failed', 'allow_failure': False}) for _ in range(5)],
            # agent-ci-experience, agent-build-and-releases
            *[ProjectPipelineJob(manager=MagicMock(), attrs={'name': 'tests_release2', 'status': 'failed', 'allow_failure': False}) for _ in range(2)]
        ])

        with patch('tasks.libs.pipeline.failure_summary.fetch_summaries', return_value=summary):
            failure_summary.send_summary_messages(
                MockContext(), allow_failure=False, jobowners="tasks/unit-tests/testdata/jobowners.txt", max_length=1000, period=timedelta(weeks=10)
            )

        # Verify called once for each channel
        self.assertEqual(len(mock_slack.call_args_list), len(expected_team_njobs))

        for call_args in mock_slack.call_args_list:
            channel = call_args.kwargs['channel']
            stats = call_args.kwargs['stats']
            njobs = len(stats)
            self.assertEqual(expected_team_njobs.get(channel, None), njobs, 'Failure for channel: ' + channel)

