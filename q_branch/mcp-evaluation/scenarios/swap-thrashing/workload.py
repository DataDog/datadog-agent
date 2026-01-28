#!/usr/bin/env python3
import time
from datetime import datetime
import multiprocessing


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def worker(worker_id, size_gb):
    """Worker process that allocates memory"""
    size_bytes = size_gb * 1024 * 1024 * 1024
    chunk_size = 100 * 1024 * 1024  # 100MB chunks

    data = []
    allocated = 0

    while allocated < size_bytes:
        try:
            chunk = bytearray(chunk_size)
            # Touch the memory to force allocation
            for i in range(0, len(chunk), 4096):
                chunk[i] = 1
            data.append(chunk)
            allocated += chunk_size
        except MemoryError:
            break

        time.sleep(0.1)

    # Keep memory allocated
    while True:
        time.sleep(60)


def main():
    log("Data processor started")

    # Get available memory and try to use more
    # Spawn 4 workers, each trying to allocate 2.5GB
    num_workers = 4
    mem_per_worker_gb = 2.5

    log(f"Starting {num_workers} workers")

    processes = []
    for i in range(num_workers):
        p = multiprocessing.Process(target=worker, args=(i, mem_per_worker_gb))
        p.start()
        processes.append(p)
        log(f"Started worker {i}")
        time.sleep(2)

    # Wait for workers
    for p in processes:
        p.join()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Data processor stopped")
    except Exception as e:
        log(f"Error: {e}")
