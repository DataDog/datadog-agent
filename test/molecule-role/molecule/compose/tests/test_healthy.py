import os
import pytest
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('trace-java-demo')


@pytest.mark.first
def test_receiver_healthy(host):
    def assert_healthy():
        c = "curl -s -o /dev/null -w \"%{http_code}\" http://localhost:7077/health"
        assert host.check_output(c) == "200"

    util.wait_until(assert_healthy, 100, 5)


@pytest.mark.second
def test_agent_ok(host):
    def assert_healthy():
        c = "docker inspect ubuntu_stackstate-agent_1 |  jq -r '.[0].State.Health.Status'"
        assert host.check_output(c) == "healthy"

    util.wait_until(assert_healthy, 100, 5)
