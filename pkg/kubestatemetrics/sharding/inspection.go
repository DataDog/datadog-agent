// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharding

// StoreInfo describes an existing store, without copying its objects or metrics.
// An empty Namespace denotes a cluster-scoped resource.
type StoreInfo struct {
	Namespace   string  `json:"namespace"`
	GroupKind   string  `json:"group_kind"`
	APIResource string  `json:"api_resource"`
	Collector   string  `json:"collector"`
	HashKey     HashKey `json:"hash_key"`
	OwnerShard  int     `json:"owner_shard"`
	Objects     *int    `json:"objects,omitempty"`
}

// CheckSnapshot describes one KSM instance's hash-sharded store inventory.
// State distinguishes initialization and eager collection from an empty shard.
type CheckSnapshot struct {
	CheckID       string      `json:"check_id"`
	State         string      `json:"state"`
	ShardID       int         `json:"shard_id"`
	ShardCount    int         `json:"shard_count"`
	ShardCriteria []string    `json:"shard_criteria"`
	Stores        []StoreInfo `json:"stores"`
}

// Inspector is an optional check interface used by the local Agent API.
type Inspector interface {
	KSMShardingSnapshot() CheckSnapshot
}
