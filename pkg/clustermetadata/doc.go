// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustermetadata defines the query contract for the shared cluster
// workload metadata cache.
//
// To serve cluster metadata from multiple DCA replicas, each replica runs a
// clustermetadata.Store implementation that can handle queries from:
//   - node agents
//   - dogstatsd pools
//   - APM libraries
//
// Note: leader election is orthogonal to this; any mutating work like the admission controller,
// or autoscaling still is gated by leader election.
//
// Each DCA watches a slightly different subset of the cluster and thus the set of queries it handles is slightly different too.
// In order to not replicate the pod watch N times across all DCA replicas, we shard the pod watch according to node. So all pods on node X
// are watched by the same DCA (not necessarily on node X, of course). OTOH, all other resources (deployments, services, etc.) are watched by all DCA replicas.
package clustermetadata
