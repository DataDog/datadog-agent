#!/usr/bin/env python3
import os
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def check_disk_space(path="/tmp"):
    """Check available disk space, return True if we should continue writing"""
    stat = os.statvfs(path)
    available_gb = (stat.f_bavail * stat.f_frsize) / (1024**3)
    total_gb = (stat.f_blocks * stat.f_frsize) / (1024**3)
    used_percent = ((total_gb - available_gb) / total_gb) * 100

    # Safety limit: stop if disk is 95% full or less than 2GB available
    if used_percent >= 95 or available_gb < 2:
        return False
    return True


def main():
    log("Archive manager started")

    # Create output directory
    output_dir = "/tmp/data_archives"
    os.makedirs(output_dir, exist_ok=True)

    file_count = 0
    chunk_size = 100 * 1024 * 1024  # 100MB per file

    while True:
        if not check_disk_space():
            log("Storage threshold reached, pausing operations")
            time.sleep(60)
            continue

        file_count += 1
        filename = os.path.join(output_dir, f"archive_{file_count:06d}.dat")

        try:
            # Write file
            with open(filename, 'wb') as f:
                f.write(os.urandom(chunk_size))

            log(f"Archived segment {file_count}")

        except IOError as e:
            log(f"Archive operation failed: {e}")
            time.sleep(10)

        time.sleep(10)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Archive manager stopped")
    except Exception as e:
        log(f"Error: {e}")
