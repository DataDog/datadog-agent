# flake8: noqa
import time

from opentelemetry import trace

tracer = trace.get_tracer(__name__)


def simple_test(event, context):
    with tracer.start_as_current_span('my-function'):
        time.sleep(0.1)
    return {"statusCode": 200, "body": "ok"}
