#!/usr/bin/env python3
"""Data processing workload."""
import hashlib
import random
import string
import time
from datetime import datetime

print("Starting data processor", flush=True)
print("Processing data batches...", flush=True)

iteration = 0
while True:
    start_time = time.time()

    # CPU-intensive operation: compute many SHA256 hashes
    for i in range(10000):
        # Generate 64 byte random string
        random_string = ''.join(random.choices(string.ascii_letters + string.digits, k=64))
        # Calculate SHA256 hash
        hashlib.sha256(random_string.encode()).hexdigest()

    elapsed_ms = (time.time() - start_time) * 1000
    timestamp = datetime.now().strftime("%H:%M:%S")

    # Log completion time
    print(f"[{timestamp}] Batch {iteration} processed in {elapsed_ms:.1f}ms", flush=True)

    iteration += 1

    # Brief sleep to make output readable
    time.sleep(0.1)
