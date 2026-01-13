#!/usr/bin/env python3
"""Cache service for session data."""
import time
from datetime import datetime

print("Cache service starting...", flush=True)
print("Loading session cache...", flush=True)

# Session cache - stores active sessions
cache = []
cache_entry_size = 2 * 1024 * 1024  # 2MB per session
start_time = time.time()

while True:
    # Add new session to cache
    session_data = bytearray(cache_entry_size)
    cache.append(session_data)

    cache_size_mb = (len(cache) * cache_entry_size) / (1024 * 1024)
    elapsed = int(time.time() - start_time)
    timestamp = datetime.now().strftime("%H:%M:%S")

    print(f"[{timestamp}] Cache size: {cache_size_mb:.1f}MB ({len(cache)} sessions, uptime: {elapsed}s)", flush=True)

    time.sleep(0.5)
