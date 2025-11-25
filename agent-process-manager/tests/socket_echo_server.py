#!/usr/bin/env python3
"""
Simple echo server that supports systemd socket activation.
Reads from activated socket and echoes back responses.
"""

import os
import socket
import sys


def main():
    # Check for socket activation
    listen_fds = os.environ.get('LISTEN_FDS')

    if not listen_fds:
        print("ERROR: No LISTEN_FDS - this server requires socket activation", file=sys.stderr)
        sys.exit(1)

    num_fds = int(listen_fds)
    if num_fds != 1:
        print(f"ERROR: Expected 1 FD, got {num_fds}", file=sys.stderr)
        sys.exit(1)

    # Socket activation: FD 3 is the listening socket
    sock = socket.fromfd(3, socket.AF_INET, socket.SOCK_STREAM)

    print(f"Socket-activated echo server started (PID {os.getpid()})", flush=True)

    # Accept ONE connection, echo data, then exit
    # This simulates a simple service
    try:
        conn, addr = sock.accept()
        print(f"Accepted connection from {addr}", flush=True)

        with conn:
            data = conn.recv(1024)
            if data:
                print(f"Received: {data.decode()}", flush=True)
                conn.sendall(data)  # Echo it back

        print("Connection closed, exiting", flush=True)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
