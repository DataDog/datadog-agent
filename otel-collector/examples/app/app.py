# These are the necessary import declarations
import logging

from opentelemetry import trace
from opentelemetry import metrics

from random import randint
from flask import Flask
import os

from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter

from opentelemetry.sdk.metrics.export import (
    PeriodicExportingMetricReader,
)

app = Flask(__name__)
logging.basicConfig(filename='/var/tmp/app.log', level=logging.INFO)

endpoint = os.environ.get('OTEL_EXPORTER_OTLP_ENDPOINT', 'http://localhost:4317')
app.logger.info(f"using endpoint:{endpoint}")
metric_reader = PeriodicExportingMetricReader(OTLPMetricExporter(endpoint=endpoint))
provider = MeterProvider(metric_readers=[metric_reader])

# Sets the global default meter provider
metrics.set_meter_provider(provider)

tracer = trace.get_tracer("diceroller.tracer")
# Acquire a meter.
meter = metrics.get_meter("diceroller.meter")


# Now create a counter instrument to make measurements with
roll_counter = meter.create_counter(
    "roll_counter",
    description="The number of rolls by roll value",
)


@app.route("/rolldice")
def roll_dice():
    r = str(do_roll())
    app.logger.info(f"roll_num: {r}")
    return r


def do_roll():
    with tracer.start_as_current_span("do_roll") as rollspan:
        res = randint(1, 6)
        rollspan.set_attribute("roll.value", res)
        # This adds 1 to the counter for the given roll value
        roll_counter.add(1, {"roll.value": res})
        return res


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=os.environ.get('SERVER_PORT', 9090))
