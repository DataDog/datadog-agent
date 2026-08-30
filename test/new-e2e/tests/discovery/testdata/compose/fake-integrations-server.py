import http.server
import socketserver
import threading


class MetricsHandler(http.server.BaseHTTPRequestHandler):
    # The real krakend metrics endpoint. Deliberately NOT on 9090 (see
    # below) so that the test proves discover_config actually probes
    # and picks the right port among several exposed ones, rather
    # than trivially succeeding because there's only one candidate.
    BODY = (
        b"# HELP krakend_requests_total Total requests.\n"
        b"# TYPE krakend_requests_total counter\n"
        b"krakend_requests_total 42\n"
        b"# HELP go_goroutines Number of goroutines.\n"
        b"# TYPE go_goroutines gauge\n"
        b"go_goroutines 17\n"
    )

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.send_header("Content-Length", str(len(self.BODY)))
        self.end_headers()
        self.wfile.write(self.BODY)

    def log_message(self, *args):
        pass


class DummyHandler(http.server.BaseHTTPRequestHandler):
    # A decoy endpoint on krakend's default metrics port (9090) that
    # is NOT valid krakend/openmetrics output. If config discovery
    # picked a port blindly instead of probing and validating each
    # candidate, it would pick this one and the test would fail.
    BODY = b"not krakend metrics\n"

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(self.BODY)))
        self.end_headers()
        self.wfile.write(self.BODY)

    def log_message(self, *args):
        pass


class HaproxyMetricsHandler(http.server.BaseHTTPRequestHandler):
    # haproxy's own metrics, on its own discovery port_hints port (8404).
    # Metric names match haproxy's real METRIC_MAP
    # (datadog_checks/haproxy/metrics.py) verbatim.
    BODY = (
        b"# HELP haproxy_process_requests Total number of requests processed by process.\n"
        b"# TYPE haproxy_process_requests counter\n"
        b"haproxy_process_requests 42\n"
        b"# HELP haproxy_backend_status Current status of the backend.\n"
        b"# TYPE haproxy_backend_status gauge\n"
        b"haproxy_backend_status 1\n"
    )

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.send_header("Content-Length", str(len(self.BODY)))
        self.end_headers()
        self.wfile.write(self.BODY)

    def log_message(self, *args):
        pass


for port, handler in (
    (9090, DummyHandler),
    (9091, MetricsHandler),
    (8404, HaproxyMetricsHandler),
):
    threading.Thread(
        target=lambda p=port, h=handler: socketserver.TCPServer(("", p), h).serve_forever(),
        daemon=True,
    ).start()

threading.Event().wait()
