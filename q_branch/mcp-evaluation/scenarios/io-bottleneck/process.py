#!/usr/bin/env python3
"""Log archival service."""
import os
import time
from datetime import datetime

ARCHIVE_DIR = "/data"
ARCHIVE_SIZE_MB = 10
CHUNK_SIZE = 4096

def log(msg):
    timestamp = datetime.now().strftime("%H:%M:%S")
    print(f"[{timestamp}] {msg}", flush=True)

log("Log archiver starting...")
log(f"Archive directory: {ARCHIVE_DIR}")

os.makedirs(ARCHIVE_DIR, exist_ok=True)

iteration = 0
while True:
    archive_file = os.path.join(ARCHIVE_DIR, f"archive_{iteration % 10}.log")

    # Write archive
    write_start = time.time()
    with open(archive_file, 'wb', buffering=0) as f:
        bytes_written = 0
        target_bytes = ARCHIVE_SIZE_MB * 1024 * 1024
        while bytes_written < target_bytes:
            f.write(os.urandom(CHUNK_SIZE))
            bytes_written += CHUNK_SIZE
    write_ms = (time.time() - write_start) * 1000

    # Verify archive
    read_start = time.time()
    with open(archive_file, 'rb', buffering=0) as f:
        while True:
            chunk = f.read(CHUNK_SIZE)
            if not chunk:
                break
    read_ms = (time.time() - read_start) * 1000

    total_ms = write_ms + read_ms
    log(f"Archive {iteration}: write={write_ms:.0f}ms, verify={read_ms:.0f}ms, total={total_ms:.0f}ms")

    iteration += 1

    # Small delay between archives
    time.sleep(0.5)
