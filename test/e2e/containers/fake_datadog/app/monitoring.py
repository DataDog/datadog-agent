import os
import sys
import time

from flask import Flask, Response, g, request
from prometheus_client import CONTENT_TYPE_LATEST, CollectorRegistry, Counter, Histogram, generate_latest, multiprocess


def extract_exception_name(exc_info=None):
    """
    Function to get the exception name and module
    :param exc_info:
    :return:
    """
    if not exc_info:
        exc_info = sys.exc_info()
    return '{}.{}'.format(exc_info[0].__module__, exc_info[0].__name__)


def monitor_flask(app: Flask):
    """
    Add components to monitor each route with prometheus
    The monitoring is available at /metrics
    :param app: Flask application
    :return:
    """
    prometheus_state_dir = os.getenv('prometheus_multiproc_dir', "")
    if "gunicorn" not in os.getenv("SERVER_SOFTWARE", "") and prometheus_state_dir == "":
        return

    if os.path.isdir(prometheus_state_dir) is False:
        os.mkdir(prometheus_state_dir)

    metrics = CollectorRegistry()

    def collect():
        registry = CollectorRegistry()
        multiprocess.MultiProcessCollector(registry)
        data = generate_latest(registry)
        return Response(data, mimetype=CONTENT_TYPE_LATEST)

    app.add_url_rule('/metrics', 'metrics', collect)

    additional_kwargs = {'registry': metrics}
    request_latency = Histogram(
        'requests_duration_seconds', 'Backend API request latency', ['method', 'path'], **additional_kwargs
    )
    status_count = Counter(
        'responses_total', 'Backend API response count', ['method', 'path', 'status_code'], **additional_kwargs
    )
    exception_latency = Histogram(
        'exceptions_duration_seconds',
        'Backend API top-level exception latency',
        ['method', 'path', 'type'],
        **additional_kwargs
    )

    @app.before_request
    def start_measure():
        g._start_time = time.time()

    @app.after_request
    def count_status(response: Response):
        status_count.labels(request.method, request.url_rule, response.status_code).inc()
        request_latency.labels(request.method, request.url_rule).observe(time.time() - g._start_time)
        return response

    # Override log_exception to increment the exception counter
    def log_exception(exc_info):
        class_name = extract_exception_name(exc_info)
        exception_latency.labels(request.method, request.url_rule, class_name).observe(time.time() - g._start_time)
        app.logger.error('Exception on %s [%s]' % (request.path, request.method), exc_info=exc_info)

    app.log_exception = log_exception
