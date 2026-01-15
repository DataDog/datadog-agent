#!/usr/bin/env python3
import time
from datetime import datetime
import random
import string


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def generate_session_data():
    """Generate 5MB of session data"""
    return {
        'session_id': ''.join(random.choices(string.ascii_letters, k=32)),
        'user_data': 'x' * (5 * 1024 * 1024),  # 5MB string
        'timestamp': time.time()
    }


def main():
    log("Session cache service started")

    cache = {}
    entry_count = 0

    while True:
        entry_count += 1
        session_id = f"session_{entry_count:06d}"

        # Add to cache without any eviction
        cache[session_id] = generate_session_data()

        if entry_count % 10 == 0:
            log(f"Cache size: {entry_count} entries")

        time.sleep(10)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Session cache service stopped")
    except Exception as e:
        log(f"Error: {e}")
