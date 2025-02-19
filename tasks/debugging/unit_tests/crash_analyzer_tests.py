import shutil
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import MagicMock, patch

from tasks.debugging.gitlab_artifacts import Artifacts


class TestCrashAnalyzer(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix='test-store-'))

    def tearDown(self):
        shutil.rmtree(self.root)

    def test_get_symbols_for_job_id(self):
        """
        Test that we can get symbols for a job id from a (mock) pipeline
        """
        from tasks.debugging.dump import CrashAnalyzer, get_symbols_for_job_id

        AGENT_VERSION = '7.56.0.git.123.123.pipeline.123'

        def _patched_download_job_artifacts(project, jobid, path):
            """
            Mock/patch to "download" artifacts without involving gitlab api
            """
            if jobid == 'package-job-id':
                p = Path(path, f'datadog-agent-{AGENT_VERSION}-1-x86_64.debug.zip')
                with zipfile.ZipFile(p, 'w') as z:
                    z.writestr('agent.exe.debug', 'fake symbols')
            else:
                Path(path, 'test.txt').write_text('fake artifacts')

        def _patched_get_package_job_id(*args):
            """
            Mock/patch to return a static job id as the package job without involving gitlab api
            """
            return 'package-job-id'

        project = MagicMock()
        project.name = 'project'
        ca = CrashAnalyzer(self.root, 'windows', 'x86_64')
        ca.select_project(project)

        with (
            patch('tasks.debugging.dump.download_job_artifacts', side_effect=_patched_download_job_artifacts),
            patch('tasks.debugging.dump.get_package_job_id', side_effect=_patched_get_package_job_id),
        ):
            version, syms = get_symbols_for_job_id(ca, 'jobid')

        self.assertEqual(version, '7.56.0.git.123.123.pipeline.123')
        # check that we got the symbols
        self.assertIsInstance(syms, Path)
        assert syms is not None
        self.assertTrue(syms.exists())
        dbg_path = Path(syms, 'agent.exe.debug')
        self.assertTrue(dbg_path.exists())
        self.assertEqual(dbg_path.read_text(), 'fake symbols')
        # check that we got the artifacts for the package job
        artifact = ca.artifact_store.get(project.name, 'package-job-id')
        self.assertIsInstance(artifact, Artifacts)
        assert artifact is not None
        path = artifact.get()
        assert path is not None
        self.assertTrue(path.exists())
        dbg_path = Path(path, f'datadog-agent-{AGENT_VERSION}-1-x86_64.debug.zip')
        self.assertTrue(dbg_path.exists())
        # check that we set the version property
        self.assertEqual(artifact.version, AGENT_VERSION)
        # check that we didn't download artifacts for jobid, only package-job-id
        artifact = ca.artifact_store.get(project.name, 'jobid')
        self.assertIsInstance(artifact, Artifacts)
        assert artifact is not None
        self.assertEqual(artifact.version, AGENT_VERSION)
        self.assertIsNone(artifact.get())
