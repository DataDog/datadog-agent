#!/usr/bin/env python3

import os
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def _set_response(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()

    def do_GET(self):
        self._set_response()
        self.wfile.write(f"GET request for {self.path}".encode())

    def do_POST(self):
        self._set_response()
        self.wfile.write(f"POST request for {self.path}".encode())


def run():
    host = '0.0.0.0'
    port = 8080
    if 'PORT' in os.environ:
        port = int(os.environ['PORT'])

    addr = (host, port)
    server = HTTPServer(addr, Handler)

    print(f"Server is running on http://{host}:{port}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    server.server_close()


if __name__ == '__main__':
    run()
