#!/usr/bin/env python3
"""API service application."""
import http.server
import socketserver
import threading
import time
from datetime import datetime

PORT = 8080
ready = False

class HealthHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/health':
            if ready:
                self.send_response(200)
                self.send_header('Content-type', 'text/plain')
                self.end_headers()
                self.wfile.write(b'OK')
            else:
                self.send_response(503)
                self.send_header('Content-type', 'text/plain')
                self.end_headers()
                self.wfile.write(b'NOT READY')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass

def start_server():
    socketserver.TCPServer.allow_reuse_address = True
    with socketserver.TCPServer(("", PORT), HealthHandler) as httpd:
        httpd.serve_forever()

def log(msg):
    timestamp = datetime.now().strftime("%H:%M:%S")
    print(f"[{timestamp}] {msg}", flush=True)

# Start HTTP server in background
server_thread = threading.Thread(target=start_server, daemon=True)
server_thread.start()

log("API service starting...")
log("HTTP server listening on port 8080")

# Initialization steps
initialization_steps = [
    ("Loading configuration", 10),
    ("Connecting to database", 15),
    ("Warming up cache", 15),
    ("Building indexes", 10),
    ("Starting background workers", 10),
]

for step, duration in initialization_steps:
    log(f"{step}...")
    time.sleep(duration)
    log(f"{step} complete")

ready = True
log("API service ready")
log("Health check: OK")

# Keep running
while True:
    time.sleep(10)
    log("Processing requests")
