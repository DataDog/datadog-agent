#!/usr/bin/env python3
import socket
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Connection tester started")

    target_host = "127.0.0.1"
    target_port = 80  # Common HTTP port

    sockets = []
    connection_count = 0

    while True:
        try:
            # Create socket and initiate connection (SYN sent)
            # but don't complete handshake
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.setblocking(False)

            try:
                sock.connect((target_host, target_port))
            except BlockingIOError:
                # Expected - connection in progress (SYN sent, waiting for SYN-ACK)
                # We keep the socket but don't complete handshake
                sockets.append(sock)
                connection_count += 1
            except Exception:
                sock.close()

            if connection_count % 100 == 0:
                log(f"Connection attempts: {connection_count}")

            time.sleep(0.01)  # 100 per second

        except Exception as e:
            log(f"Error: {e}")
            time.sleep(1)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Connection tester stopped")
    except Exception as e:
        log(f"Fatal error: {e}")
