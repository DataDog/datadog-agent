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


def _check_logs(host, controller_name, success_regex, ignored_errors_regex):
    def wait_for_successful_post():
        for pod in _get_pods(host, controller_name):
            log = _get_log(host, pod)
            assert re.search(success_regex, log)

    util.wait_until(wait_for_successful_post, 30, 3)

    for pod in _get_pods(host, controller_name):
        log = _get_log(host, pod)
        for line in log.splitlines():
            ignored = False
            for ignored_error in ignored_errors_regex:
                if len(re.findall(ignored_error, line, re.DOTALL)) > 0:
                    ignored = True
            if ignored:
                continue
            print("Considering: %s" % line)
            assert not re.search("error", line, re.IGNORECASE)


def test_stackstate_agent_log_no_errors(host):
    ignored_errors_regex = [
        "ecs/ecs.go.*No such container: ecs-agent",
        "ecs/ecs.go.*temporary failure in ecsutil",
        "ecs/ecs.go.*ECS init error",
        "util/hostname.go.*ValidHostname.*is not RFC1123 compliant",
        "cri/util.go.*temporary failure in criutil",
        "py/datadog_agent.go.*No handler function named",
        "collector/collector.go.*No module named psutil",  # TODO this should not happen!
        "net/ntp.go.*There was an error querying the ntp host",
        "clusteragent/clusteragent.go.*temporary failure in clusterAgentClient",  # happens when agents start together
        "collectors/kubernetes_main.go.*temporary failure in clusterAgentClient",
        "kubernetes/apiserver/apiserver.go.*temporary failure in apiserver",

    ]
    _check_logs(host, "stackstate-agent", "Successfully posted payload to.*stsAgent/api/v1", ignored_errors_regex)


def test_stackstate_cluster_agent_log_no_errors(host):
    ignored_errors_regex = [
        "hostname.go.*ValidHostname.*is not RFC1123 compliant",
        "kubeapi/kubernetes_topology_config.go.*urn:/kubernetes.*configmap:kube-system:coredns",  # this configmap container the word `errors`
        "serializer/serializer.go.*urn:/kubernetes.*configmap:kube-system:coredns",
    ]
    _check_logs(host, "stackstate-cluster-agent", "Sent processes metadata payload", ignored_errors_regex)
