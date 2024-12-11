# Import the necessary libraries
import signal
from http.server import BaseHTTPRequestHandler, HTTPServer

from ddtrace import tracer
from ddtrace.propagation.http import HTTPPropagator


def signal_handler(signal_received, frame):
    # Handle any cleanup here
    print('SIGHUP received, but continuing to run.')


# Define a handler for the HTTP requests.
class RequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        # Extract tracing headers
        headers = {
            'X-Datadog-Trace-Id': self.headers.get('X-Datadog-Trace-Id'),
            'X-Datadog-Parent-Id': self.headers.get('X-Datadog-Parent-Id'),
            'X-Datadog-Sampling-Priority': self.headers.get('X-Datadog-Sampling-Priority'),
        }

        # Use HTTPPropagator to extract the context from the headers
        context = HTTPPropagator.extract(headers)
        if context.trace_id:
            # Continue the trace with the extracted context
            tracer.context_provider.activate(context)

        # Now proceed with the trace
        with tracer.trace("get", service="my-http-service"):
            self.send_response(200)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(b"Hello, World!")


# Specify the port you want the server to run on
port = 8080

signal.signal(signal.SIGHUP, signal_handler)

# Create the server
server_address = ('', port)
httpd = HTTPServer(server_address, RequestHandler)
print(f"Server running on port {port}...")
try:
    # Activate the server; this will keep running until you interrupt the program with Ctrl+C
    httpd.serve_forever()
except KeyboardInterrupt:
    print("Server is stopped by user")
    httpd.server_close()
