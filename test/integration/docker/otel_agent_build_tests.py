import os
import unittest

import docker


def setUpModule():
    global client
    client = docker.from_env()


class OtelAgentBuildTest(unittest.TestCase):
    """contains setup and tests for otel agent build. Must be invoked directly
    by 'inv otel-agent.test-image-build' so that necessary image is built and
    environment variables are set"""

    def setUp(self):
        self.assertIsNotNone(os.environ.get('OT_AGENT_IMAGE_NAME'), "OT_AGENT_IMAGE_NAME envvar needed")
        self.image_name = os.environ.get('OT_AGENT_IMAGE_NAME')
        self.assertIsNotNone(os.environ.get('OT_AGENT_TAG'), "OT_AGENT_TAG envvar needed")
        self.tag = os.environ.get('OT_AGENT_TAG')
        # self.assertIsNotNone(os.environ.get('EXPECTED_VERSION'), "EXPECTED_VERSION envvar needed")
        # self.expected_version = os.environ.get('EXPECTED_VERSION')

    def test_otel_agent_docker_image(self):
        version_output = client.containers.run(
            f'{self.image_name}:{self.tag}', entrypoint='otel-agent', command='version'
        )
        self.assertIn("otel-agent", version_output.decode('utf-8'))
        # TODO: replace with expected version code when https://github.com/DataDog/datadog-agent/pull/29334
        # is merged to otel beta branch
        # self.assertIn(f"otel-agent {self.expected_version}", version_output.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
