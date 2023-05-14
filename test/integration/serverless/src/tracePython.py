# flake8: noqa
import time

from ddtrace import tracer


def simple_test(event, context):
    # submit a custom span
    with tracer.trace("integration-test"):
        time.sleep(0.1)

    return {"statusCode": 200, "body": "ok"}
