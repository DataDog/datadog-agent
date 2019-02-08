import os
import re

import testinfra.utils.ansible_runner

testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(
    os.environ["MOLECULE_INVENTORY_FILE"]).get_hosts("agent_win_vm")


def test_stackstate_agent_is_installed(host):
    pkg = "Datadog Agent"  # TODO
    res = host.ansible("win_shell", "Get-Package \"{}\"".format(pkg), check=False)
    print res
    # Name             Version
    # ----             -------
    # Datadog Agent    2.x
    assert re.search(".*{}\\s+2\\.".format(pkg), res["stdout"], re.I)


def test_stackstate_agent_running_and_enabled(host):
    def check(name, deps, depended_by):
        service = host.ansible("win_service", "name={} state=started".format(name))
        print service
        assert service["exists"]
        # assert not service["changed"]  # TODO
        # assert service["state"] == "running"  # TODO
        assert service["dependencies"] == deps
        assert service["depended_by"] == depended_by

    check("datadogagent", ["winmgmt"], ["datadog-process-agent", "datadog-trace-agent"])
    check("datadog-process-agent", ["datadogagent"], [])
