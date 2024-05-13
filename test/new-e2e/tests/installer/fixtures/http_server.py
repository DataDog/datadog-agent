# Import the necessary libraries
from http.server import BaseHTTPRequestHandler, HTTPServer

# Define a handler for the HTTP requests.
class RequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        # Send an OK response
        self.send_response(200)
        # Set the content-type to text/html
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        # Send the message "Hello, World!"
        self.wfile.write(b"Hello, World!")

# Specify the port you want the server to run on
port = 8000

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
