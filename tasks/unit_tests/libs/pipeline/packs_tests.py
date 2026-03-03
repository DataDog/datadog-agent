import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.pipeline.packs import fetch_all_packs, get_team_channels


class TestPacks(unittest.TestCase):
    @patch('tasks.libs.pipeline.packs.subprocess.check_output')
    @patch('tasks.libs.pipeline.packs.os.environ', {})
    def test_get_token_ddtool(self, mock_subprocess):
        from tasks.libs.pipeline.packs import get_token

        mock_subprocess.return_value = b"fake-token\n"
        token = get_token()
        self.assertEqual(token, "fake-token")
        mock_subprocess.assert_called_once()

    @patch('tasks.libs.pipeline.packs.os.environ', {'HOLOCENE_TOKEN': 'env-token'})
    def test_get_token_env(self):
        from tasks.libs.pipeline.packs import get_token

        token = get_token()
        self.assertEqual(token, "env-token")

    @patch('tasks.libs.pipeline.packs.requests.get')
    @patch('tasks.libs.pipeline.packs.get_token')
    def test_fetch_all_packs(self, mock_get_token, mock_get):
        mock_get_token.return_value = "token"

        # Mocking two pages of results
        mock_response1 = MagicMock()
        mock_response1.status_code = 200
        mock_response1.json.return_value = {"packs": [{"id": "pack1"}], "next_page_token": "page2"}

        mock_response2 = MagicMock()
        mock_response2.status_code = 200
        mock_response2.json.return_value = {"packs": [{"id": "pack2"}], "next_page_token": None}

        mock_get.side_effect = [mock_response1, mock_response2]

        # Clear cache before test
        fetch_all_packs.cache_clear()
        packs = fetch_all_packs()

        self.assertEqual(len(packs), 2)
        self.assertEqual(packs[0]["id"], "pack1")
        self.assertEqual(packs[1]["id"], "pack2")
        self.assertEqual(mock_get.call_count, 2)

    @patch('tasks.libs.pipeline.packs.get_packs_map')
    def test_get_team_channels(self, mock_get_packs_map):
        mock_get_packs_map.return_value = {
            "agent-devx": {"notification_channel": "C1", "review_channel": "C2", "contact_channel": "C3"},
            "no-notif": {"review_channel": "C2", "contact_channel": "C3"},
            "only-contact": {"contact_channel": "C3"},
        }

        # Test full definition
        notif, review = get_team_channels("@datadog/agent-devx")
        self.assertEqual(notif, "C1")
        self.assertEqual(review, "C2")

        # Test fallback to contact_channel for notification
        notif, review = get_team_channels("@datadog/no-notif")
        self.assertEqual(notif, "C3")
        self.assertEqual(review, "C2")

        # Test fallback to contact_channel for both
        notif, review = get_team_channels("@datadog/only-contact")
        self.assertEqual(notif, "C3")
        self.assertEqual(review, "C3")

        # Test unknown team
        notif, review = get_team_channels("@datadog/unknown")
        self.assertIsNone(notif)
        self.assertIsNone(review)
