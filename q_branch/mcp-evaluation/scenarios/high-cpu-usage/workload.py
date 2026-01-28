#!/usr/bin/env python3
import hashlib
import os
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Service started")

    iteration = 0
    data = os.urandom(1024 * 1024)

    while True:
        hasher = hashlib.sha256()
        for _ in range(1000):
            hasher.update(data)

        digest = hasher.hexdigest()
        iteration += 1

        if iteration % 10000 == 0:
            log(f"Processed batch {iteration}")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Service stopped")
    except Exception as e:
        log(f"Error: {e}")
