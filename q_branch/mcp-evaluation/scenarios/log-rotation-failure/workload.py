#!/usr/bin/env python3
import time
from datetime import datetime
import random


def log_to_file(filename, msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    with open(filename, 'a') as f:
        f.write(f"[{timestamp}] {msg}\n")


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def generate_log_entry():
    """Generate a log entry (~1KB)"""
    request_id = random.randint(100000, 999999)
    user_id = random.randint(1000, 9999)
    endpoint = random.choice(['/api/users', '/api/orders', '/api/products', '/api/auth'])
    status = random.choice([200, 200, 200, 201, 400, 404, 500])
    duration_ms = random.randint(10, 500)

    # Pad to make each entry roughly 1KB
    padding = 'x' * 800

    return f"REQ={request_id} USER={user_id} {endpoint} STATUS={status} DURATION={duration_ms}ms DATA={padding}"


def main():
    log("Application service started")

    log_file = "/tmp/app_logs/service.log"
    entry_count = 0

    while True:
        # Write log entries
        for _ in range(10):
            entry = generate_log_entry()
            log_to_file(log_file, entry)
            entry_count += 1

        if entry_count % 100 == 0:
            log(f"Logged {entry_count} entries")

        time.sleep(1)  # ~10KB per second = ~600KB per minute


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Application service stopped")
    except Exception as e:
        log(f"Error: {e}")
