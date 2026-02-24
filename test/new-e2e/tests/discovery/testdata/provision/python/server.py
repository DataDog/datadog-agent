#!/usr/bin/env python3

import logging
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

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

    dd_service = os.getenv("DD_SERVICE", "python")
    pid = os.getpid()
    logpath = f'/tmp/{dd_service}-{pid}'
    logfile = f'{logpath}/foo.log'
    restricted = dd_service == "python-restricted-dd"

    os.makedirs(logpath, exist_ok=True)

    # Write-only log file to trigger permission errors from discovery
    mode = 0o222 if restricted else 0o666

    fd = os.open(logfile, os.O_CREAT | os.O_WRONLY | os.O_APPEND, mode)
    file = os.fdopen(fd, "a")

    if restricted:
        # Test that privileged logs can open the file even when the directory is
        # not searchable.
        os.chmod(logpath, 0o0)

    logging.basicConfig(
        level=logging.INFO,
        format='%(message)s',
        # Configure logging to write to file which will be automatically
        # discovered and tailed.  Make sure we use a unique file name since
        # there are multiple instances of this server running with different
        # service names.
        handlers=[logging.StreamHandler(file), logging.StreamHandler()],
    )

    logger.info("Server is running on http://%s:%s", host, port)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    server.server_close()


if __name__ == '__main__':
    run()
