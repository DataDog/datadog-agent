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


def is_subset(subset, superset):
    if isinstance(subset, dict):
        return all(key in superset and is_subset(val, superset[key]) for key, val in subset.items())

    if isinstance(subset, list) or isinstance(subset, set):
        return all(any(is_subset(subitem, superitem) for superitem in superset) for subitem in subset)

    # assume that subset is a plain value if none of the above match
    return subset == superset
