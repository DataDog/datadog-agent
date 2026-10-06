// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

func TestShardingSnapshotDistinguishesInitializationFromEmptyShard(t *testing.T) {
	k := &KSMCheck{instance: &KSMConfig{ShardCriteria: []string{"namespace"}, ShardCount: 3, ShardID: 2}}
	snapshot := k.KSMShardingSnapshot()
	require.Equal(t, "initializing", snapshot.State)
	require.Empty(t, snapshot.Stores)
	snapshot.ShardCriteria[0] = "resource"
	require.Equal(t, []string{"namespace"}, k.instance.ShardCriteria)

	dynamicStore := ksmstore.NewDynamicStore(context.Background(), ksmstore.NewStoreFactoryRegistry(), ksmstore.DynamicStoreConfig{ShardCriteria: k.instance.ShardCriteria, ShardCount: 3, ShardID: 2})
	require.NoError(t, k.replaceStores(nil, dynamicStore, nil, func() {}))
	snapshot = k.KSMShardingSnapshot()
	require.Equal(t, "active", snapshot.State)
	require.Empty(t, snapshot.Stores)
	require.Equal(t, 2, snapshot.ShardID)
	k.Cancel()
	require.Equal(t, "cancelled", k.KSMShardingSnapshot().State)
}

func TestShardingSnapshotReportsEagerCollection(t *testing.T) {
	k := &KSMCheck{instance: &KSMConfig{}}
	require.Equal(t, "eager", k.KSMShardingSnapshot().State)
}

var _ sharding.Inspector = (*KSMCheck)(nil)
