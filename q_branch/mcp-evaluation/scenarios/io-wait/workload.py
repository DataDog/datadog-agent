#!/usr/bin/env python3
import os
import time
from datetime import datetime
import multiprocessing


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def writer_process(worker_id):
    """Process that does synchronous disk writes"""
    filename = f"/tmp/io_test_{worker_id}.dat"
    chunk_size = 10 * 1024 * 1024  # 10MB
    iteration = 0

    while True:
        try:
            with open(filename, 'wb') as f:
                data = os.urandom(chunk_size)
                f.write(data)
                # Force synchronous write to disk
                f.flush()
                os.fsync(f.fileno())

            iteration += 1

            if iteration % 10 == 0:
                os.remove(filename)  # Clean up periodically

        except Exception:
            pass

        time.sleep(0.1)


def main():
    log("Storage sync service started")

    # Spawn 4 writer processes to create I/O contention
    num_workers = 4
    processes = []

    for i in range(num_workers):
        p = multiprocessing.Process(target=writer_process, args=(i,))
        p.start()
        processes.append(p)

    log(f"Started {num_workers} sync workers")

    # Wait for workers
    for p in processes:
        p.join()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Storage sync service stopped")
    except Exception as e:
        log(f"Error: {e}")
