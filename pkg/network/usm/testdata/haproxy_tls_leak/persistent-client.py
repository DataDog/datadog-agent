#!/usr/bin/env python3
"""Persistent HTTPS client that maintains keep-alive connections to HAProxy.

Sends traffic to API and blackbox endpoints through HAProxy's TLS frontend,
using HTTP/1.1 keep-alive to maintain long-lived connections (simulating
the customer's 300s timeout configuration).
"""

import http.client
import ssl
import time
import random
import sys

API_ENDPOINTS = [
    "/v1/btc/pasithea_image_url_json",
    "/v1/nfl/scores",
    "/v1/graphics/render",
    "/conviva/health",
    "/device/status",
    "/redfish/v1/info",
]

BLACKBOX_ENDPOINTS = [
    "/elemental/alerts",
    "/elemental/channelstatus",
    "/elemental/stats",
    "/probe",
    "/metrics",
]

HAPROXY_HOST = "172.30.0.10"
HAPROXY_PORT = 8443


def create_connection():
    """Create a persistent HTTPS connection to HAProxy."""
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    conn = http.client.HTTPSConnection(HAPROXY_HOST, HAPROXY_PORT, context=ctx)
    return conn


def send_requests(conn, endpoints, count=10):
    """Send requests on an existing connection."""
    for _ in range(count):
        path = random.choice(endpoints)
        try:
            conn.request("GET", path)
            resp = conn.getresponse()
            resp.read()
        except Exception:
            return False
    return True


def main():
    print("Waiting 5s for HAProxy to start...", flush=True)
    time.sleep(5)

    all_endpoints = API_ENDPOINTS + BLACKBOX_ENDPOINTS
    connections = []

    # Create multiple persistent connections
    for i in range(4):
        try:
            conn = create_connection()
            conn.request("GET", random.choice(all_endpoints))
            resp = conn.getresponse()
            resp.read()
            connections.append(conn)
            print("Connection {} established".format(i), flush=True)
        except Exception as e:
            print("Failed to create connection {}: {}".format(i, e), flush=True)

    print("Sending persistent traffic on {} connections...".format(len(connections)),
          flush=True)

    iteration = 0
    while True:
        for i, conn in enumerate(connections):
            if not send_requests(conn, all_endpoints, count=5):
                # Reconnect on failure
                try:
                    conn.close()
                except Exception:
                    pass
                try:
                    connections[i] = create_connection()
                    print("Reconnected connection {}".format(i), flush=True)
                except Exception as e:
                    print("Reconnect failed for {}: {}".format(i, e), flush=True)

        iteration += 1
        if iteration % 100 == 0:
            print("Iteration {}, {} active connections".format(
                iteration, len(connections)), flush=True)

        time.sleep(0.01)


if __name__ == "__main__":
    main()
