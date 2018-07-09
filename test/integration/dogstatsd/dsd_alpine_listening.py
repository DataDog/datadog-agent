import unittest
import docker
import os

# The DOCKER_IMAGE envvar is needed to specify what
# image to test

SOCKET_PATH = "/tmp/statsd.socket"

COMMON_ENVIRONMENT = [
    "DD_DD_URL=http://dummy",
    "DD_API_KEY=dummy",
]

ENVIRONMENTS = {
    "udp": [],
    "uds": [
        "DD_DOGSTATSD_SOCKET=" + SOCKET_PATH,
        "DD_DOGSTATSD_PORT=0"
    ],
    "both": [
        "DD_DOGSTATSD_SOCKET=" + SOCKET_PATH,
        "DD_DOGSTATSD_PORT=8125"
    ]
}

containers = {}
client = {}


def setUpModule():
    global containers
    global client

    client = docker.from_env()

    for name, env in ENVIRONMENTS.iteritems():
        containers[name] = client.containers.run(
            os.environ.get('DOCKER_IMAGE'),
            detach=True,
            environment=COMMON_ENVIRONMENT + env,
            auto_remove=True
        )


def tearDownModule():
    global containers
    global client

    for _, container in containers.iteritems():
        container.stop()


def waitUntilListening(container, retries=20):
    for x in range(0, retries):
        out = container.exec_run(cmd="netstat -a").output
        if ":8125" in out or SOCKET_PATH in out:
            return True
    return False


def isUDPListening(container):
    out = container.exec_run(cmd="netstat -a").output
    return ":8125" in out


def isUDSListening(container, retries=10):
    out = container.exec_run(cmd="netstat -a").output
    return SOCKET_PATH in out


class DSDStaticTest(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(os.environ.get('DOCKER_IMAGE'), "DOCKER_IMAGE envvar needed")

    def test_static_binary(self):
        '''Fails if /dogstatsd is not a static binary, build options are likely broken'''
        global client
        fileOutput = client.containers.run(
            os.environ.get('DOCKER_IMAGE'),
            environment=COMMON_ENVIRONMENT,
            auto_remove=True,
            stdout=True,
            command='sh -c "apk add --no-cache file && file /dogstatsd"'
        )
        self.assertIn("statically linked", fileOutput)


class DSDListeningTest(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(os.environ.get('DOCKER_IMAGE'), "DOCKER_IMAGE envvar needed")

    def test_udp(self):
        self.assertTrue(waitUntilListening(containers["udp"]))
        self.assertTrue(isUDPListening(containers["udp"]))
        self.assertFalse(isUDSListening(containers["udp"]))

    def test_uds(self):
        self.assertTrue(waitUntilListening(containers["uds"]))
        self.assertFalse(isUDPListening(containers["uds"]))
        self.assertTrue(isUDSListening(containers["uds"]))

    def test_both(self):
        self.assertTrue(waitUntilListening(containers["both"]))
        self.assertTrue(isUDPListening(containers["both"]))
        self.assertTrue(isUDSListening(containers["both"]))


if __name__ == '__main__':
    unittest.main()
