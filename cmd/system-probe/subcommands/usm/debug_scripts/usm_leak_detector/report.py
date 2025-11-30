"""
Report generation for leak detection results.
"""

from typing import Dict, List

from .models import MapLeakInfo, PIDLeakInfo


def print_report(results: List[MapLeakInfo], namespaces: Dict[int, int], verbose: bool = False):
    """Print human-readable leak report for ConnTuple-keyed maps.

    Args:
        results: List of MapLeakInfo from analysis
        namespaces: Dict mapping netns inodes to representative PIDs
        verbose: Enable verbose output (shows namespace details)
    """
    print("USM eBPF Map Leak Detection (ConnTuple-Keyed Maps)")
    print("=" * 60)
    print()

    print(f"Network Namespaces Discovered: {len(namespaces)}")
    if verbose:
        for netns_id, pid in sorted(namespaces.items()):
            print(f"  - {netns_id} (pid={pid})")
    print()

    print("## Connection Tuple-Keyed Maps")
    print("-" * 60)

    total_maps = 0
    maps_with_leaks = 0
    total_leaked = 0
    total_race_fps = 0

    for info in results:
        total_maps += 1
        total_leaked += info.leaked
        total_race_fps += info.race_condition_fps
        if info.leaked > 0:
            maps_with_leaks += 1

        valid_pct = info.valid_rate * 100
        print(f"{info.name}: {info.total - info.leaked}/{info.total} entries ({valid_pct:.1f}% valid)")

        if info.leaked > 0:
            print(f"  Leaked entries: {info.leaked}")
            for conn, reason in info.samples[:10]:  # Show max 10 samples
                print(f"    {conn} [{reason}]")
            if len(info.samples) > 10:
                print(f"    ... and {len(info.samples) - 10} more")
        else:
            print("  No leaks detected")

        if info.race_condition_fps > 0:
            print(f"  Race condition false positives filtered: {info.race_condition_fps}")
        print()

    print("## Summary")
    print("-" * 60)
    print(f"Total maps checked: {total_maps}")
    print(f"Maps with leaks: {maps_with_leaks}")
    print(f"Total leaked entries: {total_leaked}")
    if total_race_fps > 0:
        print(f"Race condition false positives filtered: {total_race_fps}")


def print_pid_report(results: List[PIDLeakInfo], verbose: bool = False):
    """Print human-readable leak report for PID-keyed maps.

    Args:
        results: List of PIDLeakInfo from analysis
        verbose: Enable verbose output
    """
    print("USM eBPF Map Leak Detection (PID-Keyed Maps)")
    print("=" * 60)
    print()

    total_maps = 0
    maps_with_leaks = 0
    total_leaked = 0

    for info in results:
        total_maps += 1
        total_leaked += info.leaked
        if info.leaked > 0:
            maps_with_leaks += 1

        if info.leaked == 0:
            print(f"{info.name}: {info.leaked}/{info.total} entries (0.0% leaked)")
        else:
            leak_pct = info.leak_rate * 100
            print(f"{info.name}: {info.leaked}/{info.total} entries ({leak_pct:.1f}% leaked)")

        if info.leaked > 0 and info.dead_pids:
            # Show up to 20 dead PIDs
            shown_pids = info.dead_pids[:20]
            print(f"  Dead PIDs: {shown_pids}")
            if len(info.dead_pids) > 20:
                print(f"    ... and {len(info.dead_pids) - 20} more")
        elif info.leaked == 0:
            print("  No leaks detected")
        print()

    print("## Summary")
    print("-" * 60)
    print(f"Total maps checked: {total_maps}")
    print(f"Maps with leaks: {maps_with_leaks}")
    print(f"Total leaked entries: {total_leaked}")
