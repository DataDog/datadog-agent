import json
import unittest
from os import path
from pathlib import Path
from unittest import mock

from tasks.libs.common.docker_hub import get_latest_version

HERE = Path(path.dirname(path.abspath(__file__)))


class TestDockerHub(unittest.TestCase):
    @mock.patch('requests.get')
    def test_get_latest_version(self, _mock_get):
        _mock_get.return_value = mock.Mock()
        _mock_get.return_value.return_code = 200
        _mock_get.return_value.json.return_value = json.loads(
            (HERE / 'fixtures' / 'agent-package-version.json').read_text()
        )

        version = get_latest_version("agent-package")

        self.assertEqual(version, "7.55.2-1")
