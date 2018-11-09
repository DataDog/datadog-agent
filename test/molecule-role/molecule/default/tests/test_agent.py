import os
import testinfra.utils.ansible_runner

testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(
    os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_vm')


def test_stackstate_agent_is_installed(host):
    agent = host.package("stackstate-agent")
    assert agent.is_installed
    # TODO
    # assert agent.version.startswith("2")


def test_stackstate_agent_running_and_enabled(host):
    agent = host.service("stackstate-agent")
    assert agent.is_running
    assert agent.is_enabled
