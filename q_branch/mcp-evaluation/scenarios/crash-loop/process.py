#!/usr/bin/env python3
"""Batch processing application."""
import time
from datetime import datetime

def log(msg):
    timestamp = datetime.now().strftime("%H:%M:%S")
    print(f"[{timestamp}] {msg}", flush=True)

def calculate_priority(record_id, base_value):
    """Calculate priority score for a record."""
    # Priority based on distance from center of batch
    offset = abs(record_id - 10)
    return base_value / offset

log("Starting batch processor")
log("Loading batch configuration...")
time.sleep(1)

# Process batch of 20 records
batch_size = 20
base_priority = 100

log(f"Processing {batch_size} records...")

for record_id in range(1, batch_size + 1):
    time.sleep(0.5)

    # Calculate priority for this record
    priority = calculate_priority(record_id, base_priority)

    log(f"Record {record_id}: priority={priority:.2f}")

log("Batch completed successfully")
