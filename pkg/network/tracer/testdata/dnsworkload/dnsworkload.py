#!/usr/bin/env python3

import signal
import socket
import sys
import time


def signal_handler(signum, frame):
    sys.exit(0)


signal.signal(signal.SIGTERM, signal_handler)
signal.signal(signal.SIGINT, signal_handler)

# resolv.conf will expand this to my-server.local
hostname = "my-server"

while True:
    try:
        result = socket.getaddrinfo(hostname, None, socket.AF_INET)

        for res in result:
            ip_address = res[4][0]
            print(f"Name:      {hostname}")
            print(f"Address: {ip_address}")
            print()
    except socket.gaierror as e:
        print(f"DNS lookup failed for {hostname}: {e}")

    time.sleep(0.1)
