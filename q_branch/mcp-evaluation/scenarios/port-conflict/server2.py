#!/usr/bin/env python3
import http.server
import socketserver
from datetime import datetime
import time


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
        self.wfile.write(b"API Service v2\n")


def main():
    log("Starting backup API service")

    while True:
        try:
            with socketserver.TCPServer(("", PORT), Handler) as httpd:
                log(f"Listening on port {PORT}")
                httpd.serve_forever()
        except OSError as e:
            log(f"Failed to bind: {e}")
            log("Retrying in 30 seconds...")
            time.sleep(30)


if __name__ == "__main__":
    main()
