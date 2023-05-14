import os

import lib.common.app as common
import requests
from retry.api import retry_call


def aggregate_logs(query, track):
    site = os.environ["DD_SITE"]
    api_key = os.environ["DD_API_KEY"]
    app_key = os.environ["DD_APP_KEY"]

    url = f"https://api.{site}/api/v2/logs/analytics/aggregate?type={track}"
    body = {
        "compute": [{"aggregation": "count", "type": "total"}],
        "filter": {
            "from": "now-3m",
            "to": "now",
            "query": query,
        },
    }

    r = requests.post(
        url,
        headers={"DD-API-KEY": api_key, "DD-APPLICATION-KEY": app_key},
        json=body,
    )
    api_response = r.json()
    if not api_response["data"] or not api_response["data"]["buckets"]:
        raise LookupError(query)

    count = api_response["data"]["buckets"][0]["computes"]["c0"]
    if count == 0:
        raise LookupError(query)

    return api_response


def fetch_app_findings(query):
    return aggregate_logs(query, track="cpfinding")


def fetch_app_compliance_event(query):
    return aggregate_logs(query, track="compliance")


def wait_for_findings(query, tries=30, delay=5):
    return retry_call(fetch_app_findings, fargs=[query], tries=tries, delay=delay)


def wait_for_compliance_event(query, tries=30, delay=5):
    return retry_call(fetch_app_compliance_event, fargs=[query], tries=tries, delay=delay)


class App(common.App):
    pass
