import unittest
from unittest.mock import patch

from tasks.commands.docker import DockerCLI


class TestDockerCLI(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.interface = DockerCLI("my-container")

    @patch('invoke.run')
    def test_run_command(self, mock):
        self.interface.run_command(["echo", "Hello, World!"])
        mock.assert_called_once()

        # This test may be launched from a tty or not
        self.assertIn(
            mock.call_args[0][0],
            [
                "docker exec -it -w /workspaces/datadog-agent my-container echo 'Hello, World!'",
                "docker exec -w /workspaces/datadog-agent my-container echo 'Hello, World!'",
            ],
        )
