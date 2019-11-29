import os
import re
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_linux_vm')


def _get_log(host, hostname, agent_log_path):
    agent_log = host.file(agent_log_path).content_string
    with open("./{}.log".format(hostname), 'wb') as f:
        f.write(agent_log.encode('utf-8'))
    return agent_log


def test_stackstate_agent_log(host, hostname):
    agent_log_path = "/var/log/stackstate-agent/agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        agent_log = _get_log(host, hostname, agent_log_path)
        assert re.search("Successfully posted payload to.*stsAgent/api/v1", agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    # Check for errors
    agent_log = _get_log(host, hostname, agent_log_path)
    for line in agent_log.splitlines():
        print("Considering: %s" % line)
        # TODO: Collecting processes snap -> Will be addressed with STAC-3531
        if re.search("Error code \"400 Bad Request\" received while "
                     "sending transaction to \"https://.*/stsAgent/intake/.*"
                     "Failed to deserialize JSON on fields: , "
                     "with message: Object is missing required member \'internalHostname\'",
                     line):
            continue

        assert not re.search("error", line, re.IGNORECASE)


def test_stackstate_process_agent_no_log_errors(host, hostname):
    process_agent_log_path = "/var/log/stackstate-agent/process-agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        process_agent_log = _get_log(host, hostname, process_agent_log_path)
        assert re.search("Finished check #1", process_agent_log)
        if hostname != "agent-centos":
            assert re.search("starting network tracer locally", process_agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    # Check for errors
    process_agent_log = _get_log(host, hostname, process_agent_log_path)
    for line in process_agent_log.splitlines():
        print("Considering: %s" % line)
        assert not re.search("error", line, re.IGNORECASE)


def test_stackstate_trace_agent_no_log_errors(host, hostname):
    trace_agent_log_path = "/var/log/stackstate-agent/trace-agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        trace_agent_log = _get_log(host, hostname, trace_agent_log_path)
        assert re.search("total number of tracked services", trace_agent_log)
        assert re.search("trace-agent running on host", trace_agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    # Check for errors
    trace_agent_log = _get_log(host, hostname, trace_agent_log_path)
    for line in trace_agent_log.splitlines():
        print("Considering: %s" % line)
        assert not re.search("error", line, re.IGNORECASE)
