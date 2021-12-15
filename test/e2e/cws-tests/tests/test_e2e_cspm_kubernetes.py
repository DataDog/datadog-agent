import argparse
import sys
import time
import unittest
import warnings

import emoji
from lib.const import CSPM_RUNNING_K8S_MASTER_CHECK_LOG, CSPM_RUNNING_K8S_WORKER_CHECK_LOG, CSPM_START_LOG
from lib.cspm.api import wait_for_compliance_event, wait_for_finding
from lib.cspm.finding import (
    is_expected_k8s_master_node_finding,
    is_expected_k8s_worker_node_finding,
    parse_output_and_extract_findings,
)
from lib.kubernetes import KubernetesHelper
from lib.log import wait_agent_log
from lib.stepper import Step


class TestE2EKubernetes(unittest.TestCase):

    namespace = "default"
    in_cluster = False
    master_node = False

    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)
        warnings.simplefilter("ignore", category=DeprecationWarning)

        self.kubernetes_helper = KubernetesHelper(namespace=self.namespace, in_cluster=self.in_cluster)
        self.resource_id = "k8s-e2e-tests-control-plane_kubernetes_worker_node"
        if self.master_node:
            self.finding_checker = is_expected_k8s_master_node_finding
            self.cspm_running_check_log = CSPM_RUNNING_K8S_MASTER_CHECK_LOG
        else:
            self.finding_checker = is_expected_k8s_worker_node_finding
            self.cspm_running_check_log = CSPM_RUNNING_K8S_WORKER_CHECK_LOG

    def test_k8s(self):
        print("")

        agent_name = "security-agent"

        with Step(msg="select pod", emoji=":man_running:"):
            self.kubernetes_helper.select_pod_name("app=datadog-agent")

        with Step(msg="check agent start", emoji=":man_running:"):
            wait_agent_log(agent_name, self.kubernetes_helper, CSPM_START_LOG)

        with Step(msg="check agent event", emoji=":check_mark_button:"):
            output = self.kubernetes_helper.exec_command(
                agent_name, ["security-agent", "compliance", "check", "--report"]
            )
            print(output)
            findings = parse_output_and_extract_findings(output, self.cspm_running_check_log)
            self.finding = None
            for f in findings:
                if self.finding_checker(f):
                    self.finding = f
            if self.finding is None:
                raise LookupError(f"{agent_name} | {self.cspm_running_check_log}")

        with Step(msg="wait for intake (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="check app compliance event", emoji=":SOON_arrow:"):
            wait_for_compliance_event(f"resource_id:{self.resource_id}")

        with Step(msg="wait for finding generation (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="check app finding", emoji=":chart_increasing_with_yen:"):
            wait_for_finding(f"@resource_type:kubernetes_worker_node @resource:{self.resource_id}")

        print(emoji.emojize(":heart_on_fire:"), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--namespace", default="default")
    parser.add_argument("--in-cluster", action="store_true")
    parser.add_argument("--master-node", action="store_true")
    parser.add_argument("unittest_args", nargs="*")
    args = parser.parse_args()

    # setup some specific tests
    TestE2EKubernetes.namespace = args.namespace
    TestE2EKubernetes.in_cluster = args.in_cluster
    TestE2EKubernetes.master_node = args.master_node

    unit_argv = [sys.argv[0]] + args.unittest_args
    unittest.main(argv=unit_argv)


if __name__ == "__main__":
    main()
