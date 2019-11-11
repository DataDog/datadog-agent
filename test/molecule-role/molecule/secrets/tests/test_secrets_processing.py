import os
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_linux_vm')


def test_stackstate_agent_secret_output_no_datadog(host, common_vars):
    secret_cmd = host.run("sudo -u stackstate-agent -- stackstate-agent secret")
    print(secret_cmd)
    # assert that the status command ran successfully and that datadog is not contained in the output
    assert secret_cmd.rc == 0
    assert "Number of secrets decrypted: 2" in secret_cmd.stdout
    assert "api_key: from stackstate.yaml" in secret_cmd.stdout
    assert "url: from dummy_check" in secret_cmd.stdout
    assert "datadog" not in secret_cmd.stdout
    assert "Datadog" not in secret_cmd.stdout


def test_stackstate_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent enabled=true state=started")['changed']


def test_stackstate_process_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent-process state=started", become=True)['changed']


def test_stackstate_trace_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent-trace state=started", become=True)['changed']
