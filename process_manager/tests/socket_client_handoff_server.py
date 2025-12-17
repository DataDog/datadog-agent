#!/usr/bin/env python3
"""
Echo server that supports trace-loader style socket activation with client handoff.

This server expects TWO socket FDs:
1. DD_APM_NET_RECEIVER_FD - The listening socket for future connections
2. DD_APM_NET_RECEIVER_CLIENT_FD - Pre-accepted client connection from daemon

The server:
1. First handles the pre-accepted client connection (sends handoff confirmation)
2. Then accepts and handles one more connection on the listener
3. Exits after handling both
"""

import os
import socket
import sys


def main():
    # Check for socket activation with client handoff
    listener_fd_str = os.environ.get('DD_APM_NET_RECEIVER_FD')
    client_fd_str = os.environ.get('DD_APM_NET_RECEIVER_CLIENT_FD')

    print(f"PID: {os.getpid()}", flush=True)
    print(f"DD_APM_NET_RECEIVER_FD: {listener_fd_str}", flush=True)
    print(f"DD_APM_NET_RECEIVER_CLIENT_FD: {client_fd_str}", flush=True)

    if not listener_fd_str:
        print("ERROR: No DD_APM_NET_RECEIVER_FD - listener socket not passed", file=sys.stderr)
        sys.exit(1)

    listener_fd = int(listener_fd_str)

    # Get the listening socket
    listener_sock = socket.fromfd(listener_fd, socket.AF_INET, socket.SOCK_STREAM)
    print(f"Listener socket acquired from FD {listener_fd}", flush=True)

    # Handle pre-accepted client connection if provided
    if client_fd_str:
        client_fd = int(client_fd_str)
        try:
            # Get the pre-accepted client connection
            client_conn = socket.fromfd(client_fd, socket.AF_INET, socket.SOCK_STREAM)
            print(f"Pre-accepted client socket acquired from FD {client_fd}", flush=True)

            # Handle this connection
            data = client_conn.recv(1024)
            if data:
                response = f"HANDOFF_OK: {data.decode().strip()}\n"
                print(f"Pre-accepted client sent: {data.decode().strip()}", flush=True)
                client_conn.sendall(response.encode())
                print(f"Sent response to pre-accepted client: {response.strip()}", flush=True)
            client_conn.close()
            print("Pre-accepted client connection closed", flush=True)
        except Exception as e:
            print(f"Error handling pre-accepted client: {e}", file=sys.stderr)
    else:
        print("No pre-accepted client (DD_APM_NET_RECEIVER_CLIENT_FD not set)", flush=True)

    # Now accept and handle one connection on the listener
    try:
        print("Waiting for connection on listener socket...", flush=True)
        listener_sock.settimeout(10)  # 10 second timeout
        conn, addr = listener_sock.accept()
        print(f"Accepted new connection from {addr}", flush=True)

        with conn:
            data = conn.recv(1024)
            if data:
                response = f"LISTENER_OK: {data.decode().strip()}\n"
                print(f"Received from new client: {data.decode().strip()}", flush=True)
                conn.sendall(response.encode())
                print(f"Sent response: {response.strip()}", flush=True)

        print("New connection handled, exiting", flush=True)
    except TimeoutError:
        print("Timeout waiting for connection on listener", flush=True)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
