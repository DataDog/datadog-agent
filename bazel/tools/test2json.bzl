"""Shared helpers for test2json aspect fragments."""

def fragment_basename(target_name, test_log_path):
    """Basename of the aspect fragment for a TestRunner test.log path."""
    marker = target_name + "/"
    idx = test_log_path.find(marker)
    if idx >= 0:
        suffix = test_log_path[idx + len(marker):]
    else:
        suffix = test_log_path.split("/")[-1]
    safe = suffix.replace("/", "_").replace(":", "_")
    return target_name + "_" + safe + ".test2json.jsonl"

Test2JsonInfo = provider(
    doc = "Aspect-produced test2json fragment files for a test target.",
    fields = {
        "fragments": "depset[File]: test2json JSONL fragments, one per test.log.",
        "label": "str: canonical //package:target label string.",
    },
)
