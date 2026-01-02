"""
USM eBPF Map Leak Detector

Detects leaked entries in USM (Universal Service Monitoring) eBPF maps by
validating ConnTuple-keyed map entries against active TCP connections.
"""

__version__ = "1.0.0"
