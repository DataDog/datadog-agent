#!/usr/bin/env python3
import http.server
import socketserver
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


PORT = 8080


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-type", "text/plain")
        self.end_headers()
        self.wfile.write(b"API Service v1\n")


def main():
    log("Starting API service")

    try:
        with socketserver.TCPServer(("", PORT), Handler) as httpd:
            log(f"Listening on port {PORT}")
            httpd.serve_forever()
    except OSError as e:
        log(f"Failed to bind: {e}")
        raise


if __name__ == "__main__":
    main()
