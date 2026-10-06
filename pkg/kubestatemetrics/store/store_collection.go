// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type BuildStoreFunc = func(ctx context.Context, ns string) cache.Store

// ResourceScope describes whether a Kubernetes resource is namespaced or
// cluster-scoped.
type ResourceScope uint8

const (
	// ResourceScopeUnknown indicates that discovery did not determine the scope.
	ResourceScopeUnknown ResourceScope = iota
	// ResourceScopeNamespaced indicates that the resource exists within a namespace.
	ResourceScopeNamespaced
	// ResourceScopeCluster indicates that the resource exists once per cluster.
	ResourceScopeCluster
)

type storeFactory struct {
	build BuildStoreFunc
	scope ResourceScope
}

// factoryKey distinguishes collectors that watch the same Kubernetes
// GroupKind. apiResource is used for colocation lookups, while collector
// keeps factories such as pods and pods_extended from replacing each other.
type factoryKey struct {
	groupKind   string
	apiResource string
	collector   string
}

func (k factoryKey) logName() string {
	if k.collector == "" {
		return k.groupKind
	}
	return k.groupKind + "[collector=" + k.collector + "]"
}

// FactoryRegistry collects KSM store BuildStoreFuncs before constructing a DynamicStore.
type FactoryRegistry struct {
	factories map[factoryKey]storeFactory
}

// NewStoreFactoryRegistry returns an empty store factory registry.
func NewStoreFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{
		factories: make(map[factoryKey]storeFactory),
	}
}

// Register adds a factory to the registry. Collector may be empty for the
// standard collector, or identify a custom collector for the same GroupKind.
func (r *FactoryRegistry) Register(groupKind, apiResource, collector string, scope ResourceScope, build BuildStoreFunc) {
	r.factories[factoryKey{groupKind: groupKind, apiResource: apiResource, collector: collector}] = storeFactory{
		build: build,
		scope: scope,
	}
}

// DynamicStoreConfig describes the ownership rules for one dynamic store.
type DynamicStoreConfig struct {
	ShardCriteria      []string
	ResourceColocation map[string]string
	ShardCount         int
	ShardID            int
}

// NewDynamicStore snapshots its factory and sharding configuration. Copies keep
// later caller mutations from changing which resources this shard owns.
func NewDynamicStore(ctx context.Context, registry *FactoryRegistry, config DynamicStoreConfig) DynamicStore {
	factories := make(map[factoryKey]storeFactory)
	if registry == nil {
		log.Error("cannot construct dynamic store from a nil factory registry")
	} else {
		factories = make(map[factoryKey]storeFactory, len(registry.factories))
		maps.Copy(factories, registry.factories)
	}

	dynamicStore := &dynamicStoreImpl{
		ctx:                ctx,
		factories:          factories,
		stores:             make(map[string]map[factoryKey]storeAndCancelPair),
		shardCriteria:      slices.Clone(config.ShardCriteria),
		resourceColocation: maps.Clone(config.ResourceColocation),
		shardCount:         config.ShardCount,
		shardID:            config.ShardID,
	}
	// Cluster resources use the empty namespace and are built once by their owning shard.
	dynamicStore.add("", ResourceScopeCluster)
	return dynamicStore
}

type DynamicStore interface {
	Add(namespace string)
	Del(namespace string)
	Snapshot() []cache.Store
	Inventory() []sharding.StoreInfo
}

type storeAndCancelPair struct {
	store  cache.Store
	cancel context.CancelFunc
}

type dynamicStoreImpl struct {
	ctx       context.Context
	factories map[factoryKey]storeFactory

	mu sync.Mutex

	// namespace -> factory key -> store and cancel pair
	// [example-ns][{groupKind: core/Pod, collector: pods_extended}]{store, cancel}
	stores             map[string]map[factoryKey]storeAndCancelPair
	shardCriteria      []string
	resourceColocation map[string]string
	shardCount         int
	shardID            int
}

