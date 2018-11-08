import os
import testinfra.utils.ansible_runner


testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(
    os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_vm')


def test_opt_stackstate_directory(host):
    f = host.file('/opt/datadog-agent/')
    assert f.is_directory
