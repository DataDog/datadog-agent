// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
)

// KSMShardingSnapshot returns the inventory of this check's hash-sharded stores.
// It does not start collectors or read metrics from them.
func (k *KSMCheck) KSMShardingSnapshot() sharding.CheckSnapshot {
	k.stores.mu.Lock()
	dynamicStore := k.stores.dynamic
	cancelled := k.stores.cancelled
	k.stores.mu.Unlock()

	snapshot := sharding.CheckSnapshot{
		CheckID: string(k.ID()),
		State:   "initializing",
		Stores:  []sharding.StoreInfo{},
	}
	if k.instance != nil {
		snapshot.ShardID = k.instance.ShardID
		snapshot.ShardCount = k.instance.ShardCount
		snapshot.ShardCriteria = slices.Clone(k.instance.ShardCriteria)
		if len(snapshot.ShardCriteria) == 0 {
			snapshot.State = "eager"
		}
	}
	if cancelled {
		snapshot.State = "cancelled"
	} else if dynamicStore != nil {
		snapshot.State = "active"
		snapshot.Stores = dynamicStore.Inventory()
	}
	return snapshot
}
