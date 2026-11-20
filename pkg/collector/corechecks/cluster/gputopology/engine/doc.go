// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package engine computes how free GPUs are spread over a scheduler topology
// (Kueue TAS, KAI), for the gpu.scheduling.topology.* metrics.
//
// It has no Kubernetes or workloadmeta dependency on purpose, so it can be
// tested on synthetic clusters. The caller (the gpu_topology Cluster Agent
// check) converts nodes and pods into the input types.
//
// # Nodes
//
// Each node matched by the flavor is classified once, in this precedence
// (per attributed GPU resource; only no_gpu_allocatable can differ between
// resources, e.g. when one vendor's device plugin is down):
//
//  1. unsupported: GPUs v1 doesn't model. MIG is active (any
//     nvidia.com/mig-* capacity, or a nvidia.com/gpu.product label containing
//     -MIG-; nvidia.com/mig.strategy is never used: GPU Feature Discovery
//     sets it to "none" on every node), or GPUs are shared by time-slicing
//     or MPS (nvidia.com/gpu.shared in capacity, or the GFD labels
//     nvidia.com/gpu.replicas > 1, nvidia.com/gpu.sharing-strategy
//     time-slicing/mps, or a -SHARED product suffix).
//  2. missing_labels: lacks a topology level label.
//  3. unavailable: NotReady or cordoned.
//  4. no_gpu_allocatable: allocatable is 0 for the resource (e.g. device
//     plugin down).
//  5. tainted: a NoSchedule/NoExecute taint that is not tolerated. Tolerated
//     means: by the flavor tolerations, by Topology.AssumedTolerations, or
//     keyed on a GPU resource name (ExtendedResourceToleration; GKE taints
//     GPU pools nvidia.com/gpu=present:NoSchedule).
//  6. in_topology.
//
// A node is attributed to the whole-GPU resources (nvidia.com/gpu,
// amd.com/gpu, gpu.intel.com/*) present in its capacity at any value, or
// else to those of its GPU presence labels: when a device plugin dies,
// kubelet removes the resource from capacity after about 5 minutes, and the
// node must stay visible as no_gpu_allocatable. The Intel presence label maps
// to gpu.intel.com/i915 (it can't tell xe from i915). Unsupported nodes keep
// a real resource key (nvidia.com/gpu, or nvidia.com/gpu.shared when the
// device plugin renames shared GPUs) and carry a slicing mode ("mig") or a
// sharing mode ("time_slicing" or "mps"). Nodes with no GPU evidence are
// ignored.
//
// Result.Nodes has a row for every node state and every (resource, GPU
// type) seen, zeros included, plus the unsupported counts per slicing or
// sharing mode. Rows carry the GPU type. It is
// empty when no flavor node has a GPU (e.g. a GPU pool scaled to 0), because
// no attribution is known: the check should then emit
// gpu.scheduling.topology.nodes = 0 for every NodeState with
// gpu_resource:none (GPUResourceNone) and gpu_type:unknown (GPUTypeUnknown),
// so monitors see 0 rather than no data and every nodes series has the same
// tag set. A topology with no levels is invalid (Kueue requires at least one)
// and yields an empty Result.
//
// # Domains
//
// A Topology is an ordered list of node label keys, coarsest first. A domain
// at level k is the tuple of label values for levels 0..k, so "rack-1" in two
// blocks are two racks. If the topology has no kubernetes.io/hostname level,
// an implicit host level keyed on the node name is appended and flagged
// Implicit, as Kueue >= 0.20 does internally.
//
// Everything is computed per (GPU resource, GPU type), so a rack with H100
// and A100 nodes is one rack per type. The GPU type is read from the first
// parseable label of the resource's vendor (see gpuTypeLabels), normalized
// like the gpu_type tag of the node GPU check: for NVIDIA,
// nvidia.com/gpu.product (GFD), then cloud.google.com/gke-accelerator (GKE),
// then karpenter.k8s.aws/instance-gpu-name (EKS Karpenter, when the
// manufacturer label is nvidia); for AMD, amd.com/gpu.product-name (AMD node
// labeller). It is "unknown" otherwise, and always for Intel. Nodes with no
// GPU evidence (CPU-only) are ignored entirely, so they never create an
// "unknown" partition.
//
// Domains contain the labelled nodes attributed to the resource (any state
// but unsupported and missing_labels), but only in_topology nodes contribute
// capacity (= allocatable). Free =
// allocatable - allocated, clamped at 0 per node (overcommit is reported in
// Result.Overcommits for logging). A domain is unavailable when it has no
// in_topology capacity, else fully_occupied when nothing is free, free when
// nothing is allocated, partially_occupied otherwise. Keeping unavailable
// domains stops "% free racks" from going up when racks fail. A domain with
// usable capacity that also contains a node that isn't in_topology or an
// unhealthy GPU is also degraded: a rack with one cordoned host and nothing
// allocated is free and degraded. Each domain reports its free, allocated and
// unschedulable GPUs and an ID (its label values joined by "/") for a
// per-domain drill-down.
//
// Per level, the engine reports domains per state, GPUs by GPU state and
// domain state, the most free GPUs in one domain, and, per pod size s in
// PodSizes up to the largest host capacity of the resource (in any node
// state, so the reported sizes don't change with node health), the most pods
// of s GPUs that fit in one domain: slots(domain, s) = sum over its hosts of
// floor(free/s), as Kueue counts. Per resource, it reports the GPUs the
// scheduler can't use, by reason (UnschedulableReason, emitted by the check
// under the unschedulable_reason tag): unhealthy (capacity - allocatable on
// in_topology nodes), or the capacity of unavailable, tainted,
// no_gpu_allocatable and missing_labels nodes.
package engine
