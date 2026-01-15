#!/usr/bin/env python3
import time
from datetime import datetime


def log(msg):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {msg}", flush=True)


def main():
    log("Metrics collector started")

    file_handles = []
    count = 0

    while True:
        try:
            # Open files without closing them
            for _ in range(10):
                fh = open('/dev/null', 'r')
                file_handles.append(fh)  # Keep reference to prevent GC
                count += 1

            if count % 100 == 0:
                log(f"Collected {count} metric sources")

            time.sleep(1)

        except OSError as e:
            log(f"Error opening metric source: {e}")
            time.sleep(5)
        except Exception as e:
            log(f"Unexpected error: {e}")
            time.sleep(5)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("Metrics collector stopped")
    except Exception as e:
        log(f"Fatal error: {e}")
