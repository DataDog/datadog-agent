#!/usr/bin/env python3
"""Payment service API."""
import http.server
import socketserver
import time
from datetime import datetime

PORT = 8080
start_time = time.time()
DELAY = 30

class PaymentHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        elapsed = time.time() - start_time
        timestamp = datetime.now().strftime("%H:%M:%S")

        # Simulate payment processing
        if elapsed > DELAY:
            time.sleep(5.0)
        else:
            time.sleep(0.1)

        print(f"[{timestamp}] Payment processed successfully", flush=True)

        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'APPROVED\n')

    def log_message(self, format, *args):
        pass

print("Payment service starting on port 8080", flush=True)

with socketserver.TCPServer(("0.0.0.0", PORT), PaymentHandler) as httpd:
    httpd.serve_forever()
