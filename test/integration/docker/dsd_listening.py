import unittest
import docker
import os
import time

# The DOCKER_IMAGE envvar is needed to specify what
# image to test

SOCKET_PATH = "/tmp/statsd.socket"


# Main class
class dsdTest(unittest.TestCase):
    client = None
    environment = []
    container = None

    common_environment = [
        "DD_DD_URL=http://dummy",
        "DD_API_KEY=dummy",
    ]

    def setUp(self):
        self.assertIsNotNone(os.environ.get('DOCKER_IMAGE'), "DOCKER_IMAGE envvar needed")
        self.client = docker.from_env()

        self.container = self.client.containers.run(
            os.environ.get('DOCKER_IMAGE'),
            detach=True,
            environment=self.environment + self.common_environment,
            auto_remove=True
        )
        time.sleep(1)

    def tearDown(self):
        if self.container:
            self.container.stop()

    def isUDPListening(self):
        out = self.container.exec_run(cmd="netstat -a")
        return ":8125" in out

    def isUDSListening(self):
        out = self.container.exec_run(cmd="netstat -a")
        return SOCKET_PATH in out


class TestUDP(dsdTest):
    environment = []

    def test_listens(self):
        self.assertTrue(self.isUDPListening())
        self.assertFalse(self.isUDSListening())


class TestUDS(dsdTest):
    environment = [
        "DD_DOGSTATSD_SOCKET=" + SOCKET_PATH,
        "DD_DOGSTATSD_PORT=0"
    ]

    def test_listens(self):
        self.assertFalse(self.isUDPListening())
        self.assertTrue(self.isUDSListening())


class TestBoth(dsdTest):
    environment = [
        "DD_DOGSTATSD_SOCKET=" + SOCKET_PATH,
        "DD_DOGSTATSD_PORT=8125"
    ]

    def test_listens(self):
        self.assertTrue(self.isUDPListening())
        self.assertTrue(self.isUDSListening())


if __name__ == '__main__':
    unittest.main()
