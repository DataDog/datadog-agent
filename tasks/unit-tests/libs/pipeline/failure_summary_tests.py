import os
import shutil
from contextlib import contextmanager
from datetime import UTC, datetime, timedelta
from unittest import TestCase
from unittest.mock import MagicMock, patch

from gitlab.v4.objects import ProjectManager, ProjectPipeline, ProjectPipelineJob
from invoke import Context

from tasks.libs.pipeline import failure_summary
from tasks.libs.pipeline.failure_summary import SummaryData, SummaryStats

TEST_DIR = '/tmp/summary'


class FailureSummaryTest(TestCase):
    def setUp(self) -> None:
        os.makedirs(TEST_DIR, exist_ok=True)

        self.patches = [
            patch('tasks.libs.pipeline.failure_summary.write_file', self.write_file),
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
                {'name': 'Job1', 'status': 'success', 'allow_failure': False},
                {'name': 'Job2', 'status': 'failed', 'allow_failure': False},
                {'name': 'Job1', 'status': 'failed', 'allow_failure': False},
                {'name': 'Job2', 'status': 'success', 'allow_failure': False},
                {'name': 'Job1', 'status': 'failed', 'allow_failure': False},
            ]
        )

        stats = SummaryStats(data, allow_failure=False)
        result = stats.make_stats(max_length=1000, team=None)
        result = sorted(result, key=lambda d: d['name'])

        self.assertEqual(len(result), 2)
        self.assertEqual(result[0], {'name': 'Job1', 'failures': 2, 'runs': 3})
        self.assertEqual(result[1], {'name': 'Job2', 'failures': 1, 'runs': 2})

    def test_make_stats_allow_failure(self):
        data = self.get_dummy_summary_data(
            [
                {'name': 'Job1', 'status': 'success', 'allow_failure': False},
                {'name': 'Job2', 'status': 'failed', 'allow_failure': False},
                {'name': 'Job1', 'status': 'failed', 'allow_failure': False},
                {'name': 'Job2', 'status': 'success', 'allow_failure': False},
                {'name': 'Job1', 'status': 'failed', 'allow_failure': False},
                {'name': 'Job3', 'status': 'failed', 'allow_failure': True},
                {'name': 'Job4', 'status': 'success', 'allow_failure': True},
                {'name': 'Job3', 'status': 'success', 'allow_failure': True},
                {'name': 'Job4', 'status': 'success', 'allow_failure': True},
            ]
        )

        stats = SummaryStats(data, allow_failure=False)
        result = stats.make_stats(max_length=1000, team=None)
        result = sorted(result, key=lambda d: d['name'])

        self.assertEqual(len(result), 2)
        self.assertEqual(result[0], {'name': 'Job1', 'failures': 2, 'runs': 3})
        self.assertEqual(result[1], {'name': 'Job2', 'failures': 1, 'runs': 2})

        stats = SummaryStats(data, allow_failure=True)
        result = stats.make_stats(max_length=1000, team=None)
        result = sorted(result, key=lambda d: d['name'])

        self.assertEqual(len(result), 1)
        self.assertEqual(result[0], {'name': 'Job3', 'failures': 1, 'runs': 2})


class ModuleTest(FailureSummaryTest):
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

        days = [2, 4, 6, 8, 10]
        summaries = []
        for day in days:
            id = int(datetime(2042, 1, day, tzinfo=UTC).timestamp())
            summary = SummaryData(MagicMock(), id, jobs=[ProjectPipelineJob(manager=MagicMock(), attrs={'id': day})])
            summary.write()
            summaries.append(summary)

        ids = [s.id for s in summaries]

        failure_summary.clean_summaries(MagicMock(), period=timedelta(days=10))
        new_ids = failure_summary.list_files(MagicMock())
        new_ids = sorted(SummaryData.get_id(name) for name in new_ids)

        # 6, 8, 10
        self.assertEqual(new_ids, ids[2:])
