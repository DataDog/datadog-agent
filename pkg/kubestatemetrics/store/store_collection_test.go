// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
)

func TestNewDynamicStoreCopiesFactoryRegistry(t *testing.T) {
	registry := NewStoreFactoryRegistry()
	built := 0
	var storeContext context.Context
	registry.Register("core/Pod", "pods", "", ResourceScopeNamespaced, func(ctx context.Context, _ string) cache.Store {
		built++
		storeContext = ctx
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})

	dynamicStore := NewDynamicStore(context.Background(), registry, DynamicStoreConfig{
		ShardCriteria: []string{"namespace"},
		ShardCount:    1,
	})
	lateFactoryBuilt := false
	registry.Register("apps/Deployment", "deployments", "", ResourceScopeNamespaced, func(context.Context, string) cache.Store {
		lateFactoryBuilt = true
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})

	dynamicStore.Add("default")
	dynamicStore.Add("default")
	require.Equal(t, 1, built)
	require.False(t, lateFactoryBuilt)
	require.Len(t, dynamicStore.Snapshot(), 1)

	dynamicStore.Del("default")
	require.ErrorIs(t, storeContext.Err(), context.Canceled)
	require.Empty(t, dynamicStore.Snapshot())
}

func TestDynamicStoreBuildsClusterScopedFactoriesOnce(t *testing.T) {
	registry := NewStoreFactoryRegistry()
	clusterBuilds := 0
	namespacedBuilds := 0
	var clusterNamespace string

	registry.Register("core/Node", "nodes", "", ResourceScopeCluster, func(_ context.Context, namespace string) cache.Store {
		clusterBuilds++
		clusterNamespace = namespace
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})
	registry.Register("core/Pod", "pods", "", ResourceScopeNamespaced, func(context.Context, string) cache.Store {
		namespacedBuilds++
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})

	dynamicStore := NewDynamicStore(context.Background(), registry, DynamicStoreConfig{
		ShardCriteria: []string{"namespace", "resource"},
		ShardCount:    1,
	})
	require.Equal(t, 1, clusterBuilds)
	require.Empty(t, clusterNamespace)
	require.Zero(t, namespacedBuilds)

	dynamicStore.Add("default")
	dynamicStore.Add("default")
	dynamicStore.Add("other")
	require.Equal(t, 1, clusterBuilds)
	require.Equal(t, 2, namespacedBuilds)
	require.Len(t, dynamicStore.Snapshot(), 3)
}

func TestClusterScopedFactoryHasOneOwner(t *testing.T) {
	registry := NewStoreFactoryRegistry()
	builds := 0
	registry.Register("core/Node", "nodes", "", ResourceScopeCluster, func(context.Context, string) cache.Store {
		builds++
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})

	for shardID := 0; shardID < 3; shardID++ {
		NewDynamicStore(context.Background(), registry, DynamicStoreConfig{
			ShardCriteria: []string{"namespace", "resource"},
			ShardCount:    3,
			ShardID:       shardID,
		})
	}

	require.Equal(t, 1, builds)
}

func TestCollectorsForSameResourceStayOnSameShard(t *testing.T) {
	registry := NewStoreFactoryRegistry()
	registry.Register("core/Pod", "pods", "", ResourceScopeNamespaced, func(context.Context, string) cache.Store {
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})
	registry.Register("core/Pod", "pods", "pods_extended", ResourceScopeNamespaced, func(context.Context, string) cache.Store {
		return cache.NewStore(cache.MetaNamespaceKeyFunc)
	})

	criteria := []string{"namespace", "resource"}
	keyBuilder := &dynamicStoreImpl{shardCriteria: criteria}
	standardHashKey := keyBuilder.hashKey("default", factoryKey{groupKind: "core/Pod", apiResource: "pods"})
	extendedHashKey := keyBuilder.hashKey("default", factoryKey{groupKind: "core/Pod", apiResource: "pods", collector: "pods_extended"})
	require.Equal(t, sharding.HashKey("default|core/Pod"), standardHashKey)
	require.Equal(t, standardHashKey, extendedHashKey)

	storeCounts := make([]int, 2)
	for shardID := range storeCounts {
		dynamicStore := NewDynamicStore(context.Background(), registry, DynamicStoreConfig{
			ShardCriteria: criteria,
			ShardCount:    len(storeCounts),
			ShardID:       shardID,
		})
		dynamicStore.Add("default")
		storeCounts[shardID] = len(dynamicStore.Snapshot())
	}

	// Both factories survive registry insertion, but the underlying Pod
	// resource is assigned to only one shard.
	require.ElementsMatch(t, []int{0, 2}, storeCounts)
}

func TestNewDynamicStoreCopiesShardCriteria(t *testing.T) {
	criteria := []string{"namespace"}
	dynamicStore := NewDynamicStore(context.Background(), NewStoreFactoryRegistry(), DynamicStoreConfig{
		ShardCriteria: criteria,
		ShardCount:    2,
	})

	// Mutating the caller's slice must not change how this shard hashes.
	criteria[0] = "resource"

	impl, ok := dynamicStore.(*dynamicStoreImpl)
	require.True(t, ok)
	require.Equal(t, []string{"namespace"}, impl.shardCriteria)
}

func TestColocatedResourcesUseCanonicalCollectorInHashKey(t *testing.T) {
	dynamicStore := &dynamicStoreImpl{
		shardCriteria: []string{"namespace", "resource"},
		resourceColocation: map[string]string{
			"deployments": "deployments",
			"replicasets": "deployments",
		},
	}

	deploymentKey := dynamicStore.hashKey("default", factoryKey{groupKind: "apps/Deployment", apiResource: "deployments"})
	extendedDeploymentKey := dynamicStore.hashKey("default", factoryKey{groupKind: "apps/Deployment", apiResource: "deployments", collector: "deployments_extended"})
	replicaSetKey := dynamicStore.hashKey("default", factoryKey{groupKind: "apps/ReplicaSet", apiResource: "replicasets"})

	require.Equal(t, sharding.HashKey("default|deployments"), deploymentKey)
	require.Equal(t, deploymentKey, extendedDeploymentKey)
	require.Equal(t, deploymentKey, replicaSetKey)
}

func TestNewDynamicStoreCopiesResourceColocation(t *testing.T) {
	resourceColocation := map[string]string{"replicasets": "deployments"}
	dynamicStore := NewDynamicStore(context.Background(), NewStoreFactoryRegistry(), DynamicStoreConfig{
		ShardCriteria:      []string{"resource"},
		ResourceColocation: resourceColocation,
		ShardCount:         2,
	})

	resourceColocation["replicasets"] = "pods"

	impl, ok := dynamicStore.(*dynamicStoreImpl)
	require.True(t, ok)
	require.Equal(t, "deployments", impl.resourceColocation["replicasets"])
}
