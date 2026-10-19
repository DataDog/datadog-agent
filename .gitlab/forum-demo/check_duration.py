"""Check the demo's milliseconds-to-seconds conversion without dependencies."""

from duration import milliseconds_to_seconds

for milliseconds, expected in [(0, 0.0), (1000, 1.0), (2500, 2.5)]:
    actual = milliseconds_to_seconds(milliseconds)
    assert actual == expected, f"milliseconds_to_seconds({milliseconds}): expected {expected}, got {actual}"

print("PASS: milliseconds-to-seconds conversion.")
