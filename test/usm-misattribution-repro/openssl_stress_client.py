#!/usr/bin/env python3
"""
Stress test client for OpenSSL misattribution reproduction.
Creates rapid TLS connections to multiple servers to trigger potential issues.
"""

import argparse
import ssl
import socket
import threading
import random
import time
import gc
import sys
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor

# Global stats
stats = defaultdict(int)
stats_lock = threading.Lock()

def make_request(server, port, path, skip_close_rate):
    """Make a single HTTPS request."""
    sock = None
    ssock = None
    try:
        # Create NEW SSL context per request to maximize memory churn
        # This matches Go's behavior of creating new Transport per request
        ssl_context = ssl.create_default_context()
        ssl_context.check_hostname = False
        ssl_context.verify_mode = ssl.CERT_NONE

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(10)
        sock.connect((server, port))

        ssock = ssl_context.wrap_socket(sock, server_hostname=server)

        # Send HTTP request
        request = f"GET {path} HTTP/1.1\r\nHost: {server}:{port}\r\nConnection: close\r\n\r\n"
        ssock.sendall(request.encode())

        # Read response
        response = b""
        while True:
            chunk = ssock.recv(4096)
            if not chunk:
                break
            response += chunk

        with stats_lock:
            stats[f'server{port}_requests'] += 1

        # Randomly skip close to simulate leaked connections
        if random.random() < skip_close_rate:
            with stats_lock:
                stats['skipped_close'] += 1
            # Don't close! Let GC handle it (or not)
            return

        ssock.close()
        sock.close()

    except Exception as e:
        with stats_lock:
            stats['errors'] += 1
        if ssock:
            try:
                ssock.close()
            except:
                pass
        if sock:
            try:
                sock.close()
            except:
                pass

def worker(server1, port1, server2, port2, duration, skip_close_rate, request_rate):
    """Worker thread that makes requests to both servers."""
    paths1 = ['/', '/health', f'/server{port1}/identify', '/api/v1/data']
    paths2 = ['/', '/health', f'/server{port2}/identify', '/api/v1/users']

    end_time = time.time() + duration

    while time.time() < end_time:
        # Randomly pick a server
        if random.random() < 0.5:
            server, port, paths = server1, port1, paths1
        else:
            server, port, paths = server2, port2, paths2

        path = random.choice(paths)
        make_request(server, port, path, skip_close_rate)

        time.sleep(request_rate)

        # Occasionally force GC to trigger memory reuse
        if random.randint(0, 100) < 5:
            gc.collect()

def print_progress(duration):
    """Print progress every 5 seconds."""
    end_time = time.time() + duration
    while time.time() < end_time:
        time.sleep(5)
        with stats_lock:
            print(f"Progress: server8443={stats['server8443_requests']}, "
                  f"server9443={stats['server9443_requests']}, "
                  f"errors={stats['errors']}, "
                  f"skipped_close={stats['skipped_close']}")
            sys.stdout.flush()

def main():
    parser = argparse.ArgumentParser(description='OpenSSL stress test client')
    parser.add_argument('-server1', default='localhost:8443', help='First server address')
    parser.add_argument('-server2', default='localhost:9443', help='Second server address')
    parser.add_argument('-duration', type=int, default=60, help='Test duration in seconds')
    parser.add_argument('-concurrency', type=int, default=50, help='Number of concurrent workers')
    parser.add_argument('-skip-close', type=float, default=0.1, help='Fraction of connections to skip Close()')
    parser.add_argument('-rate', type=float, default=0.01, help='Delay between requests per worker (seconds)')
    args = parser.parse_args()

    # Parse server addresses
    server1, port1 = args.server1.rsplit(':', 1)
    port1 = int(port1)
    server2, port2 = args.server2.rsplit(':', 1)
    port2 = int(port2)

    print(f"Starting stress test:")
    print(f"  Server 1: {server1}:{port1}")
    print(f"  Server 2: {server2}:{port2}")
    print(f"  Duration: {args.duration}s")
    print(f"  Concurrency: {args.concurrency}")
    print(f"  Skip Close rate: {args.skip_close * 100:.1f}%")
    print(f"  SSL context: NEW per request (matches Go behavior)")
    sys.stdout.flush()

    # Start progress reporter
    progress_thread = threading.Thread(target=print_progress, args=(args.duration,))
    progress_thread.daemon = True
    progress_thread.start()

    # Start workers
    threads = []
    for i in range(args.concurrency):
        t = threading.Thread(
            target=worker,
            args=(server1, port1, server2, port2, args.duration,
                  args.skip_close, args.rate)
        )
        t.start()
        threads.append(t)

    # Wait for all workers
    for t in threads:
        t.join()

    print(f"\nTest complete:")
    print(f"  Server 1 ({port1}) requests: {stats[f'server{port1}_requests']}")
    print(f"  Server 2 ({port2}) requests: {stats[f'server{port2}_requests']}")
    print(f"  Errors: {stats['errors']}")
    print(f"  Skipped Close: {stats['skipped_close']}")

if __name__ == '__main__':
    main()