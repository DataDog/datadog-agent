import unittest
from unittest.mock import patch

from tasks.commands.interface import CLI


class TestLocalInterface(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.interface = CLI()

    @patch('invoke.run')
    def test_run_command(self, mock):
        self.interface.run_command(["echo", "Hello, World!"])
        mock.assert_called_once_with("echo 'Hello, World!'", pty=True)
