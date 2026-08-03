// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configmapdata holds the allow-list of ConfigMaps whose data is collected in full instead
// of being stripped before the manifest is emitted. The list is delivered over remote config and is
// read by the orchestrator check on every collection tick.
package configmapdata

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// maxEntries bounds how many ConfigMaps a single remote config update can opt in. Every allowed
// ConfigMap uploads its full body on every resource version change, so the list is capped rather
// than trusted.
const maxEntries = 200

// Entry identifies one opted-in ConfigMap. Remote config payloads are org-scoped and
// kube-system/coredns exists in every cluster of an org, so the cluster has to be part of the
// identity. ClusterID is the orch_cluster_id, the UID of the kube-system namespace.
type Entry struct {
	ClusterID string `json:"cluster_id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// payload is the shape of a single DEBUG product config.
type payload struct {
	Version    int     `json:"version"`
	ConfigMaps []Entry `json:"configmaps"`
}

// key identifies a ConfigMap within one cluster. An AllowSet is already narrowed to a cluster, so
// the cluster is not part of the key.
type key struct {
	namespace string
	name      string
}

// AllowSet is an immutable, cluster-scoped view of the allow-list. It is taken once per collection
// tick so that every read within a tick agrees: the resource version and the strip decision are
// read at three different points of the tick, and an update landing between two of them would
// otherwise emit a manifest whose cache token disagrees with its content.
type AllowSet map[key]struct{}

// IsAllowed reports whether the named ConfigMap is opted into full data collection.
func (a AllowSet) IsAllowed(namespace, name string) bool {
	_, ok := a[key{namespace: namespace, name: name}]
	return ok
}

// Store holds the current allow-list as delivered by remote config.
type Store struct {
	mu      sync.RWMutex
	entries []Entry
}

var globalStore = &Store{}

// Get returns the process-wide store.
//
// The store is a singleton because threading it down to the handler would mean touching the check
// factory, CollectorRunConfig, K8sProcessorContext and the !kubeapiserver stub. That is the
// productionization step, not a correctness difference.
func Get() *Store {
	return globalStore
}

// Replace swaps the whole allow-list. Remote config always sends complete configs, so there is no
// incremental update path: a removed entry has to stop being allowed, which is the opt-out.
func (s *Store) Replace(entries []Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = entries
}

// Snapshot returns the entries for one cluster as an immutable set. The caller must not mutate it.
func (s *Store) Snapshot(clusterID string) AllowSet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.entries) == 0 {
		return nil
	}

	set := make(AllowSet, len(s.entries))
	for _, e := range s.entries {
		if e.ClusterID != clusterID {
			continue
		}
		set[key{namespace: e.Namespace, name: e.Name}] = struct{}{}
	}
	return set
}

// Len returns the number of entries currently held, across all clusters.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// SubscribeFunc matches the Subscribe method of both the raw remote config client used by the
// cluster agent and the rcclient component used by the node agent, so that this package does not
// have to import either of them.
type SubscribeFunc func(product string, cb func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))

// Subscribe registers the allow-list handler against the DEBUG product. It must be called before
// the remote config client is started so the product is part of the first poll.
func Subscribe(subscribe SubscribeFunc) {
	subscribe(state.ProductDebug, onUpdate)
}

// onUpdate rebuilds the allow-list from the full set of configs remote config currently holds. An
// empty update means every ConfigMap reverts to metadata-only.
func onUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	paths := make([]string, 0, len(update))
	for path := range update {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var (
		entries []Entry
		seen    = make(map[Entry]struct{})
	)

	for _, path := range paths {
		var p payload
		if err := json.Unmarshal(update[path].Config, &p); err != nil {
			log.Errorf("Could not parse ConfigMap data allow-list from remote config %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		for _, e := range p.ConfigMaps {
			if e.ClusterID == "" || e.Namespace == "" || e.Name == "" {
				log.Warnf("Ignoring incomplete ConfigMap data allow-list entry from remote config %s: %+v", path, e)
				continue
			}
			if _, dup := seen[e]; dup {
				continue
			}
			if len(entries) >= maxEntries {
				log.Warnf("ConfigMap data allow-list capped at %d entries, ignoring the rest", maxEntries)
				break
			}
			seen[e] = struct{}{}
			entries = append(entries, e)
		}

		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	Get().Replace(entries)
	log.Infof("ConfigMap data allow-list updated from remote config: %d entries from %d configs", len(entries), len(update))
}
