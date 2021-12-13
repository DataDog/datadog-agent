import json


def extract_findings(lines):
    if not lines:
        return []

    res_lines = ["["]
    for line in lines:
        if line == "}":
            res_lines.append("},")
        else:
            res_lines.append(line)
    res_lines.pop()
    res_lines.extend(["}", "]"])
    return json.loads("".join(res_lines))


def is_expected_docker_finding(finding, container_id):
    if finding["agent_rule_id"] != "cis-docker-1.2.0-5.4":
        return False
    if finding["agent_framework_id"] != "cis-docker":
        return False
    if finding["result"] != "failed":
        return False
    if finding["resource_type"] != "docker_container":
        return False
    return finding["data"]["container.id"] == container_id


def is_expected_k8s_finding(finding):
    if finding["agent_rule_id"] != "cis-kubernetes-1.5.1-4.2.6":
        return False
    if finding["agent_framework_id"] != "cis-kubernetes":
        return False
    if finding["result"] != "failed":
        return False
    if finding["resource_type"] != "kubernetes_worker_node":
        return False
    if finding["data"]["file.path"] != "/var/lib/kubelet/config.yaml":
        return False
    return True
