#!/usr/bin/env python3
import os
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Cache manager started")

    cache_dir = "/tmp/cache_files"
    os.makedirs(cache_dir, exist_ok=True)

    file_count = 0
    batch_size = 1000

    while True:
        try:
            # Create many tiny files
            for i in range(batch_size):
                file_count += 1
                filepath = os.path.join(cache_dir, f"cache_{file_count:08d}.tmp")

                try:
                    with open(filepath, 'w') as f:
                        f.write('x')  # 1 byte file
                except OSError as e:
                    log(f"File creation error: {e}")
                    time.sleep(10)
                    break

            if file_count % 10000 == 0:
                log(f"Cache entries: {file_count}")

            time.sleep(1)

        except Exception as e:
            log(f"Error: {e}")
            time.sleep(10)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Cache manager stopped")
    except Exception as e:
        log(f"Fatal error: {e}")
