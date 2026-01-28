#!/usr/bin/env python3
import socket
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Data collector service started")

    connections = []
    connection_count = 0

    # Target a well-known service that accepts connections
    target_host = "1.1.1.1"  # Cloudflare DNS
    target_port = 53

    while True:
        try:
            # Open connection but never close it
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(2)
            sock.connect((target_host, target_port))
            connections.append(sock)  # Keep reference to prevent GC
            connection_count += 1

            if connection_count % 50 == 0:
                log(f"Active connections: {connection_count}")

        except socket.timeout:
            pass
        except socket.error as e:
            log(f"Connection error: {e}")
            time.sleep(1)
        except Exception as e:
            log(f"Error: {e}")
            time.sleep(1)

        time.sleep(0.1)  # 10 connections per second


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Data collector service stopped")
    except Exception as e:
        log(f"Fatal error: {e}")
