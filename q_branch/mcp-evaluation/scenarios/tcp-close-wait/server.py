#!/usr/bin/env python3
import socket
import threading
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def handle_client(client_sock, addr):
    """Handle client connection WITHOUT closing socket"""
    try:
        request = client_sock.recv(4096).decode('utf-8')
        if request.startswith('GET'):
            response = (
                "HTTP/1.1 200 OK\r\n"
                "Content-Type: text/plain\r\n"
                "Content-Length: 3\r\n"
                "\r\n"
                "OK\n"
            )
            client_sock.sendall(response.encode('utf-8'))
        # Intentionally NOT closing socket - this causes CLOSE_WAIT
        # client_sock.close()  # <-- Missing!
    except Exception:
        pass


def main():
    log("HTTP service started")

    server_sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server_sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server_sock.bind(('0.0.0.0', 9000))
    server_sock.listen(100)

    log("Listening on port 9000")

    request_count = 0

    while True:
        try:
            client_sock, addr = server_sock.accept()
            request_count += 1

            # Handle in thread to accept more connections
            thread = threading.Thread(target=handle_client, args=(client_sock, addr))
            thread.daemon = True
            thread.start()

            if request_count % 10 == 0:
                log(f"Handled {request_count} requests")

        except Exception as e:
            log(f"Error: {e}")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("HTTP service stopped")
    except Exception as e:
        log(f"Fatal error: {e}")
