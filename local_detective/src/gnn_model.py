"""Graph Auto-Encoder for unsupervised root cause detection.

This model learns what "normal" system graphs look like during healthy operation.
When a failure occurs, it identifies root causes by reconstruction error - nodes
that cannot be reconstructed are anomalous.

NO LABELS NEEDED - Zero-shot unsupervised learning.
"""

import torch
import torch.nn as nn
import torch.nn.functional as F
from torch_geometric.nn import GCNConv
from torch_geometric.data import Data
from typing import List, Tuple, Dict
import numpy as np


class GraphAutoEncoder(nn.Module):
    """
    Graph Auto-Encoder for learning normal system topology patterns.

    Architecture:
        Encoder: Graph → Latent (compressed representation)
        Decoder: Latent → Reconstructed Graph

    The model learns to compress and reconstruct HEALTHY graphs.
    During inference, nodes with high reconstruction error are anomalous.
    """

    def __init__(self, input_dim: int = 11, hidden_dim: int = 32, latent_dim: int = 16):
        """
        Initialize the Auto-Encoder.

        Args:
            input_dim: Dimension of node features (11D: type, cpu, state, blocked,
                       block_duration, latency, degree, betweenness, memory, psi_combined, psi_critical)
            hidden_dim: Hidden layer dimension
            latent_dim: Bottleneck (latent) dimension
        """
        super(GraphAutoEncoder, self).__init__()

        # Encoder: Compress graph to latent space
        self.enc1 = GCNConv(input_dim, hidden_dim)
        self.enc2 = GCNConv(hidden_dim, latent_dim)

        # Decoder: Reconstruct graph from latent space
        self.dec1 = GCNConv(latent_dim, hidden_dim)
        self.dec2 = GCNConv(hidden_dim, input_dim)

        self.dropout = nn.Dropout(0.1)

    def encode(self, data: Data) -> torch.Tensor:
        """
        Encode graph to latent representation.

        Args:
            data: PyG Data object with x (node features) and edge_index

        Returns:
            Latent representation tensor
        """
        x, edge_index = data.x, data.edge_index

        # Encoder layer 1
        x = self.enc1(x, edge_index)
        x = F.relu(x)
        x = self.dropout(x)

        # Encoder layer 2 (bottleneck)
        x = self.enc2(x, edge_index)

        return x

    def decode(self, z: torch.Tensor, edge_index: torch.Tensor) -> torch.Tensor:
        """
        Decode latent representation back to node features.

        Args:
            z: Latent representation
            edge_index: Graph edge indices

        Returns:
            Reconstructed node features
        """
        # Decoder layer 1
        x = self.dec1(z, edge_index)
        x = F.relu(x)
        x = self.dropout(x)

        # Decoder layer 2 (reconstruction)
        x = self.dec2(x, edge_index)

        return x

    def forward(self, data: Data) -> torch.Tensor:
        """
        Forward pass: Encode then Decode.

        Args:
            data: PyG Data object

        Returns:
            Reconstructed node features
        """
        z = self.encode(data)
        x_recon = self.decode(z, data.edge_index)
        return x_recon


