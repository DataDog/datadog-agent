import shutil
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from tasks.debugging.gitlab_artifacts import Artifacts, ArtifactStore


class TestArtifactStore(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix='test-store-'))
        self.artifact_store = ArtifactStore(Path(self.root, 'artifacts'))

    def tearDown(self):
        shutil.rmtree(self.root)

    # Test that we can add an artifact to the store
    def test_add_artifacts(self):
        """
        Test that we can add artifacts to the store
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            Path(tmpdir, 'test.txt').touch()
            artifact = self.artifact_store.add('project', 'jobid', tmpdir)
        self.assertIsInstance(artifact, Artifacts)
        # artifacts organized by project/jobid
        self.assertTrue(Path(self.root, 'artifacts', 'project', 'jobid', 'artifacts', 'test.txt').exists())
        self.assertEqual(None, artifact.version)
        # get artifacts
        artifact = self.artifact_store.get('project', 'jobid')
        self.assertIsInstance(artifact, Artifacts)
        assert artifact is not None
        path = artifact.get()
        assert path is not None
        self.assertTrue(path.exists())
        self.assertEqual(None, artifact.version)

    def test_add_job_properties_without_artifacts(self):
        """
        Test that we can add job properties without artifacts
        """
        # add artifact with version property
        artifact = self.artifact_store.add('project', 'jobid')
        artifact.version = '1.0'
        version_path = Path(self.root, 'artifacts', 'project', 'jobid', 'version.txt')
        self.assertTrue(version_path.exists())
        self.assertEqual('1.0', version_path.read_text())
        # fetch artifact and check version property
        artifact = self.artifact_store.get('project', 'jobid')
        self.assertIsInstance(artifact, Artifacts)
        assert artifact is not None
        self.assertEqual(artifact.version, '1.0')

    def test_get_or_fetch_artifacts(self):
        """
        Test that get_or_fetch_artifacts will fetch artifacts if they don't exist
        """
        from tasks.debugging.dump import get_or_fetch_artifacts

        def _patched_download_job_artifacts(project, jobid, path):
            Path(path, 'test.txt').write_text('fake artifacts')

        project = MagicMock()
        project.name = 'project'
        with patch('tasks.debugging.dump.download_job_artifacts', side_effect=_patched_download_job_artifacts):
            artifact = get_or_fetch_artifacts(self.artifact_store, project, 'jobid')
        self.assertIsInstance(artifact, Artifacts)
        self.assertEqual(None, artifact.version)
        expected_path = Path(self.root, 'artifacts', 'project', 'jobid', 'artifacts')
        self.assertTrue(expected_path.exists())
        path = artifact.get()
        self.assertIsInstance(path, Path)
        self.assertEqual(expected_path, path)
        assert path is not None
        artifact_path = Path(path, 'test.txt')
        self.assertTrue(artifact_path.exists())
        self.assertEqual('fake artifacts', artifact_path.read_text())

        # add test case that ensures download_job_artifacts is not called now that the artifacts exist
        with patch('tasks.debugging.dump.download_job_artifacts') as download_job_artifacts:
            artifact = get_or_fetch_artifacts(self.artifact_store, project, 'jobid')
            download_job_artifacts.assert_not_called()
        self.assertIsInstance(artifact, Artifacts)
        path = artifact.get()
        self.assertIsInstance(path, Path)
        assert path is not None
        artifact_path = Path(path, 'test.txt')
        self.assertEqual('fake artifacts', artifact_path.read_text())

    def test_get_or_fetch_artifacts_when_job_has_no_artifacts(self):
        """
        Test that get_or_fetch_artifacts still returns an Artifacts object when the job has no artifacts

        We may still want to get the version property, to find symbols, for example
        """
        from tasks.debugging.dump import get_or_fetch_artifacts

        def _patched_download_job_artifacts(project, jobid, path):
            pass

        project = MagicMock()
        project.name = 'project'
        with patch('tasks.debugging.dump.download_job_artifacts', side_effect=_patched_download_job_artifacts):
            artifact = get_or_fetch_artifacts(self.artifact_store, project, 'jobid')
        self.assertIsInstance(artifact, Artifacts)
        path = artifact.get()
        self.assertIsNone(path)
