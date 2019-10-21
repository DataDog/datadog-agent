import os
import util
import pytest
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')

kubeconfig_env = "KUBECONFIG=/home/ubuntu/deployment/aws-eks/tf-cluster/kubeconfig "


@pytest.mark.first
def test_receiver_healthy(host):
    def assert_healthy():
        c = "curl -s -o /dev/null -w \"%{http_code}\" http://localhost:7077/health"
        assert host.check_output(c) == "200"

    util.wait_until(assert_healthy, 30, 5)


@pytest.mark.second
def test_node_agent_healthy(host):
    def assert_healthy():
        c = kubeconfig_env + "kubectl wait --for=condition=ready --timeout=1s -l app=stackstate-agent pod"
        assert host.run(c).rc == 0

    util.wait_until(assert_healthy, 30, 5)


@pytest.mark.third
def test_cluster_agent_healthy(host):
    def assert_healthy():
        c = kubeconfig_env + "kubectl wait --for=condition=available --timeout=1s deployment/stackstate-cluster-agent"
        assert host.run(c).rc == 0

    util.wait_until(assert_healthy, 30, 5)


@pytest.mark.fourth
def test_apply_pod_to_service_demo(host):
    # We recognize DNAT connections after the agent is started, because we dunno directions of in-flight connections
    # so we make sure we deploy the demo after the node agent is healthy
    c = kubeconfig_env + "kubectl apply -f /home/ubuntu/deployment/test_connections/pod-to-service-cluster-ip.yaml"
    assert host.run(c).rc == 0