func (d *dynamicStoreImpl) Add(ns string) {
	d.add(ns, ResourceScopeNamespaced)
}

func (d *dynamicStoreImpl) hashKey(ns string, key factoryKey) sharding.HashKey {
	hashResource := key.groupKind
	if colocatedResource, found := d.resourceColocation[key.apiResource]; found {
		hashResource = colocatedResource
	}
	return sharding.NewHashKey(d.shardCriteria, ns, hashResource)
}

func (d *dynamicStoreImpl) add(ns string, scope ResourceScope) {
	// locking so that if Snapshot (called in KSMCheck.Run) is called
	// during an active Add (called by a goroutine running a namespace informer)
	// the information is complete
	d.mu.Lock()
	defer d.mu.Unlock()

	// loop over the resources and their respective builder functions collected
	// by BuildStoreFactoryRegistry
	for key, factory := range d.factories {
		// Namespace events must not create a cluster-wide reflector per namespace.
		if factory.scope != scope {
			continue
		}

		// determine shard ownership, if this shard is not responsible then short-circuit
		shard := sharding.ShardResponsibleForKey(d.shardCount, d.hashKey(ns, key))
		if shard != d.shardID {
			continue
		}

		// entry for namespace doesn't exist yet
		if _, ok := d.stores[ns]; !ok {
			d.stores[ns] = make(map[factoryKey]storeAndCancelPair)
		}

		// entry for this resource collector doesn't exist yet
		if _, ok := d.stores[ns][key]; !ok {
			ctx, cancel := context.WithCancel(d.ctx)
			d.stores[ns][key] = storeAndCancelPair{
				store:  factory.build(ctx, ns),
				cancel: cancel,
			}
		}
	}
}

func (d *dynamicStoreImpl) Del(ns string) {
	d.mu.Lock()
	pairs := d.stores[ns]
	delete(d.stores, ns)
	d.mu.Unlock()

	// cancel all store reflectors
	for key, pair := range pairs {
		log.Debugf("stopping reflector for %s/%s", ns, key.logName())
		pair.cancel()
	}
}

// Snapshot returns the stores active when the snapshot is taken. The returned
// slice is independent of later namespace additions and deletions.
func (d *dynamicStoreImpl) Snapshot() []cache.Store {
	d.mu.Lock()
	defer d.mu.Unlock()

	stores := make([]cache.Store, 0)
	for _, resources := range d.stores {
		for _, pair := range resources {
			stores = append(stores, pair.store)
		}
	}
	return stores
}

// Inventory snapshots store identities and cached object counts. A store may
// exist before its initial list succeeds; object counts do not establish watch health.
func (d *dynamicStoreImpl) Inventory() []sharding.StoreInfo {
	d.mu.Lock()

	stores := make([]sharding.StoreInfo, 0)
	for namespace, resources := range d.stores {
		for key, pair := range resources {
			hashKey := d.hashKey(namespace, key)
			info := sharding.StoreInfo{
				Namespace:   namespace,
				GroupKind:   key.groupKind,
				APIResource: key.apiResource,
				Collector:   key.collector,
				HashKey:     hashKey,
				OwnerShard:  sharding.ShardResponsibleForKey(d.shardCount, hashKey),
			}
			if counter, ok := pair.store.(interface{ ObjectCount() int }); ok {
				count := counter.ObjectCount()
				info.Objects = &count
			}
			stores = append(stores, info)
		}
	}
	d.mu.Unlock()
	slices.SortFunc(stores, func(a, b sharding.StoreInfo) int {
		if a.Namespace != b.Namespace {
			return strings.Compare(a.Namespace, b.Namespace)
		}
		if a.GroupKind != b.GroupKind {
			return strings.Compare(a.GroupKind, b.GroupKind)
		}
		if a.APIResource != b.APIResource {
			return strings.Compare(a.APIResource, b.APIResource)
		}
		return strings.Compare(a.Collector, b.Collector)
	})
	return stores
}
