import os
import re
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')

kubeconfig_env = "KUBECONFIG=/home/ubuntu/deployment/aws-eks/tf-cluster/kubeconfig "


def _get_pods(host, controller_name):
    jsonpath = "'{.items[?(@.spec.containers[*].name==\"%s\")].metadata.name}'" % controller_name
    cmd = host.run(kubeconfig_env + "kubectl get pod -o jsonpath=" + jsonpath)
    assert cmd.rc == 0
    pods = cmd.stdout.split()
    print(pods)
    return pods


def _get_log(host, pod):
    cmd = host.ansible("shell", kubeconfig_env + "/usr/local/bin/kubectl logs " + pod, check=False)
    assert cmd["rc"] == 0
    stackstate_agent_log = cmd["stdout"]
    with open("./stackstate-agent-%s.log" % pod, 'wb') as f:
        f.write(stackstate_agent_log.encode('utf-8'))
    return stackstate_agent_log


def _check_logs(host, controller_name, success_regex, ignored_errors):
    def wait_for_successful_post():
        for pod in _get_pods(host, controller_name):
            log = _get_log(host, pod)
            assert re.search(success_regex, log)

    util.wait_until(wait_for_successful_post, 30, 3)

    for pod in _get_pods(host, controller_name):
        log = _get_log(host, pod)
        for line in log.splitlines():
            ignored = False
            for ignored_error in ignored_errors:
                if ignored_error in line:
                    ignored = True
            if ignored:
                continue
            print("Considering: %s" % line)
            assert not re.search("error", line, re.IGNORECASE)


def test_stackstate_agent_log_no_errors(host):
    ignored_errors = [
        "No handler function named",
        "error querying the ntp"
    ]
    _check_logs(host, "stackstate-agent", "Successfully posted payload to.*stsAgent/api/v1", ignored_errors)


def test_stackstate_cluster_agent_log_no_errors(host):
    ignored_errors = [
        "configmap:kube-system:coredns"  # this configmap container the word `errors`
    ]
    _check_logs(host, "stackstate-cluster-agent", "Sent processes metadata payload", ignored_errors)
