import unittest
from unittest.mock import patch

from tasks.libs.pipeline.notifications import ProxyMap


class TestProxyMap(unittest.TestCase):
    def test_proxy_map_lazy_loading(self):
        # We want to verify that the packs are only called once we access the map
        with (
            patch('tasks.libs.pipeline.notifications.load_and_validate') as mock_load,
            patch('tasks.libs.pipeline.packs.get_team_channels') as mock_get_channels,
        ):
            mock_load.return_value = {"@datadog/team1": "#local-chan1", "@datadog/team2": "#local-chan2"}

            # Should NOT have called get_team_channels yet
            proxy = ProxyMap("dummy.yaml", "notification")
            self.assertEqual(mock_get_channels.call_count, 0)

            # Accessing an item should trigger initialization
            mock_get_channels.return_value = ("#pack-chan1", "#pack-review1")
            val = proxy["@datadog/team1"]

            self.assertEqual(val, "#pack-chan1")
            self.assertGreater(mock_get_channels.call_count, 0)

    def test_proxy_map_fallback(self):
        with (
            patch('tasks.libs.pipeline.notifications.load_and_validate') as mock_load,
            patch('tasks.libs.pipeline.packs.get_team_channels') as mock_get_channels,
        ):
            mock_load.return_value = {"@datadog/team1": "#local-chan1", "@datadog/team2": "#local-chan2"}

            # Pack returns None for team2, should fallback to local
            def side_effect(team):
                if team == "@datadog/team1":
                    return ("#pack-chan1", None)
                return (None, None)

            mock_get_channels.side_effect = side_effect

            proxy = ProxyMap("dummy.yaml", "notification")

            self.assertEqual(proxy["@datadog/team1"], "#pack-chan1")
            self.assertEqual(proxy["@datadog/team2"], "#local-chan2")

    def test_proxy_map_error_fallback(self):
        with (
            patch('tasks.libs.pipeline.notifications.load_and_validate') as mock_load,
            patch('tasks.libs.pipeline.packs.get_team_channels') as mock_get_channels,
        ):
            mock_load.return_value = {"@datadog/team1": "#local-chan1"}

            mock_get_channels.side_effect = Exception("Packs service down")

            proxy = ProxyMap("dummy.yaml", "notification")

            # Should still work and return local value
            self.assertEqual(proxy["@datadog/team1"], "#local-chan1")
