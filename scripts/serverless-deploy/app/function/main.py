import functions_framework
import json
import logging
import os


@functions_framework.http
def hello(request):
    logging.info(
        json.dumps({"message": "request received", "service": "dd-test-cloudrun-function"})
    )
    return json.dumps({"status": "ok", "platform": "cloud_run_function"}), 200, {
        "Content-Type": "application/json"
    }
