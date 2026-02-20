"""
Command-line interface for the USM leak detector.
"""

import argparse
import os
import sys

from .backends import get_backend
from .constants import DEFAULT_PROC_ROOT, DEFAULT_RECHECK_DELAY
from .logging_config import configure_logging, logger
from .map_discovery import find_conn_tuple_maps
from .map_utils import filter_maps_by_names
from .network import discover_namespaces, build_connection_index
from .analyzer import analyze_map
from .pid_validator import find_pid_keyed_maps, analyze_pid_map
from .report import print_report, print_pid_report


def parse_args():
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Detect leaked entries in USM eBPF maps"
    )
    parser.add_argument(
        "--maps",
        type=str,
        help="Comma-separated list of specific map names to check"
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Enable verbose output"
    )
    parser.add_argument(
        "--proc-root",
        type=str,
        default=DEFAULT_PROC_ROOT,
        help=f"Path to /proc filesystem (default: {DEFAULT_PROC_ROOT})"
    )
    parser.add_argument(
        "--system-probe",
        type=str,
        metavar="PATH",
        help="Path to system-probe binary (auto-detected if not specified)"
    )
    parser.add_argument(
        "--conn-tuple-only",
        action="store_true",
        help="Only check ConnTuple-keyed maps (skip PID-keyed maps)"
    )
    parser.add_argument(
        "--pid-only",
        action="store_true",
        help="Only check PID-keyed maps (skip ConnTuple-keyed maps)"
    )
    parser.add_argument(
        "--recheck-delay",
        type=float,
        default=DEFAULT_RECHECK_DELAY,
        metavar="SECONDS",
        help=f"Delay before re-checking leaked entries to filter race conditions (default: {DEFAULT_RECHECK_DELAY}, 0 to disable)"
    )

    return parser.parse_args()


def main():
    """Main entry point for the USM leak detector."""
    args = parse_args()

    # Configure logging based on verbosity
    configure_logging(args.verbose)

    # Check for root privileges
    if os.geteuid() != 0:
        print("Warning: Not running as root. May not have access to all maps/namespaces.",
              file=sys.stderr)

    if not args.verbose:
        print("Analyzing USM eBPF maps (this may take a few seconds)...")

    # Step 1: Get eBPF backend
    backend = get_backend(system_probe_path=args.system_probe)
    if backend is None:
        print("Error: No eBPF backend available. Install bpftool or ensure system-probe is accessible.",
              file=sys.stderr)
        sys.exit(1)

    # Step 2: List eBPF maps
    logger.debug("Listing eBPF maps...")
    all_maps = backend.list_maps()
    if not all_maps:
        print("Error: Could not list eBPF maps.", file=sys.stderr)
        sys.exit(1)

    conn_tuple_results = []
    pid_results = []
    namespaces = {}

    # Step 3: Check ConnTuple-keyed maps (unless --pid-only)
    if not args.pid_only:
        conn_tuple_maps = find_conn_tuple_maps(all_maps)

        logger.debug(f"Found {len(conn_tuple_maps)} ConnTuple-keyed maps: {list(conn_tuple_maps.keys())}")

        # Filter to specific maps if requested
        if args.maps:
            requested = set(args.maps.split(","))
            conn_tuple_maps = filter_maps_by_names(conn_tuple_maps, requested)

        if conn_tuple_maps:
            # Discover network namespaces
            logger.debug("Discovering network namespaces...")
            namespaces = discover_namespaces(args.proc_root)
            if not namespaces:
                print("Warning: No network namespaces discovered.", file=sys.stderr)

            logger.debug(f"Found {len(namespaces)} namespaces")

            # Build connection indexes
            logger.debug("Building connection indexes...")
            connection_index = build_connection_index(namespaces, args.proc_root)

            # Analyze each ConnTuple-keyed map
            for map_name in sorted(conn_tuple_maps.keys()):
                logger.debug(f"Analyzing ConnTuple map: {map_name}...")
                info = analyze_map(
                    map_name, backend, connection_index,
                    recheck_delay=args.recheck_delay, proc_root=args.proc_root
                )
                conn_tuple_results.append(info)

    # Step 4: Check PID-keyed maps (unless --conn-tuple-only)
    if not args.conn_tuple_only:
        pid_maps = find_pid_keyed_maps(all_maps)

        logger.debug(f"Found {len(pid_maps)} PID-keyed maps: {list(pid_maps.keys())}")

        # Filter to specific maps if requested
        if args.maps:
            requested = set(args.maps.split(","))
            pid_maps = filter_maps_by_names(pid_maps, requested)

        # Analyze each PID-keyed map
        for map_name in sorted(pid_maps.keys()):
            logger.debug(f"Analyzing PID-keyed map: {map_name}...")
            info = analyze_pid_map(map_name, backend, args.proc_root)
            pid_results.append(info)

    # Check if we found anything
    if not conn_tuple_results and not pid_results:
        print("No USM maps found. Is system-probe running with USM enabled?", file=sys.stderr)
        sys.exit(1)

    # Step 5: Print reports (PID-keyed first, then ConnTuple-keyed)
    print()
    if pid_results:
        print_pid_report(pid_results)

    if conn_tuple_results:
        if pid_results:
            print()  # Add separator between reports
        print_report(conn_tuple_results, namespaces)


if __name__ == "__main__":
    main()
