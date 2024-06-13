import os
import shutil
from unittest import TestCase
from unittest.mock import patch, MagicMock

from invoke import Context

from tasks.libs.pipeline import failure_summary


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
        print('Writing file', name)

        with open(f'{TEST_DIR}/{name}', 'w') as f:
            f.write(data)

    def read_file(self, ctx: Context, name: str) -> str:
        print('Reading file', name)

        with open(f'{TEST_DIR}/{name}') as f:
            return f.read()

    def remove_files(self, ctx: Context, names: list[str]):
        print('Removing files', names)

        os.system(f'rm -f {TEST_DIR}/{{{",".join(names)}}}')

    def list_files(self, ctx: Context) -> list[str]:
        return os.listdir(TEST_DIR)


class SummaryDataTest(FailureSummaryTest):
    def test_list_summaries(self):
        failure_summary.write_file(None, 'hello', 'world')

