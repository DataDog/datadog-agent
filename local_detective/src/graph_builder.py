"""Graph construction from system events."""

from typing import List, Dict, Tuple
from datetime import datetime, timedelta
import networkx as nx
import torch
from torch_geometric.data import Data
import numpy as np

from .events import SystemEvent


class RollingGraphBuilder:
    """Builds and maintains a rolling window graph of system entities."""

    def __init__(self, window_seconds: int = 60):
        """
        Initialize graph builder.

        Args:
            window_seconds: Size of rolling window in seconds
        """
        self.window_seconds = window_seconds
        self.graph = nx.DiGraph()
        self.events: List[SystemEvent] = []
        self.node_to_idx: Dict[str, int] = {}
        self.idx_to_node: Dict[int, str] = {}

    def add_event(self, event: SystemEvent):
        """Add an event to the graph and maintain rolling window."""
        self.events.append(event)

        # Remove old events outside the window
        cutoff_time = event.timestamp - timedelta(seconds=self.window_seconds)
        self.events = [e for e in self.events if e.timestamp >= cutoff_time]

        # Rebuild graph from current window
        self._rebuild_graph()

    def _rebuild_graph(self):
        """Rebuild the graph from current event window."""
        self.graph.clear()
        self.node_to_idx.clear()
        self.idx_to_node.clear()

        # Store PSI data for feature extraction
        self.psi_data = {}

        for event in self.events:
            if event.event_type == "process":
                self._add_process_event(event)
            elif event.event_type == "file_op":
                self._add_file_event(event)
            elif event.event_type == "network":
                self._add_network_event(event)
            elif event.event_type == "resource":
                self._add_resource_event(event)
            elif event.event_type == "system":
                self._add_system_event(event)

    def _get_or_create_node(self, node_id: str, node_type: str, **attrs) -> str:
        """Get or create a node with the given ID and attributes."""
        if node_id not in self.graph:
            self.graph.add_node(node_id, node_type=node_type, **attrs)

            # Maintain node index mapping
            idx = len(self.node_to_idx)
            self.node_to_idx[node_id] = idx
            self.idx_to_node[idx] = node_id

        return node_id

    def _add_process_event(self, event: SystemEvent):
        """Add process event to graph."""
        proc_id = f"proc_{event.pid}"
        self._get_or_create_node(
            proc_id,
            node_type="process",
            pid=event.pid,
            cmd=event.cmd or "unknown",
            cpu_pct=event.cpu_pct or 0.0,
            state=event.state or "S",
            memory_rss_mb=event.memory_rss_mb or 0.0
        )

        # Add parent relationship if ppid exists
        if event.ppid and event.ppid != event.pid:
            parent_id = f"proc_{event.ppid}"
            self._get_or_create_node(
                parent_id,
                node_type="process",
                pid=event.ppid,
                cmd="parent",
                cpu_pct=0.0,
                state="S"
            )
            self.graph.add_edge(parent_id, proc_id, edge_type="parent", weight=1.0)

    def _add_file_event(self, event: SystemEvent):
        """Add file operation event to graph."""
        if not event.pid or not event.file_path:
            return

        proc_id = f"proc_{event.pid}"
        file_id = f"file_{event.file_path}"

        # Ensure process node exists
        self._get_or_create_node(
            proc_id,
            node_type="process",
            pid=event.pid,
            cmd="unknown",
            cpu_pct=0.0,
            state="S"
        )

        # Create file node
        self._get_or_create_node(
            file_id,
            node_type="file",
            path=event.file_path,
            blocked_duration_ms=event.blocked_duration_ms or 0.0
        )

        # Add edge with weight based on blocked duration
        weight = 1.0
        if event.blocked_duration_ms:
            weight = min(10.0, event.blocked_duration_ms / 100.0)  # Scale to 0-10

        self.graph.add_edge(
            proc_id,
            file_id,
            edge_type=f"file_{event.operation}",
            weight=weight,
            blocked_ms=event.blocked_duration_ms or 0.0
        )

    def _add_network_event(self, event: SystemEvent):
        """Add network event to graph."""
        if not event.pid:
            return

        proc_id = f"proc_{event.pid}"

        # Ensure process node exists
        self._get_or_create_node(
            proc_id,
            node_type="process",
            pid=event.pid,
            cmd="unknown",
            cpu_pct=0.0,
            state="S"
        )

        # Create network endpoint node
        if event.dst_ip and event.port:
            net_id = f"net_{event.dst_ip}:{event.port}"
            self._get_or_create_node(
                net_id,
                node_type="network",
                endpoint=f"{event.dst_ip}:{event.port}",
                latency_ms=event.latency_ms or 0.0
            )

            # Add edge with weight based on latency
            weight = 1.0
            if event.latency_ms:
                # Latency > 1000ms = high weight
                weight = min(10.0, event.latency_ms / 1000.0)

            self.graph.add_edge(
                proc_id,
                net_id,
                edge_type="network_call",
                weight=weight,
                latency_ms=event.latency_ms or 0.0
            )

    def _add_resource_event(self, event: SystemEvent):
        """Add resource event to graph."""
        # Resource events are system-wide, create a system node
        sys_id = "system"
        self._get_or_create_node(
            sys_id,
            node_type="system"
        )

        # If tied to a process, add edge
        if event.pid:
            proc_id = f"proc_{event.pid}"
            self._get_or_create_node(
                proc_id,
                node_type="process",
                pid=event.pid,
                cmd="unknown",
                cpu_pct=0.0,
                state="S"
            )
            self.graph.add_edge(proc_id, sys_id, edge_type="resource_usage", weight=5.0)

    def _add_system_event(self, event: SystemEvent):
        """Add system-level PSI (Pressure Stall Information) event to graph."""
        # Store PSI data globally for use in feature extraction
        self.psi_data = {
            'cpu_some': event.psi_cpu_some_avg10 or 0.0,
            'memory_some': event.psi_memory_some_avg10 or 0.0,
            'memory_full': event.psi_memory_full_avg10 or 0.0,
            'io_some': event.psi_io_some_avg10 or 0.0,
            'io_full': event.psi_io_full_avg10 or 0.0
        }

        # Create a system node to represent system-wide state
        sys_id = "system"
        self._get_or_create_node(
            sys_id,
            node_type="system",
            psi_cpu_some=self.psi_data['cpu_some'],
            psi_memory_some=self.psi_data['memory_some'],
            psi_memory_full=self.psi_data['memory_full'],
            psi_io_some=self.psi_data['io_some'],
            psi_io_full=self.psi_data['io_full']
        )

        # Connect all process nodes to system node
        # (processes are affected by system-wide pressure)
        for node_id in list(self.graph.nodes):
            node_data = self.graph.nodes[node_id]
            if node_data.get('node_type') == 'process':
                # Edge weight based on combined pressure
                psi_combined = (
                    self.psi_data['cpu_some'] +
                    self.psi_data['memory_some'] +
                    self.psi_data['io_some']
                ) / 3.0
                weight = max(1.0, psi_combined / 10.0)  # Normalize
                self.graph.add_edge(node_id, sys_id, edge_type="system_pressure", weight=weight)

    def to_pyg_data(self) -> Data:
        """
        Convert NetworkX graph to PyTorch Geometric Data object.

        Returns:
            PyG Data object with node features and edge information
        """
        if len(self.graph.nodes) == 0:
            # Empty graph
            return Data(
                x=torch.zeros((0, 8)),
                edge_index=torch.zeros((2, 0), dtype=torch.long),
                edge_attr=torch.zeros((0, 1))
            )

        # Extract node features
        node_features = []
        for node_id in sorted(self.node_to_idx.keys(), key=lambda x: self.node_to_idx[x]):
            node_data = self.graph.nodes[node_id]
            features = self._extract_node_features(node_id, node_data)
            node_features.append(features)

        x = torch.tensor(node_features, dtype=torch.float)

        # Extract edges
        edge_index = []
        edge_weights = []

        for src, dst, edge_data in self.graph.edges(data=True):
            src_idx = self.node_to_idx[src]
            dst_idx = self.node_to_idx[dst]
            edge_index.append([src_idx, dst_idx])
            edge_weights.append(edge_data.get('weight', 1.0))

        if edge_index:
            edge_index = torch.tensor(edge_index, dtype=torch.long).t().contiguous()
            edge_attr = torch.tensor(edge_weights, dtype=torch.float).unsqueeze(1)
        else:
            edge_index = torch.zeros((2, 0), dtype=torch.long)
            edge_attr = torch.zeros((0, 1))

        # No ground truth labels in unsupervised approach
        # y is not used during training (reconstruction-based)
        y = torch.zeros(len(self.node_to_idx))

        return Data(x=x, edge_index=edge_index, edge_attr=edge_attr, y=y)

    def _extract_node_features(self, node_id: str, node_data: dict) -> List[float]:
        """
        Extract feature vector from node.

        Feature vector (11 dimensions):
        - [0]: node_type (0=process, 1=file, 2=network, 3=system)
        - [1]: cpu_pct (normalized 0-1)
        - [2]: state encoding (0=S, 1=R, 2=D, 3=Z)
        - [3]: has_blocked_operation (0 or 1)
        - [4]: blocked_duration_ms (normalized, log scale)
        - [5]: latency_ms (normalized, log scale)
        - [6]: degree centrality
        - [7]: betweenness centrality (simplified)
        - [8]: memory_rss_mb (normalized, log scale 0-1000MB → 0-1)
        - [9]: psi_combined (weighted avg of CPU + memory + I/O pressure)
        - [10]: psi_critical (max of full pressures: memory_full, io_full)
        """
        node_type = node_data.get('node_type', 'process')
        node_type_enc = {'process': 0.0, 'file': 1.0, 'network': 2.0, 'system': 3.0}.get(node_type, 0.0)

        cpu_pct = min(1.0, node_data.get('cpu_pct', 0.0) / 100.0)

        state = node_data.get('state', 'S')
        state_enc = {'S': 0.0, 'R': 1.0, 'D': 2.0, 'Z': 3.0}.get(state, 0.0) / 3.0

        # Check for blocked operations or high latency in edges
        has_blocked = 0.0
        max_blocked_duration = 0.0
        max_latency = 0.0

        for _, _, edge_data in self.graph.edges(node_id, data=True):
            blocked_ms = edge_data.get('blocked_ms', 0)
            if blocked_ms > 0:
                has_blocked = 1.0
                max_blocked_duration = max(max_blocked_duration, blocked_ms)

            latency_ms = edge_data.get('latency_ms', 0)
            if latency_ms > 0:
                max_latency = max(max_latency, latency_ms)

        # Also check node-level metrics
        if node_type == 'file':
            node_blocked = node_data.get('blocked_duration_ms', 0.0)
            if node_blocked > 0:
                has_blocked = 1.0
                max_blocked_duration = max(max_blocked_duration, node_blocked)

        if node_type == 'network':
            node_latency = node_data.get('latency_ms', 0.0)
            if node_latency > 0:
                max_latency = max(max_latency, node_latency)

        # Normalize blocked duration (log scale): 0-1000ms → 0-1
        blocked_norm = min(1.0, (max_blocked_duration / 1000.0)) if max_blocked_duration > 0 else 0.0

        # Normalize latency (log scale): 0-10000ms → 0-1
        latency_norm = min(1.0, (max_latency / 10000.0)) if max_latency > 0 else 0.0

        # Graph centrality metrics
        degree = self.graph.degree(node_id)
        degree_cent = min(1.0, degree / 10.0)  # Normalize

        # Simplified betweenness (in-degree / out-degree ratio)
        in_deg = self.graph.in_degree(node_id)
        out_deg = self.graph.out_degree(node_id)
        between = (in_deg + 1) / (out_deg + in_deg + 2)  # Avoid div by zero

        # Memory RSS (normalized, log scale): 0-1000MB → 0-1
        memory_rss_mb = node_data.get('memory_rss_mb', 0.0)
        memory_norm = min(1.0, memory_rss_mb / 1000.0) if memory_rss_mb > 0 else 0.0

        # PSI (Pressure Stall Information) - system-wide metrics
        # Get PSI data from stored system event (if available)
        psi_data = getattr(self, 'psi_data', {})

        # PSI combined: weighted average of some pressures
        # Higher weight on memory (most critical for stability)
        psi_combined = (
            psi_data.get('cpu_some', 0.0) * 0.3 +
            psi_data.get('memory_some', 0.0) * 0.4 +
            psi_data.get('io_some', 0.0) * 0.3
        ) / 100.0  # Normalize to 0-1

        # PSI critical: max of "full" pressures (all tasks blocked)
        # These are severe conditions indicating imminent failure
        psi_critical = max(
            psi_data.get('memory_full', 0.0),
            psi_data.get('io_full', 0.0)
        ) / 100.0  # Normalize to 0-1

        return [
            node_type_enc,      # [0]
            cpu_pct,            # [1]
            state_enc,          # [2]
            has_blocked,        # [3]
            blocked_norm,       # [4]
            latency_norm,       # [5]
            degree_cent,        # [6]
            between,            # [7]
            memory_norm,        # [8]
            psi_combined,       # [9]
            psi_critical        # [10]
        ]

    def get_symptom_nodes(self) -> List[str]:
        """
        Identify symptom nodes based on metrics (high latency, blocked operations).

        In unsupervised approach, symptoms are detected by metric thresholds.
        """
        symptom_nodes = []
        for node_id, node_data in self.graph.nodes(data=True):
            # Check for high latency or blocked operations
            for _, _, edge_data in self.graph.edges(node_id, data=True):
                if edge_data.get('latency_ms', 0) > 1000 or edge_data.get('blocked_ms', 0) > 100:
                    symptom_nodes.append(node_id)
                    break
        return symptom_nodes

    def get_root_cause_nodes(self) -> List[str]:
        """
        No ground truth in unsupervised approach.

        This method is deprecated - use model predictions instead.
        """
        return []
