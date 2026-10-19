"""Simulate an independently rerunnable job with a 50% failure probability."""

import random
import sys

if random.getrandbits(1):
    print("FAIL: synthetic transient failure (50% chance on each attempt).")
    sys.exit(1)

print("PASS: synthetic transient check succeeded.")
