import http.server
import os
import re
import ssl

# bucketID = hash(apiKey) % 10, so a single digit is the entire valid range.
# Tightened from \d+ to [0-9] catches off-by-one or arithmetic bugs in createBucketTag.
BUCKET_TAG_RE = re.compile(r'^/v2/[^/]+/manifests/\d+-gr[0-9]$')
FAKE_DIGEST = os.environ['FAKE_DIGEST']
PORT = int(os.environ.get('PORT', '5000'))


class RegistryHandler(http.server.BaseHTTPRequestHandler):
    def do_HEAD(self):
        if BUCKET_TAG_RE.match(self.path):
            self.send_response(200)
            self.send_header('Docker-Content-Digest', FAKE_DIGEST)
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        if self.path == '/healthz':
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'ok')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass


ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(os.environ['TLS_CERT'], os.environ['TLS_KEY'])

server = http.server.HTTPServer(('', PORT), RegistryHandler)
server.socket = ctx.wrap_socket(server.socket, server_side=True)
print(f'Mock registry listening on :{PORT}', flush=True)
server.serve_forever()
