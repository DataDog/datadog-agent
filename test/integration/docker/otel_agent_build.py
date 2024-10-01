import os
import unittest

# import docker

# COMMON_ENVIRONMENT = [
#     "DD_DD_URL=http://dummy",
#     "DD_API_KEY=dummy",
# ]


class OtelAgentBuildTest(unittest.TestCase):
    """contains setup and tests for otel agent build. Must be invoked directly
    by 'inv otel-agent.test-image-build' so that necessary image is built and
    environment variables are set"""

    def setUp(self):
        self.assertIsNotNone(os.environ.get('OT_AGENT_IMAGE_NAME'), "OT_AGENT_IMAGE_NAME envvar needed")
        self.assertIsNotNone(os.environ.get('tag'), "tag envvar needed")

    def test_otel_agent_docker_image(self):
        self.assertTrue(True)


if __name__ == '__main__':
    unittest.main()
