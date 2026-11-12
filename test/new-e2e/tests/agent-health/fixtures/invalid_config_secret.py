#!/usr/bin/env python3
import json
import sys

SECRETS = {
    "pod_labels": '{"app": "kube_app", "https://qa:RAW_SECRET_URL_PASSWORD_e2e@example.test": "url"}',
    "source_excludes": '{"10.0.0.0/8": ["80"]}',
    "pool_size": "not-a-number",
    "inventories_enabled": "maybe",
    "zone_tag": "zone",
    "hostname_file": "RAW_SECRET_HOSTNAME_FILE_e2e",
}

if __name__ == "__main__":
    handles = json.loads(sys.stdin.read())["secrets"]
    print(json.dumps({h: {"value": SECRETS[h], "error": None} for h in handles}))
