import json
import os
import ssl
import threading
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, unquote, urlparse


ISSUE_NAME = "Invalid Config"
ISSUE_TYPE = "invalid_config"
RESOURCE_TYPE = "agent_health_issue_ids"

lock = threading.Lock()
issues = {
    "node": [
        {
            "issue_id": os.environ["STALE_NODE_ISSUE_ID"],
            "issue_name": ISSUE_NAME,
            "issue_type": ISSUE_TYPE,
        }
    ],
    "cluster": [
        {
            "issue_id": os.environ["STALE_CLUSTER_ISSUE_ID"],
            "issue_name": ISSUE_NAME,
            "issue_type": ISSUE_TYPE,
        }
    ],
}
requests = []


class Handler(BaseHTTPRequestHandler):
    def log_message(self, _format, *_args):
        return

    def _json(self, status, body):
        payload = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/health":
            self._json(200, {"status": "ok"})
            return
        if parsed.path == "/_test/requests":
            with lock:
                self._json(200, {"requests": list(requests)})
            return

        prefix = "/api/v2/agenthealth/hosts/"
        suffix = "/issues"
        if not parsed.path.startswith(prefix) or not parsed.path.endswith(suffix):
            self._json(404, {"error": "not found"})
            return

        resource_id = unquote(parsed.path[len(prefix) : -len(suffix)])
        agent_type = parse_qs(parsed.query).get("agent_type", [""])[0]
        api_key_present = bool(self.headers.get("DD-API-KEY", "").strip())
        request = {
            "resource_id": resource_id,
            "agent_type": agent_type,
            "api_key_present": api_key_present,
            "accept": self.headers.get("Accept", ""),
            "agent_version_present": bool(self.headers.get("DD-Agent-Version", "").strip()),
            "user_agent": self.headers.get("User-Agent", ""),
            "received_at": datetime.now(timezone.utc).isoformat(),
        }
        with lock:
            requests.append(request)
            response_issues = list(issues.get(agent_type, []))

        if not api_key_present:
            self._json(401, {"error": "missing API key"})
            return
        if agent_type not in issues:
            self._json(400, {"error": "unknown agent_type"})
            return

        self._json(
            200,
            {
                "data": {
                    "id": resource_id,
                    "type": RESOURCE_TYPE,
                    "attributes": {"issues": response_issues},
                }
            },
        )

    def do_PUT(self):
        prefix = "/_test/issues/"
        parsed = urlparse(self.path)
        if not parsed.path.startswith(prefix):
            self._json(404, {"error": "not found"})
            return

        agent_type = parsed.path[len(prefix) :]
        if agent_type not in issues:
            self._json(400, {"error": "unknown agent_type"})
            return

        try:
            length = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(length))
            replacement = body["issues"]
        except (KeyError, TypeError, ValueError, json.JSONDecodeError):
            self._json(400, {"error": "invalid request"})
            return

        with lock:
            issues[agent_type] = replacement
        self._json(200, {"issues": replacement})


server = ThreadingHTTPServer(("0.0.0.0", 8443), Handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(os.environ["TLS_CERT"], os.environ["TLS_KEY"])
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
