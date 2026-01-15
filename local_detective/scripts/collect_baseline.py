#!/usr/bin/env python3
"""Collect healthy baseline graphs from running Kubernetes cluster.

This script runs the collector multiple times and saves graph snapshots
for training the unsupervised Graph Auto-Encoder.
"""

import sys
import os
import time
from datetime import datetime
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

try:
    from src.host_proc_collector import HostProcCollector
    from src.graph_builder import RollingGraphBuilder
except ImportError:
    sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))
    from host_proc_collector import HostProcCollector
    from graph_builder import RollingGraphBuilder

import torch


def collect_baseline(
    num_samples: int = 60,
    interval_seconds: int = 10,
    output_dir: str = "models/healthy_baseline"
):
    """
    Collect healthy baseline graph snapshots.

    Args:
        num_samples: Number of graph samples to collect
        interval_seconds: Time between samples
        output_dir: Directory to save graph snapshots
    """
    print("=" * 70)
    print("Healthy Baseline Collection")
    print("=" * 70)
    print()
    print(f"Configuration:")
    print(f"  Samples: {num_samples}")
    print(f"  Interval: {interval_seconds}s")
    print(f"  Output: {output_dir}")
    print(f"  Total time: ~{num_samples * interval_seconds / 60:.1f} minutes")
    print()

    # Create output directory
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    # Detect proc root
    proc_root = "/host/proc" if os.path.exists("/host/proc") else "/proc"
    print(f"Using proc root: {proc_root}")
    print()

    # Initialize collector
    collector = HostProcCollector(proc_root=proc_root)

    # Collect samples
    successful_samples = 0
    failed_samples = 0

    for i in range(num_samples):
        start_time = time.time()
        timestamp = datetime.now()

        try:
            print(f"[{i+1}/{num_samples}] Collecting at {timestamp.strftime('%H:%M:%S')}...", end=" ")

            # Collect events
            events = collector.collect()

            # Build graph
            builder = RollingGraphBuilder(window_seconds=60)
            for event in events:
                builder.add_event(event)

            # Convert to PyG Data
            pyg_data = builder.to_pyg_data()

            # Validate graph
            if pyg_data.x.shape[0] == 0:
                print(f"⚠ Empty graph, skipping")
                failed_samples += 1
                continue

            # Save to disk
            filename = f"healthy_graph_{timestamp.strftime('%Y%m%d_%H%M%S')}.pt"
            filepath = output_path / filename
            torch.save(pyg_data, filepath)

            successful_samples += 1
            elapsed = time.time() - start_time

            print(f"✓ Saved {filename} ({pyg_data.x.shape[0]} nodes, {elapsed:.1f}s)")

        except Exception as e:
            print(f"❌ Error: {e}")
            failed_samples += 1
            continue

        # Sleep until next interval
        elapsed = time.time() - start_time
        sleep_time = max(0, interval_seconds - elapsed)
        if sleep_time > 0 and i < num_samples - 1:
            time.sleep(sleep_time)

    # Summary
    print()
    print("=" * 70)
    print("Collection Complete")
    print("=" * 70)
    print()
    print(f"Results:")
    print(f"  Successful: {successful_samples}/{num_samples} ({successful_samples/num_samples*100:.1f}%)")
    print(f"  Failed: {failed_samples}/{num_samples}")
    print(f"  Output directory: {output_path.absolute()}")
    print()

    if successful_samples >= 50:
        print("✓ Sufficient samples collected for training!")
    elif successful_samples >= 30:
        print("⚠ Marginal number of samples. Consider collecting more.")
    else:
        print("❌ Insufficient samples for training. Need at least 30.")

    print()
    print("Next steps:")
    print("  1. Train model: python scripts/train_gnn.py")
    print("  2. Test detection: python scripts/poc_runner.py --mode test")


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Collect healthy baseline graphs")
    parser.add_argument("--samples", type=int, default=60, help="Number of samples to collect")
    parser.add_argument("--interval", type=int, default=10, help="Interval between samples (seconds)")
    parser.add_argument("--output-dir", type=str, default="models/healthy_baseline", help="Output directory")

    args = parser.parse_args()

    collect_baseline(
        num_samples=args.samples,
        interval_seconds=args.interval,
        output_dir=args.output_dir
    )
