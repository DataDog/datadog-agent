#!/usr/bin/env python3
import socket
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Connection monitor started")

    hosts = [
        "example.com",
        "google.com",
        "github.com",
    ]

    check_count = 0

    while True:
        check_count += 1

        for host in hosts:
            try:
                socket.gethostbyname(host)
                log(f"Connected to {host}")
            except socket.gaierror as e:
                log(f"Failed to resolve {host}: {e}")
            except Exception as e:
                log(f"Connection error for {host}: {e}")

        if check_count % 5 == 0:
            log(f"Completed {check_count} check cycles")

        time.sleep(10)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Connection monitor stopped")
    except Exception as e:
        log(f"Error: {e}")
