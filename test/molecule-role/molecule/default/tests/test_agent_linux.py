import os
import re
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_linux_vm')


def test_stackstate_agent_is_installed(host, common_vars):
    agent = host.package("stackstate-agent")
    print(agent.version)
    assert agent.is_installed

    agent_current_branch = common_vars["agent_current_branch"]
    if agent_current_branch == "master":
        assert agent.version.startswith("2")


def test_stackstate_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent enabled=true state=started")['changed']


def test_stackstate_process_agent_running_and_enabled(host):
    # We don't check enabled because on systemd redhat is not needed check omnibus/package-scripts/agent/posttrans
    assert not host.ansible("service", "name=stackstate-agent-process state=started", become=True)['changed']


def test_stackstate_trace_agent_running_and_enabled(host):
    assert not host.ansible("service", "name=stackstate-agent-trace state=started", become=True)['changed']


def test_stackstate_agent_log(host, hostname):
    agent_log_path = "/var/log/stackstate-agent/agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        agent_log = host.file(agent_log_path).content_string
        print(agent_log)
        assert re.search("Sent host metadata payload", agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    agent_log = host.file(agent_log_path).content_string
    with open("./{}.log".format(hostname), 'w') as f:
        f.write(agent_log.encode('utf-8'))

    # Check for errors
    for line in agent_log.splitlines():
        print("Considering: %s" % line)
        # TODO: Collecting processes snap -> Will be addressed with STAC-3531
        if re.search("Error code \"400 Bad Request\" received while "
                     "sending transaction to \"https://.*/stsAgent/intake/.*"
                     "Failed to deserialize JSON on fields: , "
                     "with message: Object is missing required member \'internalHostname\'",
                     line):
            continue

        # https://stackstate.atlassian.net/browse/STAC-3202 first
        assert not re.search("\\| error \\|", line, re.IGNORECASE)


def test_stackstate_process_agent_no_log_errors(host, hostname):
    process_agent_log_path = "/var/log/stackstate-agent/process-agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        process_agent_log = host.file(process_agent_log_path).content_string
        print(process_agent_log)

        assert re.search("Finished check #1", process_agent_log)
        if hostname != "agent-centos":
            assert re.search("starting network tracer locally", process_agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    process_agent_log = host.file(process_agent_log_path).content_string
    with open("./{}-process.log".format(hostname), 'w') as f:
        f.write(process_agent_log.encode('utf-8'))

    # Check for errors
    for line in process_agent_log.splitlines():
        print("Considering: %s" % line)
        assert not re.search("error", line, re.IGNORECASE)


def test_stackstate_trace_agent_no_log_errors(host, hostname):
    trace_agent_log_path = "/var/log/stackstate-agent/trace-agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        trace_agent_log = host.file(trace_agent_log_path).content_string
        print(trace_agent_log)

        assert re.search("total number of tracked services", trace_agent_log)
        assert re.search("trace-agent running on host", trace_agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    trace_agent_log = host.file(trace_agent_log_path).content_string
    with open("./{}-trace.log".format(hostname), 'w') as f:
        f.write(trace_agent_log.encode('utf-8'))

    # Check for errors
    for line in trace_agent_log.splitlines():
        print("Considering: %s" % line)
        assert not re.search("error", line, re.IGNORECASE)


def test_agent_namespaces_docker(host):
    hostname = host.ansible.get_variables()["inventory_hostname"]
    if hostname == "agent-connection-namespaces":
        f = host.file('/etc/docker/')
        assert f.is_directory
    else:
        pass
