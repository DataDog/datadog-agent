import os
import shutil
from contextlib import contextmanager
from unittest import TestCase
from unittest.mock import MagicMock, patch

from gitlab.v4.objects import ProjectPipelineJob, ProjectPipeline, ProjectManager
from invoke import Context

from tasks.libs.pipeline import failure_summary
from tasks.libs.pipeline.failure_summary import SummaryData

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

    def get_dummy_summary_data(self, job_ids: list[int], id=618) -> SummaryData:
        jobs = [ProjectPipelineJob(manager=MagicMock(), attrs={'id': id}) for id in job_ids]

        return SummaryData(MagicMock(), id, jobs=jobs, pipeline=ProjectPipeline(manager=MagicMock(), attrs={'id': 42}))

    @contextmanager
    def patch_fetch_jobs(self, job_ids: list[int]):
        p = patch('tasks.libs.pipeline.failure_summary.fetch_jobs', return_value=self.get_dummy_summary_data(job_ids))
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
        data = self.get_dummy_summary_data([1, 2, 3], id=618)
        data.write()

        data = SummaryData.read(MagicMock(), MagicMock(), 618)
        self.assertEqual(len(data.jobs), 3)
        self.assertEqual(data.jobs[0].id, 1)
        self.assertEqual(data.jobs[1].id, 2)
        self.assertEqual(data.jobs[2].id, 3)

    def test_list_write_summaries(self):
        self.get_dummy_summary_data([1, 2, 3], id=314).write()
        self.get_dummy_summary_data([4, 5, 6], id=618).write()

        summaries = SummaryData.list_summaries(MagicMock())

        self.assertEqual(len(summaries), 2)
        self.assertEqual(summaries[0], 314)
        self.assertEqual(summaries[1], 618)

    def test_list_write_summaries_before(self):
        self.get_dummy_summary_data([1, 2, 3], id=314).write()
        self.get_dummy_summary_data([4, 5, 6], id=618).write()

        summaries = SummaryData.list_summaries(MagicMock(), before=500)

        self.assertEqual(len(summaries), 1)
        self.assertEqual(summaries[0], 314)

    def test_list_write_summaries_after(self):
        self.get_dummy_summary_data([1, 2, 3], id=314).write()
        self.get_dummy_summary_data([4, 5, 6], id=618).write()

        summaries = SummaryData.list_summaries(MagicMock(), after=500)

        self.assertEqual(len(summaries), 1)
        self.assertEqual(summaries[0], 618)

    def test_merge_summaries(self):
        summary = SummaryData.merge([
            self.get_dummy_summary_data([1, 2, 3], id=314),
            self.get_dummy_summary_data([4, 5, 6], id=618),
            self.get_dummy_summary_data([7, 8], id=1618),
        ])
        self.assertEqual(len(summary.jobs), 8)
        self.assertEqual(summary.id, None)


class HighLevelTest(FailureSummaryTest):
    def test_upload_summary(self):
        # Upload summary then retrieve the summary and expect 2 jobs
        with self.patch_fetch_jobs([1, 2]):
            summary_id = failure_summary.upload_summary(None, None).id
            summary = SummaryData.read(None, MagicMock(), summary_id)

            self.assertEqual(len(summary.jobs), 2)
            self.assertEqual(summary.jobs[0].id, 1)
            self.assertEqual(summary.jobs[1].id, 2)

