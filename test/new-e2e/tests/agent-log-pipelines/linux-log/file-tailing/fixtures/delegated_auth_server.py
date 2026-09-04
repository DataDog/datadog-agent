import http.server
import json
import ssl


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/api/v2/intake-key":
            authorization = self.headers.get("Authorization", "")
            scheme, separator, proof = authorization.partition(" ")
            proof_parts = proof.split("|")
            if (
                scheme != "Delegated"
                or not separator
                or len(proof_parts) != 4
                or not all(proof_parts)
                or proof_parts[2] != "POST"
                or any(character.isspace() for character in proof)
            ):
                self.send_error(401)
                return

            with open("/tmp/dela-auth-requests.log", "a", encoding="utf-8") as requests:
                requests.write("exchange\n")
            body = json.dumps({"data": {"attributes": {"api_key": "dela-e2e-api-key"}}}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_error(404)

    def log_message(self, _format, *_args):
        return


server = http.server.ThreadingHTTPServer(("127.0.0.1", 443), Handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain("/tmp/dela-auth.crt", "/tmp/dela-auth.key")
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
