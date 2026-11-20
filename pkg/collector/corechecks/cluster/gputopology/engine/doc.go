// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package engine computes how free GPUs are spread over a scheduler topology
// (Kueue TAS, KAI), for the gpu.scheduling.topology.* metrics.
//
// It is pure Go on purpose: no Kubernetes or workloadmeta types, so it can be
// tested on synthetic clusters. The caller (the gpu_topology Cluster Agent
// check) converts nodes and pods into the input types below.
//
// # Model
//
// A Topology is an ordered list of node label keys, coarsest first (for
// example block, rack, host). Nodes are grouped into domains per level:
//
//   - A domain at level k is the tuple of the node's label values for levels
//     0..k, so "rack-1" in two different blocks are two different racks.
//   - A host level is always present. If the topology does not declare
//     kubernetes.io/hostname, an implicit level keyed on the node name is
//     appended as the finest one and flagged Implicit (Kueue does the same).
//
// Everything is computed separately per GPU resource name (nvidia.com/gpu,
// amd.com/gpu), so one vendor never makes another vendor's domain look used.
// A node is in scope for a resource when its capacity for it is > 0. Each
// node in scope gets exactly one state, the first that applies:
//
//  1. unsupported: flagged by the caller (e.g. MIG nodes)
//  2. missing_labels: missing any level label
//  3. no_gpu_allocatable: allocatable is 0
//  4. unavailable: NotReady or cordoned
//  5. tainted: a NoSchedule/NoExecute taint not tolerated by the flavor
//  6. in_topology
//
// Only in_topology nodes contribute. A domain's capacity is the allocatable
// of its nodes; capacity - allocatable is reported as unavailable GPUs, per
// domain and as a per-resource total that also covers no_gpu_allocatable
// nodes (e.g. device plugin down). Free
// = allocatable - allocated, clamped at 0 per node (overcommit is reported in
// Result.Overcommits for logging). A domain is full when nothing is free,
// free when nothing is allocated, partial otherwise.
//
// Per level, the engine reports the number of domains per state, the GPUs by
// GPU state and domain state, the most free GPUs in one domain, and, for each
// pod size s in PodSizes, the most pods of s GPUs that fit in one domain:
// slots(domain, s) = sum over its hosts of floor(free/s), as Kueue counts.
//
// A topology with no levels is invalid (Kueue requires at least one) and
// yields an empty Result.
package engine
