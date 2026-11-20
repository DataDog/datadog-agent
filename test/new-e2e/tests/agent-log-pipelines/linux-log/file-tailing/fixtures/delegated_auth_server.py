import base64
import http.server
import json
import ssl

OIDC_AUDIENCE = "datadog/00000000-0000-0000-0000-000000000001"


def oidc_token_audience(token):
    parts = token.split(".")
    if len(parts) != 3:
        return None
    try:
        payload = base64.urlsafe_b64decode(parts[1] + "=" * (-len(parts[1]) % 4))
        return json.loads(payload).get("aud")
    except (ValueError, AttributeError):
        return None


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/api/v2/intake-key":
            authorization = self.headers.get("Authorization", "")
            scheme, separator, proof = authorization.partition(" ")
            if scheme != "Delegated" or not separator or any(character.isspace() for character in proof):
                self.send_error(401)
                return
            if proof.startswith("oidc-"):
                provider, api_key = "oidc", "dela-e2e-oidc-api-key"
                valid = oidc_token_audience(proof[len("oidc-") :]) == OIDC_AUDIENCE
            else:
                provider, api_key = "aws", "dela-e2e-api-key"
                proof_parts = proof.split("|")
                valid = len(proof_parts) == 4 and all(proof_parts) and proof_parts[2] == "POST"
            if not valid:
                self.send_error(401)
                return

            with open("/tmp/dela-auth-requests.log", "a", encoding="utf-8") as requests:
                requests.write(provider + "\n")
            body = json.dumps({"data": {"attributes": {"api_key": api_key}}}).encode()
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
