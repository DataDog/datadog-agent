# flake8: noqa
import time

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

resource = Resource(attributes={SERVICE_NAME: 'python-otlp'})
endpoint = 'http://localhost:4318/v1/traces'

# initialize tracer
provider = TracerProvider(resource=resource)
processor = BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint))
provider.add_span_processor(processor)
trace.set_tracer_provider(provider)

tracer = trace.get_tracer(__name__)


@tracer.start_as_current_span('my-handler')
def simple_test(event, context):
    with tracer.start_as_current_span('my-function'):
        time.sleep(0.1)
    return {"statusCode": 200, "body": "ok"}
