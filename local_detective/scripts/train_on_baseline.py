#!/usr/bin/env python3
"""Train Graph Auto-Encoder on collected healthy baseline graphs."""

import sys
import os
import glob
import argparse
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

import torch
from src.gnn_model import GraphAutoEncoder, AutoEncoderTrainer, RootCauseDetector
import numpy as np


def load_baseline_graphs(baseline_dir: str):
    """Load all baseline graph snapshots from disk."""
    pattern = os.path.join(baseline_dir, "healthy_graph_*.pt")
    files = sorted(glob.glob(pattern))

    if not files:
        raise ValueError(f"No baseline graphs found in {baseline_dir}")

    print(f"Loading {len(files)} baseline graphs from {baseline_dir}...")
    graphs = []

    for filepath in files:
        try:
            data = torch.load(filepath, weights_only=False)
            graphs.append(data)
        except Exception as e:
            print(f"  Warning: Failed to load {os.path.basename(filepath)}: {e}")

    print(f"Successfully loaded {len(graphs)} graphs")
    return graphs


def print_graph_statistics(graphs):
    """Print statistics about the loaded graphs."""
    node_counts = [g.x.shape[0] for g in graphs]
    edge_counts = [g.edge_index.shape[1] for g in graphs]

    print(f"\nGraph Statistics:")
    print(f"  Total graphs: {len(graphs)}")
    print(f"  Node counts: min={min(node_counts)}, max={max(node_counts)}, mean={np.mean(node_counts):.1f}")
    print(f"  Edge counts: min={min(edge_counts)}, max={max(edge_counts)}, mean={np.mean(edge_counts):.1f}")
    print(f"  Feature dimension: {graphs[0].x.shape[1]}D")


def main():
    parser = argparse.ArgumentParser(
        description="Train Graph Auto-Encoder on collected baseline data"
    )

    parser.add_argument(
        "--baseline-dir",
        default="models/healthy_baseline",
        help="Directory containing baseline graph snapshots"
    )

    parser.add_argument(
        "--epochs",
        type=int,
        default=100,
        help="Number of training epochs"
    )

    parser.add_argument(
        "--learning-rate",
        type=float,
        default=0.001,
        help="Learning rate"
    )

    parser.add_argument(
        "--hidden-dim",
        type=int,
        default=32,
        help="Hidden dimension"
    )

    parser.add_argument(
        "--latent-dim",
        type=int,
        default=16,
        help="Latent dimension"
    )

    parser.add_argument(
        "--output",
        default="models/trained_autoencoder.pt",
        help="Output model path"
    )

    args = parser.parse_args()

    print("\n" + "="*70)
    print("Graph Auto-Encoder Training on Real Baseline Data")
    print("="*70 + "\n")

    print(f"Configuration:")
    print(f"  Baseline directory: {args.baseline_dir}")
    print(f"  Epochs: {args.epochs}")
    print(f"  Learning rate: {args.learning_rate}")
    print(f"  Hidden dim: {args.hidden_dim}")
    print(f"  Latent dim: {args.latent_dim}")
    print(f"  Output: {args.output}")
    print()

    # Load baseline graphs
    print("="*70)
    print("Phase 1: Loading Baseline Graphs")
    print("="*70 + "\n")

    graphs = load_baseline_graphs(args.baseline_dir)
    print_graph_statistics(graphs)

    if len(graphs) < 10:
        print(f"\n⚠ Warning: Only {len(graphs)} graphs found. Recommend at least 30 for good training.")

    # Create model
    print("\n" + "="*70)
    print("Phase 2: Creating Model")
    print("="*70 + "\n")

    input_dim = graphs[0].x.shape[1]
    model = GraphAutoEncoder(
        input_dim=input_dim,
        hidden_dim=args.hidden_dim,
        latent_dim=args.latent_dim
    )

    num_params = sum(p.numel() for p in model.parameters())
    print(f"Model Architecture:")
    print(f"  Encoder: {input_dim}D → {args.hidden_dim}D → {args.latent_dim}D")
    print(f"  Decoder: {args.latent_dim}D → {args.hidden_dim}D → {input_dim}D")
    print(f"  Total parameters: {num_params:,}")

    # Train
    print("\n" + "="*70)
    print("Phase 3: Training")
    print("="*70 + "\n")

    trainer = AutoEncoderTrainer(model, learning_rate=args.learning_rate)

    print(f"Training on {len(graphs)} healthy graphs for {args.epochs} epochs...")
    print("Objective: Learn to reconstruct normal system patterns\n")

    losses = trainer.train(graphs, epochs=args.epochs, verbose=True)

    # Evaluate
    print("\n" + "="*70)
    print("Phase 4: Evaluation")
    print("="*70 + "\n")

    metrics = trainer.evaluate(graphs)

    print("Reconstruction Quality:")
    print(f"  Mean reconstruction loss: {metrics['mean_reconstruction_loss']:.6f}")
    print(f"  Std reconstruction loss:  {metrics['std_reconstruction_loss']:.6f}")
    print(f"  Mean node error:          {metrics['mean_node_error']:.6f}")
    print(f"  Max node error:           {metrics['max_node_error']:.6f}")

    # Compute anomaly threshold
    threshold = trainer.compute_anomaly_threshold(graphs, percentile=95.0)
    print(f"\n  Anomaly threshold (95th percentile): {threshold:.6f}")
    print("  Nodes with reconstruction error > threshold will be flagged")

    # Plot training curve
    print("\n" + "="*70)
    print("Phase 5: Training Visualization")
    print("="*70 + "\n")

    print("Loss progression (every 10 epochs):")
    for i in range(0, len(losses), 10):
        epoch = i + 1
        loss = losses[i]
        bar_length = int(50 * (1 - min(loss, 0.1) / 0.1))
        bar = "█" * bar_length + "░" * (50 - bar_length)
        print(f"  Epoch {epoch:3d}: {loss:.6f} {bar}")

    if len(losses) >= 10:
        improvement = (losses[0] - losses[-1]) / losses[0] * 100
        print(f"\n  Loss reduction: {improvement:.1f}%")

    # Save model
    print("\n" + "="*70)
    print("Phase 6: Saving Model")
    print("="*70 + "\n")

    os.makedirs(os.path.dirname(args.output), exist_ok=True)
    detector = RootCauseDetector(model)
    detector.save(args.output)

    print(f"✓ Model saved to: {args.output}")

    # Summary
    print("\n" + "="*70)
    print("Training Summary")
    print("="*70 + "\n")

    print(f"✓ Trained on {len(graphs)} real system graphs")
    print(f"✓ Final reconstruction loss: {losses[-1]:.6f}")
    print(f"✓ Anomaly detection threshold: {threshold:.6f}")
    print(f"✓ Model ready for deployment")

    print("\nThe model has learned:")
    print("  • Normal CPU/memory/network patterns")
    print("  • Typical process relationships")
    print("  • Expected system pressure (PSI) levels")
    print("  • Baseline latency and blocking patterns")

    print("\nNext steps:")
    print("  • Deploy as DaemonSet: kubectl apply -f deploy/daemonset.yaml")
    print("  • Test failure detection: inject failures and monitor anomaly scores")

    print("\n" + "="*70)
    print("Training Complete!")
    print("="*70 + "\n")


if __name__ == "__main__":
    main()
