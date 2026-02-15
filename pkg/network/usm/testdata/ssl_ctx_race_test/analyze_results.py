#!/usr/bin/env python3
"""
Analyze USM HTTP monitoring debug output to understand the race condition.

This script provides detailed analysis of:
1. Total captured vs expected requests
2. Correct vs misattributed requests
3. Missing requests (not captured at all)
4. Whether the issue is race condition or missing tcp_recvmsg hook

Usage:
    curl -s --unix-socket /opt/datadog-agent/run/sysprobe.sock \
        "http://unix/network_tracer/debug/http_monitoring" | python3 analyze_results.py

Or:
    python3 analyze_results.py < /tmp/http_debug.json
"""

import json
import sys
from collections import defaultdict


def analyze(data, expected_iterations=500):
    """Analyze HTTP monitoring data for race condition evidence."""

    # Filter to our test requests (contain conn1 or conn2 in path)
    conn1_requests = [e for e in data if 'conn1' in e.get('Path', '')]
    conn2_requests = [e for e in data if 'conn2' in e.get('Path', '')]

    # Expected: each connection should have requests going to its own port
    # conn1 -> port 18001
    # conn2 -> port 18002

    conn1_to_18001 = [e for e in conn1_requests if e.get('Server', {}).get('Port') == 18001]
    conn1_to_18002 = [e for e in conn1_requests if e.get('Server', {}).get('Port') == 18002]
    conn2_to_18001 = [e for e in conn2_requests if e.get('Server', {}).get('Port') == 18001]
    conn2_to_18002 = [e for e in conn2_requests if e.get('Server', {}).get('Port') == 18002]

    total_captured = len(conn1_requests) + len(conn2_requests)
    total_expected = expected_iterations * 2  # One request per connection per iteration

    correct = len(conn1_to_18001) + len(conn2_to_18002)
    wrong = len(conn1_to_18002) + len(conn2_to_18001)
    missing = total_expected - total_captured

    print("=" * 60)
    print("SSL Context Race Condition Analysis")
    print("=" * 60)
    print()

    print("CAPTURE STATISTICS:")
    print(f"  Expected requests:  {total_expected}")
    print(f"  Captured requests:  {total_captured}")
    print(f"  Missing requests:   {missing} ({100*missing/total_expected:.1f}%)")
    print()

    print("ATTRIBUTION ANALYSIS:")
    print(f"  conn1 -> 18001 (correct): {len(conn1_to_18001)}")
    print(f"  conn1 -> 18002 (WRONG):   {len(conn1_to_18002)}")
    print(f"  conn2 -> 18001 (WRONG):   {len(conn2_to_18001)}")
    print(f"  conn2 -> 18002 (correct): {len(conn2_to_18002)}")
    print()

    if total_captured > 0:
        print("RATES (of captured requests):")
        print(f"  Correctly attributed: {correct} ({100*correct/total_captured:.1f}%)")
        print(f"  Misattributed:        {wrong} ({100*wrong/total_captured:.1f}%)")
        print()

    print("=" * 60)
    print("DIAGNOSIS:")
    print("=" * 60)

    if missing > total_expected * 0.5:
        print()
        print(">>> HIGH MISSING RATE ({:.1f}%)".format(100*missing/total_expected))
        print("    This suggests the correlation is failing entirely.")
        print("    Possible causes:")
        print("    - Connections not using fallback path (started after monitoring)")
        print("    - tcp_sendmsg not firing for these requests")
        print("    - ssl_sock_by_ctx already populated (check test setup)")
        print()

    if wrong > 0 and total_captured > 0:
        print()
        print(">>> MISATTRIBUTION DETECTED ({:.1f}% of captured)".format(100*wrong/total_captured))
        print("    This is evidence of the RACE CONDITION.")
        print()
        print("    The race happens when:")
        print("    1. SSL_write(conn1) stores ctx1 in ssl_ctx_by_pid_tgid")
        print("    2. SSL_write(conn2) OVERWRITES with ctx2")
        print("    3. tcp_sendmsg for conn1 reads ctx2 -> WRONG tuple!")
        print()

        # Check if it's roughly symmetric (both directions equally wrong)
        if len(conn1_to_18002) > 0 and len(conn2_to_18001) > 0:
            ratio = len(conn1_to_18002) / len(conn2_to_18001)
            if 0.7 < ratio < 1.4:
                print("    Misattribution is SYMMETRIC (both directions equally affected).")
                print("    This confirms the race - contexts are being swapped randomly.")
            else:
                print("    Misattribution is ASYMMETRIC. This might indicate a different issue.")
        print()

    if correct > 0 and wrong == 0:
        print()
        print(">>> NO MISATTRIBUTION - All captured requests are correct!")
        print("    Possible explanations:")
        print("    - Race window is too narrow for this test")
        print("    - Connections are using primary path (ssl_sock_by_ctx hit)")
        print("    - tcp_sendmsg fires synchronously within SSL_write")
        print()

    # Extract iteration numbers to check for patterns
    print("=" * 60)
    print("DETAILED ITERATION ANALYSIS:")
    print("=" * 60)

    def get_iter(path):
        """Extract iteration number from path like /200/conn1-iter123"""
        try:
            return int(path.split('-iter')[1].split()[0].split('/')[0])
        except:
            return -1

    conn1_correct_iters = set(get_iter(e['Path']) for e in conn1_to_18001)
    conn1_wrong_iters = set(get_iter(e['Path']) for e in conn1_to_18002)
    conn2_correct_iters = set(get_iter(e['Path']) for e in conn2_to_18002)
    conn2_wrong_iters = set(get_iter(e['Path']) for e in conn2_to_18001)

    # Check first few iterations specifically
    print()
    print("First 10 iterations breakdown:")
    for i in range(10):
        c1 = "CORRECT" if i in conn1_correct_iters else ("WRONG" if i in conn1_wrong_iters else "MISSING")
        c2 = "CORRECT" if i in conn2_correct_iters else ("WRONG" if i in conn2_wrong_iters else "MISSING")
        print(f"  iter {i}: conn1={c1}, conn2={c2}")

    # Check if first iteration is different (ssl_sock_by_ctx gets populated)
    print()
    if 0 in conn1_correct_iters and 0 in conn2_correct_iters:
        print("First iteration is CORRECT for both connections.")
        print("This might mean ssl_sock_by_ctx gets populated on first use,")
        print("preventing the race on subsequent iterations.")
    elif 0 in conn1_wrong_iters or 0 in conn2_wrong_iters:
        print("First iteration shows MISATTRIBUTION!")
        print("This confirms the race happens from the very first request.")

    print()
    print("=" * 60)


if __name__ == "__main__":
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON: {e}", file=sys.stderr)
        print("Make sure to pipe valid JSON from the debug endpoint.", file=sys.stderr)
        sys.exit(1)

    # Try to detect iterations from data
    iterations = 500  # default
    for entry in data:
        path = entry.get('Path', '')
        if '-iter' in path:
            try:
                iter_num = int(path.split('-iter')[1].split()[0].split('/')[0])
                iterations = max(iterations, iter_num + 1)
            except:
                pass

    analyze(data, iterations)