# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
import unittest

import datadog_agent


class TestAgent(unittest.TestCase):
    def test_agent_version(self):
        self.assertEqual(datadog_agent.get_version(), "6.0.0")

    def test_get_config(self):
        self.assertEqual(datadog_agent.get_config('dd_url'), "https://test.datadoghq.com")

    def test_headers(self):
        self.assertEqual(
            datadog_agent.headers(),
            {
                'Content-Type': 'application/x-www-form-urlencoded',
                'Accept': 'text/html, */*',
                'User-Agent': 'Datadog Agent/6.0.0',
            },
        )

    def test_get_hostname(self):
        self.assertEqual(datadog_agent.get_hostname(), "test.hostname")


if __name__ == '__main__':
    unittest.main()
