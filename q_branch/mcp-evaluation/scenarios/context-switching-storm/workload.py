#!/usr/bin/env python3
import threading
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def worker_thread(thread_id, lock, condition):
    """Worker thread that constantly acquires lock and signals condition"""
    while True:
        with condition:
            condition.notify_all()
            condition.wait(timeout=0.001)  # Very short timeout


def main():
    log("Task coordinator started")

    # Create many threads with shared condition variable
    num_threads = 50
    lock = threading.Lock()
    condition = threading.Condition(lock)

    threads = []

    for i in range(num_threads):
        t = threading.Thread(target=worker_thread, args=(i, lock, condition))
        t.daemon = True
        t.start()
        threads.append(t)

    log(f"Started {num_threads} coordinator threads")

    # Keep main thread alive
    while True:
        time.sleep(60)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Task coordinator stopped")
    except Exception as e:
        log(f"Error: {e}")