class RootCauseDetector:
    """
    Detector that identifies root causes via reconstruction error.

    The model tries to reconstruct the system graph. Nodes with high
    reconstruction error (cannot be reconstructed well) are anomalous.
    """

    def __init__(self, model: GraphAutoEncoder, device: str = "cpu"):
        """
        Initialize detector.

        Args:
            model: Trained GraphAutoEncoder
            device: Device to run inference on
        """
        self.model = model
        self.device = device
        self.model.to(device)
        self.model.eval()

    def detect(self, data: Data, node_to_idx: Dict[str, int],
               idx_to_node: Dict[int, str], top_k: int = 5) -> List[Tuple[str, float]]:
        """
        Detect top-k most anomalous nodes via reconstruction error.

        Args:
            data: PyG Data object
            node_to_idx: Mapping from node ID to index
            idx_to_node: Mapping from index to node ID
            top_k: Number of top candidates to return

        Returns:
            List of (node_id, reconstruction_error) tuples sorted by error (descending)
        """
        data = data.to(self.device)

        with torch.no_grad():
            # Reconstruct the graph
            reconstructed = self.model(data)

            # Per-node reconstruction error (MSE)
            errors = torch.mean((data.x - reconstructed) ** 2, dim=1)
            errors = errors.cpu().numpy()

        # Handle single node case
        if errors.ndim == 0:
            errors = np.array([errors])

        # Get top-k nodes with highest reconstruction error
        top_indices = np.argsort(errors)[::-1][:top_k]
        results = [(idx_to_node[idx], float(errors[idx])) for idx in top_indices]

        return results

    def compute_reconstruction_error(self, data: Data) -> Tuple[float, np.ndarray]:
        """
        Compute overall and per-node reconstruction error.

        Args:
            data: PyG Data object

        Returns:
            Tuple of (overall_error, per_node_errors)
        """
        data = data.to(self.device)

        with torch.no_grad():
            reconstructed = self.model(data)
            per_node_errors = torch.mean((data.x - reconstructed) ** 2, dim=1).cpu().numpy()
            overall_error = np.mean(per_node_errors)

        return overall_error, per_node_errors

    @classmethod
    def load(cls, model_path: str, input_dim: int = 8, hidden_dim: int = 32,
             latent_dim: int = 16, device: str = "cpu") -> 'RootCauseDetector':
        """Load a trained model from disk."""
        model = GraphAutoEncoder(input_dim, hidden_dim, latent_dim)
        model.load_state_dict(torch.load(model_path, map_location=device))
        return cls(model, device)

    def save(self, model_path: str):
        """Save model to disk."""
        torch.save(self.model.state_dict(), model_path)


class AutoEncoderTrainer:
    """
    Trainer for the GraphAutoEncoder using unsupervised learning.

    Trains ONLY on healthy graphs - no labels needed!
    """

    def __init__(self, model: GraphAutoEncoder, learning_rate: float = 0.001, device: str = "cpu"):
        """
        Initialize trainer.

        Args:
            model: GraphAutoEncoder to train
            learning_rate: Learning rate for optimizer
            device: Device to train on
        """
        self.model = model
        self.device = device
        self.model.to(device)

        self.optimizer = torch.optim.Adam(model.parameters(), lr=learning_rate)
        self.criterion = nn.MSELoss()  # Reconstruction error

    def train_step(self, data: Data) -> float:
        """
        Perform one training step on a healthy graph.

        Args:
            data: PyG Data object (healthy graph)

        Returns:
            Reconstruction loss
        """
        self.model.train()
        data = data.to(self.device)

        self.optimizer.zero_grad()

        # Forward pass: Reconstruct the graph
        reconstructed = self.model(data)

        # Loss: How well can we reconstruct the original?
        loss = self.criterion(reconstructed, data.x)

        # Backward pass
        loss.backward()
        self.optimizer.step()

        return loss.item()

    def train(self, healthy_graphs: List[Data], epochs: int = 100,
              verbose: bool = True) -> List[float]:
        """
        Train the Auto-Encoder on healthy graphs (unsupervised).

        Args:
            healthy_graphs: List of PyG Data objects representing healthy system states
            epochs: Number of training epochs
            verbose: Print training progress

        Returns:
            List of average losses per epoch
        """
        losses = []

        for epoch in range(epochs):
            epoch_losses = []

            for graph in healthy_graphs:
                loss = self.train_step(graph)
                epoch_losses.append(loss)

            avg_loss = np.mean(epoch_losses)
            losses.append(avg_loss)

            if verbose and (epoch + 1) % 10 == 0:
                print(f"Epoch {epoch + 1}/{epochs}, Reconstruction Loss: {avg_loss:.6f}")

        return losses

    def evaluate(self, test_graphs: List[Data]) -> Dict[str, float]:
        """
        Evaluate reconstruction quality on test graphs.

        Args:
            test_graphs: List of PyG Data objects (can be healthy or failure graphs)

        Returns:
            Dictionary with evaluation metrics
        """
        self.model.eval()

        all_losses = []
        all_node_errors = []

        with torch.no_grad():
            for graph in test_graphs:
                graph = graph.to(self.device)
                reconstructed = self.model(graph)

                # Overall graph reconstruction loss
                loss = self.criterion(reconstructed, graph.x).item()
                all_losses.append(loss)

                # Per-node reconstruction error
                node_errors = torch.mean((graph.x - reconstructed) ** 2, dim=1).cpu().numpy()
                all_node_errors.extend(node_errors)

        return {
            'mean_reconstruction_loss': np.mean(all_losses),
            'std_reconstruction_loss': np.std(all_losses),
            'mean_node_error': np.mean(all_node_errors),
            'std_node_error': np.std(all_node_errors),
            'max_node_error': np.max(all_node_errors),
            'num_graphs': len(test_graphs)
        }

    def compute_anomaly_threshold(self, healthy_graphs: List[Data],
                                   percentile: float = 95.0) -> float:
        """
        Compute anomaly threshold from healthy graphs.

        Nodes with reconstruction error above this threshold are anomalous.

        Args:
            healthy_graphs: List of healthy PyG Data objects
            percentile: Percentile for threshold (default: 95th percentile)

        Returns:
            Threshold value for anomaly detection
        """
        self.model.eval()

        all_errors = []

        with torch.no_grad():
            for graph in healthy_graphs:
                graph = graph.to(self.device)
                reconstructed = self.model(graph)
                errors = torch.mean((graph.x - reconstructed) ** 2, dim=1).cpu().numpy()
                all_errors.extend(errors)

        threshold = np.percentile(all_errors, percentile)
        return threshold


