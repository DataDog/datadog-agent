import json
import os

from flask import Flask

app = Flask(__name__)


def _platform():
    return (
        os.environ.get("K_SERVICE")
        or os.environ.get("CONTAINER_APP_NAME")
        or os.environ.get("WEBSITE_SITE_NAME")
        or "unknown"
    )


@app.route("/")
def hello():
    return (
        json.dumps({"status": "ok", "platform": _platform()}),
        200,
        {"Content-Type": "application/json"},
    )


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", 8080)))
