import json
import logging
import os

from datadog import initialize, statsd
from flask import Flask

# Initialize DogStatsD
initialize()

app = Flask(__name__)

logging.basicConfig(level=logging.INFO, format="%(message)s")


def _platform_name():
    return (
        os.environ.get("K_SERVICE")
        or os.environ.get("CONTAINER_APP_NAME")
        or os.environ.get("WEBSITE_SITE_NAME")
        or "unknown"
    )


@app.route("/")
def hello():
    platform = _platform_name()

    # Emit DogStatsD metric
    statsd.increment("serverless.test.request_count", tags=[f"platform:{platform}"])

    # Structured log line
    logging.info(
        json.dumps({"message": "request received", "service": platform, "status": 200})
    )

    return json.dumps({"status": "ok", "platform": platform}), 200, {"Content-Type": "application/json"}


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8080))
    app.run(host="0.0.0.0", port=port)
