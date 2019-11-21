import os
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_linux_vm')


def test_stackstate_agent_is_installed(host, common_vars):
    agent = host.package("stackstate-agent")
    print(agent.version)
    assert agent.is_installed

    agent_current_branch = common_vars["agent_current_branch"]
    if agent_current_branch == "master":
        assert agent.version.startswith("2")


def test_stackstate_agent_status_output_no_datadog(host):
    status_cmd = host.run("sudo -u stackstate-agent -- stackstate-agent status")
    print(status_cmd)
    # assert that the status command ran successfully and that datadog is not contained in the output
    assert status_cmd.rc == 0
    assert "datadog" not in status_cmd.stdout
    assert "Datadog" not in status_cmd.stdout

    help_cmd = host.run("sudo -u stackstate-agent -- stackstate-agent --help")
    print(help_cmd)
    # assert that the help command ran successfully and that datadog is not contained in the output
    assert help_cmd.rc == 0
    assert "datadog" not in help_cmd.stdout
    assert "Datadog" not in help_cmd.stdout


def test_stackstate_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent enabled=true state=started")['changed']


def test_stackstate_process_agent_running_and_enabled(host):
    # We don't check enabled because on systemd redhat is not needed check omnibus/package-scripts/agent/posttrans
    assert not host.ansible("service", "name=stackstate-agent-process state=started", become=True)['changed']


def test_stackstate_trace_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent-trace state=started", become=True)['changed']


def test_agent_namespaces_docker(host):
    hostname = host.ansible.get_variables()["inventory_hostname"]
    if hostname == "agent-connection-namespaces":
        f = host.file('/etc/docker/')
        assert f.is_directory
    else:
        pass
