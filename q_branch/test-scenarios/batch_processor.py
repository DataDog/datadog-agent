#!/usr/bin/env python3
# ruff: noqa
"""
Batch Micro-Processing - Queue Consumer Pattern

Simulates an application that processes messages from a queue in batches.

K8s sees: Running, healthy
At 15s: "Averaging 35% CPU, seems fine"
At 1Hz: Clear sawtooth - spike during batch, valley while waiting
"""

import queue
import random
import sys
import threading
import time

MIN_BATCH_INTERVAL = 3.0
MAX_BATCH_INTERVAL = 8.0
MIN_BATCH_SIZE = 50
MAX_BATCH_SIZE = 200


def cpu_work_for_message():
    result = 0
    for i in range(5000):
        result += i * i
    return result


def message_producer(msg_queue, stats):
    batch_num = 0
    while True:
        interval = random.uniform(MIN_BATCH_INTERVAL, MAX_BATCH_INTERVAL)
        time.sleep(interval)

        batch_size = random.randint(MIN_BATCH_SIZE, MAX_BATCH_SIZE)
        batch_num += 1

        for i in range(batch_size):
            msg_queue.put({'batch': batch_num, 'index': i, 'timestamp': time.time()})

        with stats['lock']:
            stats['batches_received'] += 1
            stats['messages_received'] += batch_size

        print(f"[Producer] Batch {batch_num}: {batch_size} msgs (next in {interval:.1f}s)")
        sys.stdout.flush()


def message_consumer(msg_queue, stats):
    while True:
        batch = []
        try:
            msg = msg_queue.get(timeout=1.0)
            batch.append(msg)
        except queue.Empty:
            continue

        while True:
            try:
                batch.append(msg_queue.get_nowait())
            except queue.Empty:
                break

        if not batch:
            continue

        process_start = time.time()
        for msg in batch:
            cpu_work_for_message()

        process_time = time.time() - process_start
        oldest_msg = min(m['timestamp'] for m in batch)
        lag = time.time() - oldest_msg

        with stats['lock']:
            stats['messages_processed'] += len(batch)
            stats['batches_processed'] += 1
            stats['total_process_time'] += process_time

        print(f"[Consumer] Processed {len(batch)} msgs in {process_time:.2f}s (lag: {lag:.2f}s)")
        sys.stdout.flush()


def monitor(msg_queue, stats):
    while True:
        time.sleep(10)
        with stats['lock']:
            received = stats['messages_received']
            processed = stats['messages_processed']
            batches = stats['batches_processed']
            proc_time = stats['total_process_time']

        backlog = received - processed
        avg_process = proc_time / max(batches, 1)

        print(
            f"[Monitor] Received: {received} | Processed: {processed} | "
            f"Backlog: {backlog} | Avg batch: {avg_process:.2f}s"
        )
        sys.stdout.flush()


def main():
    print("=" * 60)
    print("Batch Micro-Processing Demo")
    print("=" * 60)
    print(f"Batch interval: {MIN_BATCH_INTERVAL}-{MAX_BATCH_INTERVAL}s")
    print(f"Batch size: {MIN_BATCH_SIZE}-{MAX_BATCH_SIZE} messages")
    print("Expected: Sawtooth CPU - spike during batch, valley while waiting")
    print("=" * 60)
    sys.stdout.flush()

    msg_queue = queue.Queue()
    stats = {
        'batches_received': 0,
        'messages_received': 0,
        'batches_processed': 0,
        'messages_processed': 0,
        'total_process_time': 0.0,
        'lock': threading.Lock(),
    }

    threading.Thread(target=message_producer, args=(msg_queue, stats), daemon=True).start()
    threading.Thread(target=message_consumer, args=(msg_queue, stats), daemon=True).start()
    threading.Thread(target=monitor, args=(msg_queue, stats), daemon=True).start()

    while True:
        time.sleep(60)


if __name__ == "__main__":
    main()
