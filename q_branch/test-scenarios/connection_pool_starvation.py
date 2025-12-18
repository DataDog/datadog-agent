#!/usr/bin/env python3
"""
Connection Pool Starvation - Feast/Famine Pattern

Simulates an application with an undersized database connection pool.

K8s sees: Running, healthy, ~25% average CPU
At 15s: "Looks fine, moderate CPU usage"
At 1Hz: Clear bimodal distribution - HIGH (processing) or NEAR-ZERO (blocked)
"""

import sys
import threading
import time

POOL_SIZE = 2
NUM_WORKERS = 8
WORK_DURATION = 2.0


class ConnectionPool:
    def __init__(self, size):
        self.semaphore = threading.Semaphore(size)
        self.size = size
        self.active = 0
        self.lock = threading.Lock()

    def acquire(self):
        self.semaphore.acquire()
        with self.lock:
            self.active += 1

    def release(self):
        with self.lock:
            self.active -= 1
        self.semaphore.release()

    def get_active(self):
        with self.lock:
            return self.active


def cpu_intensive_work(duration):
    end_time = time.time() + duration
    result = 0
    while time.time() < end_time:
        for i in range(10000):
            result += i * i
    return result


def worker(worker_id, pool, stats):
    while True:
        wait_start = time.time()
        pool.acquire()
        wait_time = time.time() - wait_start

        try:
            work_start = time.time()
            cpu_intensive_work(WORK_DURATION)
            work_time = time.time() - work_start

            with stats['lock']:
                stats['queries'] += 1
                stats['total_wait'] += wait_time
                stats['total_work'] += work_time
        finally:
            pool.release()

        time.sleep(0.1)


def monitor(pool, stats):
    last_queries = 0
    while True:
        time.sleep(5)
        with stats['lock']:
            queries = stats['queries']
            total_wait = stats['total_wait']
            total_work = stats['total_work']

        qps = (queries - last_queries) / 5
        avg_wait = total_wait / max(queries, 1)
        avg_work = total_work / max(queries, 1)

        print(
            f"[Monitor] Pool: {pool.get_active()}/{pool.size} | "
            f"Queries: {queries} ({qps:.1f}/s) | "
            f"Avg wait: {avg_wait:.2f}s | Avg work: {avg_work:.2f}s"
        )
        sys.stdout.flush()
        last_queries = queries


def main():
    print("=" * 60)
    print("Connection Pool Starvation Demo")
    print("=" * 60)
    print(f"Pool size: {POOL_SIZE}, Workers: {NUM_WORKERS}")
    print("Expected: Bimodal CPU - HIGH (processing) or LOW (blocked)")
    print("=" * 60)
    sys.stdout.flush()

    pool = ConnectionPool(POOL_SIZE)
    stats = {'queries': 0, 'total_wait': 0.0, 'total_work': 0.0, 'lock': threading.Lock()}

    threading.Thread(target=monitor, args=(pool, stats), daemon=True).start()

    for i in range(NUM_WORKERS):
        threading.Thread(target=worker, args=(i, pool, stats), daemon=True).start()
        time.sleep(0.05)

    while True:
        time.sleep(60)


if __name__ == "__main__":
    main()
