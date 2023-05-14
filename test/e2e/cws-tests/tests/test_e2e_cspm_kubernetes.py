import argparse
import sys
import time
import unittest
import warnings

import emoji
from lib.const import CSPM_START_LOG
from lib.cspm.api import App
from lib.kubernetes import KubernetesHelper
from lib.log import wait_agent_log
from lib.stepper import Step
from test_e2e_cspm import expect_findings


class TestE2EKubernetes(unittest.TestCase):

    namespace = "default"
    in_cluster = False
    expectedFindingsMasterEtcdNode = {
        "cis-kubernetes-1.5.1-1.1.12": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.16": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.19": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.21": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.22": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.23": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.24": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.25": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.26": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.33": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.2.6": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.3.2": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.3.3": [
            {
                "result": "passed",
            }
        ],
        "cis-kubernetes-1.5.1-1.3.4": [
            {
                "result": "passed",
            }
        ],
        "cis-kubernetes-1.5.1-1.3.5": [
            {
                "result": "passed",
            }
        ],
        "cis-kubernetes-1.5.1-1.3.6": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-1.3.7": [
            {
                "result": "passed",
            }
        ],
        "cis-kubernetes-1.5.1-1.4.1": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-3.2.1": [
            {
                "result": "failed",
            }
        ],
    }
    expectedFindingsWorkerNode = {
        "cis-kubernetes-1.5.1-4.1.1": [
            {
                "result": "error",
            }
        ],
        "cis-kubernetes-1.5.1-4.1.2": [
            {
                "result": "error",
            }
        ],
        "cis-kubernetes-1.5.1-4.1.3": [
            {
                "result": "error",
            }
        ],
        "cis-kubernetes-1.5.1-4.1.4": [
            {
                "result": "error",
            }
        ],
        "cis-kubernetes-1.5.1-4.1.7": [
            {
                "result": "error",
            }
        ],
        "cis-kubernetes-1.5.1-4.1.8": [
            {
                "result": "error",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.1": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.3": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.4": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.5": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.6": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.10": [
            {
                "result": "failed",
            }
        ],
        "cis-kubernetes-1.5.1-4.2.12": [
            {
                "result": "failed",
            }
        ],
    }
    hostname = "k8s-e2e-tests-control-plane"

    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)
        warnings.simplefilter("ignore", category=DeprecationWarning)

        self.kubernetes_helper = KubernetesHelper(namespace=self.namespace, in_cluster=self.in_cluster)
        self.resource_id = "k8s-e2e-tests-control-plane_kubernetes_*_node"
        self.app = App()

    def test_k8s(self):
        print("")

        agent_name = "security-agent"

        with Step(msg="select pod", emoji=":man_running:"):
            self.kubernetes_helper.select_pod_name("app.kubernetes.io/component=agent")

        with Step(msg="check agent start", emoji=":man_running:"):
            wait_agent_log(agent_name, self.kubernetes_helper, CSPM_START_LOG)

        with Step(msg="check agent events", emoji=":check_mark_button:"):
            self.kubernetes_helper.exec_command(
                agent_name, ["security-agent", "compliance", "check", "--dump-reports", "/tmp/reports", "--report"]
            )
            output = self.kubernetes_helper.exec_command(agent_name, ["bash", "-c", "cat /tmp/reports"])
            # if the output is JSON, it automatically calls json.loads on it. Yeah, I know... I've felt the same too
            findings = eval(output)
            expected_findings = dict(
                **TestE2EKubernetes.expectedFindingsMasterEtcdNode, **TestE2EKubernetes.expectedFindingsWorkerNode
            )
            expect_findings(self, findings, expected_findings)

        with Step(msg="wait for intake (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="wait for datadog.security_agent.compliance.running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric("datadog.security_agent.compliance.running", host=TestE2EKubernetes.hostname)

        ## Disabled while no CSPM API is available
        # with Step(msg="check app compliance event", emoji=":SOON_arrow:"):
        #     wait_for_compliance_event(f"resource_id:{self.resource_id}")

        with Step(msg="wait for finding generation (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="wait for datadog.security_agent.compliance.containers_running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric(
                "datadog.security_agent.compliance.containers_running", host=TestE2EKubernetes.hostname
            )

        ## Disabled while no CSPM API is available
        # with Step(msg="check app findings", emoji=":chart_increasing_with_yen:"):
        #     wait_for_findings(f"@resource_type:kubernetes_*_node @resource:{self.resource_id}")

        print(emoji.emojize(":heart_on_fire:"), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--namespace", default="default")
    parser.add_argument("--in-cluster", action="store_true")
    parser.add_argument("unittest_args", nargs="*")
    args = parser.parse_args()

    # setup some specific tests
    TestE2EKubernetes.namespace = args.namespace
    TestE2EKubernetes.in_cluster = args.in_cluster

    unit_argv = [sys.argv[0]] + args.unittest_args
    unittest.main(argv=unit_argv)


if __name__ == "__main__":
    main()
