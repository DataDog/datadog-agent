#!/usr/bin/env python3
"""
OpenSSL-based HTTPS server for misattribution reproduction test.
Uses Python's ssl module (which wraps OpenSSL) instead of Go's crypto/tls.
"""

import argparse
import ssl
import socket
import threading
import sys

def handle_client(conn, addr, port, server_name):
    """Handle a single HTTP request."""
    try:
        data = conn.recv(4096)
        if not data:
            return

        request = data.decode('utf-8', errors='ignore')
        lines = request.split('\r\n')
        if not lines:
            return

        # Parse request line: GET /path HTTP/1.1
        parts = lines[0].split(' ')
        if len(parts) < 2:
            return

        method = parts[0]
        path = parts[1]

        # Generate response
        body = f"Hello from {server_name}! Path: {path}\n"
        response = (
            f"HTTP/1.1 200 OK\r\n"
            f"Content-Type: text/plain\r\n"
            f"Content-Length: {len(body)}\r\n"
            f"Connection: close\r\n"
            f"\r\n"
            f"{body}"
        )

        conn.sendall(response.encode('utf-8'))
    except Exception as e:
        pass
    finally:
        try:
            conn.shutdown(socket.SHUT_RDWR)
        except:
            pass
        conn.close()

def run_server(port, cert_file, key_file):
    """Run the HTTPS server."""
    server_name = f"openssl-server-{port}"

    # Create SSL context using OpenSSL
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(cert_file, key_file)
    context.minimum_version = ssl.TLSVersion.TLSv1_2

    # Create socket
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(('0.0.0.0', port))
    sock.listen(128)

    print(f"Starting {server_name} on :{port}")
    sys.stdout.flush()

    with context.wrap_socket(sock, server_side=True) as ssock:
        while True:
            try:
                conn, addr = ssock.accept()
                # Handle each connection in a thread
                t = threading.Thread(target=handle_client, args=(conn, addr, port, server_name))
                t.daemon = True
                t.start()
            except ssl.SSLError as e:
                # Client disconnected during handshake
                pass
            except Exception as e:
                print(f"Error: {e}", file=sys.stderr)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='OpenSSL-based HTTPS server')
    parser.add_argument('-port', type=int, default=8443, help='Port to listen on')
    parser.add_argument('-cert', default='server.crt', help='TLS certificate file')
    parser.add_argument('-key', default='server.key', help='TLS key file')
    args = parser.parse_args()

    run_server(args.port, args.cert, args.key)