"""Report which trust sources accept the compromised eMudhra chain.

Run with the Agent's embedded Python: python tls_check.py <install-dir>
"""

import os
import socket
import ssl
import sys

HOSTS = ("dv-g3b-aaa-emudhra.we-have-a-problem.com", "app.datadoghq.com")


def probe(host, ctx):
    try:
        with socket.create_connection((host, 443), timeout=10) as sock:
            with ctx.wrap_socket(sock, server_hostname=host):
                return "ACCEPT"
    except ssl.SSLCertVerificationError as exc:
        return f"reject ({exc.verify_message})"
    except OSError as exc:
        return f"error ({exc})"


def main():
    root = sys.argv[1]
    sources = []
    for dirpath, _, filenames in os.walk(root):
        for name in filenames:
            if name in ("cacert.pem", "cert.pem"):
                path = os.path.join(dirpath, name)
                ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
                ctx.load_verify_locations(cafile=path)
                sources.append((os.path.relpath(path, root), ctx))
    sources.append(("ssl.create_default_context() (OS store)", ssl.create_default_context()))
    for label, ctx in sources:
        print(label)
        for host in HOSTS:
            print(f"    {host:<45} {probe(host, ctx)}")


if __name__ == "__main__":
    main()
