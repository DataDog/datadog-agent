import os
import re
import util
import testinfra.utils.ansible_runner

testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(
    os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent_vm')


def test_stackstate_agent_is_installed(host):
    agent = host.package("stackstate-agent")
    assert agent.is_installed
    print agent.version
    assert agent.version.startswith("2")


def test_stackstate_agent_running_and_enabled(host):
    agent = host.service("stackstate-agent")
    assert agent.is_running
    assert agent.is_enabled


def test_stackstate_process_agent_running_and_enabled(host):
    process_agent = host.service("stackstate-agent-process")
    assert process_agent.is_running
    assert process_agent.is_enabled


def test_stackstate_agent_log(host):
    # Wait some time for stuff to enter the logs
    agent_log_path = "/var/log/stackstate-agent/agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        agent_log = host.file(agent_log_path).content_string
        print agent_log

        assert re.search("Sent host metadata payload", agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    agent_log = host.file(agent_log_path).content_string

    # Check for errors
    for line in agent_log.splitlines():
        print("Considering: %s" % line)
        # TODO: Update event endpoint should get rid of this error,
        # once STAC-2500 is fixed
        if re.search(
                "Error code \"400 Bad Request\" received while sending " +
                "transaction to \"http://.*:7077/stsAgent/intake/",
                line):
            continue

        # https://stackstate.atlassian.net/browse/STAC-3202 first
        assert not re.search("\| error \|", line, re.IGNORECASE)


def test_stackstate_process_agent_no_log_errors(host):
    # Wait some time for stuff to enter the logs
    process_agent_log_path = "/var/log/stackstate-agent/process-agent.log"

    # Check for presence of success
    def wait_for_check_successes():
        process_agent_log = host.file(process_agent_log_path).content_string
        print process_agent_log

        assert re.search("Finished check #1", process_agent_log)
        assert re.search("starting network tracer locally", process_agent_log)

    util.wait_until(wait_for_check_successes, 30, 3)

    process_agent_log = host.file(process_agent_log_path).content_string

    # Check for errors
    for line in process_agent_log.splitlines():
        print("Considering: %s" % line)
        assert not re.search("error", line, re.IGNORECASE)
