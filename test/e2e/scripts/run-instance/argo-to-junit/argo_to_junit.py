#!/usr/bin/env python3

import json
from argparse import ArgumentParser
from datetime import datetime

from junit_xml import TestCase, TestSuite


def _str_to_datetime(date_str):
    return datetime.strptime(date_str, '%Y-%m-%dT%H:%M:%SZ')


def _generate_test_suites(root_name, argo_nodes):
    """
    Groups argo nodes by parents, generate the test cases
    and yields the corresponding test suites.
    """
    for node_id, node_status in argo_nodes.items():
        if node_status.get("type") in ["StepGroup", "DAG"]:
            test_cases = list()
            tc = TestCase(node_status.get("displayName", node_id))
            children = node_status.get("children", [])
            for child_id in children:
                child_status = argo_nodes.get(child_id, None)
                if not child_status or child_status.get("type") != "Pod":
                    continue
                children.extend(child_status.get("children", []))
                end = _str_to_datetime(child_status.get("finishedAt"))
                start = _str_to_datetime(child_status.get("startedAt"))
                job_duration = (end - start).total_seconds()
                tc = TestCase(child_status.get("displayName"), elapsed_sec=job_duration)
                if child_status.get("phase") == "Failed":
                    tc.add_failure_info(child_status.get("message"))
                test_cases.append(tc)
            if len(test_cases) == 0:
                continue
            parent_name = argo_nodes.get(node_status.get("boundaryID")).get("displayName")
            # Some steps are tied directly to the root workflow (i.e the parent is argo-datadog-agent-*)
            # Thus, we use a deterministic format to generate the test suite name in that case.
            ts_name = parent_name if parent_name != root_name else "root" + "/" + node_status.get("displayName")
            yield TestSuite(ts_name, test_cases)


def main():
    parser = ArgumentParser()
    parser.add_argument("-i", "--input-file", help="File containing the Argo CRD in JSON", required=True)
    parser.add_argument("-o", "--output-file", default="junit.xml", help="The junit xml file")
    args = parser.parse_args()

    with open(args.input_file) as f:
        crd = json.loads(f.read())
    crd_name = crd.get("metadata", {}).get("name")
    nodes = crd.get("status", {}).get("nodes")
    if not crd_name or not nodes:
        print(json.dumps(crd))
        raise Exception("Incompatible CRD")

    test_suites = list()
    for ts in _generate_test_suites(crd_name, nodes):
        test_suites.append(ts)
    with open(args.output_file, "w") as f:
        TestSuite.to_file(f, test_suites)


if __name__ == "__main__":
    main()