def validate_on_failures(model: GraphAutoEncoder, healthy_graphs: List[Data],
                        failure_graphs: List[Data], failure_node_ids: List[str],
                        all_node_to_idx: List[Dict[str, int]], all_idx_to_node: List[Dict[int, str]],
                        device: str = "cpu") -> Dict[str, any]:
    """
    Validate the Auto-Encoder's ability to detect injected failures.

    Args:
        model: Trained GraphAutoEncoder
        healthy_graphs: List of healthy graphs (for baseline)
        failure_graphs: List of failure graphs (with injected anomalies)
        failure_node_ids: List of ground truth anomalous node IDs (for each failure graph)
        all_node_to_idx: List of node ID to index mappings (one per failure graph)
        all_idx_to_node: List of index to node ID mappings (one per failure graph)
        device: Device to run on

    Returns:
        Dictionary with validation metrics
    """
    detector = RootCauseDetector(model, device)

    # Compute baseline reconstruction error on healthy graphs
    healthy_errors = []
    for graph in healthy_graphs:
        _, errors = detector.compute_reconstruction_error(graph)
        healthy_errors.extend(errors)

    mean_healthy_error = np.mean(healthy_errors)
    std_healthy_error = np.std(healthy_errors)

    # Test on failure graphs
    results = {
        'top1_correct': 0,
        'top3_correct': 0,
        'top5_correct': 0,
        'total_failures': len(failure_graphs),
        'mean_healthy_error': mean_healthy_error,
        'anomaly_ratios': []  # Ratio of anomaly error to healthy error
    }

    for i, (failure_graph, expected_node_id, node_to_idx, idx_to_node) in enumerate(
        zip(failure_graphs, failure_node_ids, all_node_to_idx, all_idx_to_node)):
        predictions = detector.detect(failure_graph, node_to_idx, idx_to_node, top_k=5)

        # Check if expected node is in top-k
        predicted_ids = [node_id for node_id, _ in predictions]

        if len(predicted_ids) > 0 and predicted_ids[0] == expected_node_id:
            results['top1_correct'] += 1
        if expected_node_id in predicted_ids[:3]:
            results['top3_correct'] += 1
        if expected_node_id in predicted_ids[:5]:
            results['top5_correct'] += 1

        # Compute anomaly ratio (how much higher is the anomalous node's error?)
        if predictions:
            top_error = predictions[0][1]
            ratio = top_error / (mean_healthy_error + 1e-8)
            results['anomaly_ratios'].append(ratio)

    # Compute accuracies
    total = results['total_failures']
    results['top1_accuracy'] = results['top1_correct'] / total if total > 0 else 0.0
    results['top3_accuracy'] = results['top3_correct'] / total if total > 0 else 0.0
    results['top5_accuracy'] = results['top5_correct'] / total if total > 0 else 0.0
    results['mean_anomaly_ratio'] = np.mean(results['anomaly_ratios']) if results['anomaly_ratios'] else 0.0

    return results
