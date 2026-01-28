#!/usr/bin/env python3
import subprocess
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Task manager started")

    task_count = 0

    while True:
        task_count += 1

        # Spawn child process without waiting for it
        # This creates zombies since we don't reap them
        subprocess.Popen(["/bin/sh", "-c", "exit 0"])

        if task_count % 10 == 0:
            log(f"Dispatched {task_count} tasks")

        time.sleep(5)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Task manager stopped")
    except Exception as e:
        log(f"Error: {e}")
