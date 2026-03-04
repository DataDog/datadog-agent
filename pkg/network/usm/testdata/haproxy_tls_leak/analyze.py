#!/usr/bin/env python3
"""Analyze USM debug output to detect TLS tag misattribution.

Reads /debug/http_monitoring JSON output and categorizes entries into:
- TLS frontend (expected): server=HAProxy:8443, with TLS tags
- Correct plaintext backend: server=backend:80, no TLS tags
- TLS on plaintext backend (BUG): server=backend:80, WITH TLS tags
"""

import json
import sys

# Backend IPs in the docker-compose setup
BACKEND_IPS = {"172.30.0.20", "172.30.0.30"}
HAPROXY_IP = "172.30.0.10"

# Tag bitmasks from tags-types.h
TAG_NAMES = {
    0x01: "GnuTLS",
    0x02: "OpenSSL",
    0x04: "Go",
    0x08: "ConnTLS",
    0x10: "Istio",
    0x20: "NodeJS",
}


def ip_from_addr(addr):
    """Convert address string to IP."""
    if isinstance(addr, dict):
        return addr.get("IP", "")
    return str(addr)


def decode_tags(tags_val):
    """Decode StaticTags bitmask to human-readable names."""
    if not tags_val:
        return "none"
    names = []
    for bit, name in TAG_NAMES.items():
        if tags_val & bit:
            names.append(name)
    return ",".join(names) if names else "unknown({})".format(tags_val)


def main():
    if len(sys.argv) > 1:
        with open(sys.argv[1]) as f:
            data = json.load(f)
    else:
        data = json.load(sys.stdin)

    tls_frontend = 0
    correct_backend = 0
    tls_on_backend = 0  # THE BUG
    other = 0

    tls_on_backend_entries = []

    for entry in data:
        server = entry.get("Server", entry.get("Dest", {}))
        server_ip = ip_from_addr(server.get("IP", ""))
        server_port = server.get("Port", 0)
        tags = entry.get("StaticTags", 0)
        path = entry.get("Path", "")
        req_count = entry.get("RequestCount", entry.get("Count", 1))

        is_backend = server_ip in BACKEND_IPS
        is_haproxy = server_ip == HAPROXY_IP
        has_tls_tags = tags != 0

        if is_haproxy and has_tls_tags:
            tls_frontend += req_count
        elif is_backend and not has_tls_tags:
            correct_backend += req_count
        elif is_backend and has_tls_tags:
            tls_on_backend += req_count
            tls_on_backend_entries.append({
                "path": path,
                "server": "{}:{}".format(server_ip, server_port),
                "tags": decode_tags(tags),
                "count": req_count,
            })
        else:
            other += req_count

    total = tls_frontend + correct_backend + tls_on_backend + other

    print("=== USM TLS Misattribution Analysis ===")
    print()
    print("TLS frontend (expected):          {}".format(tls_frontend))
    print("Correct plaintext backend:        {}".format(correct_backend))
    print("TLS on plaintext backend (BUG):   {}".format(tls_on_backend))
    print("Other:                            {}".format(other))
    print("Total:                            {}".format(total))
    print()

    if tls_on_backend > 0:
        leak_pct = tls_on_backend / total * 100 if total > 0 else 0
        print("*** MISATTRIBUTION DETECTED: {} requests ({:.1f}%) ***".format(
            tls_on_backend, leak_pct))
        print()
        print("Sample misattributed entries:")
        for e in tls_on_backend_entries[:10]:
            print("  path={}, server={}, tags={}, count={}".format(
                e["path"], e["server"], e["tags"], e["count"]))
    else:
        print("No misattribution detected.")


if __name__ == "__main__":
    main()
