#!/opt/datadog-agent/embedded/bin/python3
import json
import sys

with open("/tmp/fa-metadata-values.json", encoding="utf-8") as stream:
    values = json.load(stream)
request = json.load(sys.stdin)
json.dump({handle: {"value": values[handle]} for handle in request["secrets"]}, sys.stdout)
