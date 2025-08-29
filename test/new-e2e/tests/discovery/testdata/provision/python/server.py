#!/usr/bin/env python3

import logging
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

# Configure logging to write to file which will be automatically discovered and
# tailed.
logging.basicConfig(
    level=logging.INFO,
    format='%(message)s',
    handlers=[logging.FileHandler('/tmp/python-svc.log'), logging.StreamHandler()],
)

logger = logging.getLogger(__name__)


class Handler(BaseHTTPRequestHandler):
    def _set_response(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()

    def do_GET(self):
        logger.info("GET %s", self.path)
        self._set_response()
        self.wfile.write(f"GET request for {self.path}".encode())

    def do_POST(self):
        logger.info("POST %s", self.path)
        self._set_response()
        self.wfile.write(f"POST request for {self.path}".encode())


def run():
    host = '0.0.0.0'
    port = 8080
    if 'PORT' in os.environ:
        port = int(os.environ['PORT'])

    addr = (host, port)
    server = HTTPServer(addr, Handler)

    logger.info("Server is running on http://%s:%s", host, port)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    server.server_close()


if __name__ == '__main__':
    run()
