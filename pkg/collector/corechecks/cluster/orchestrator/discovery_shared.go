// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package orchestrator

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
)

// sharedDiscovery is the API discovery of the bundles of a check. A check run queries the API server once, for all
// the bundles that refresh it.
type sharedDiscovery struct {
	collector *discovery.DiscoveryCollector
	// refreshed reports whether the current check run refreshed the discovery, and err is the result.
	refreshed bool
	err       error
	// generation counts the refreshes that added resources. A bundle prepares its collectors again when it changes.
	generation uint64
	// crdChanges counts the CRD changes that the bundles flag. crdChangesAtRefresh is its value before the last refresh.
	crdChanges          atomic.Uint64
	crdChangesAtRefresh uint64
}

func newSharedDiscovery(fetch discovery.ServerGroupsAndResourcesFunc) *sharedDiscovery {
	return &sharedDiscovery{collector: discovery.NewDiscoveryCollector(fetch)}
}

// nextRun makes the next refresh query the API server again.
func (s *sharedDiscovery) nextRun() {
	s.refreshed = false
}

// refresh refreshes the discovery once per check run. It returns the generation of the discovery, and whether no
// CRD changed since the refresh.
func (s *sharedDiscovery) refresh() (generation uint64, current bool, err error) {
	if !s.refreshed {
		s.crdChangesAtRefresh = s.crdChanges.Load()
		var updated bool
		updated, s.err = s.collector.Refresh()
		if updated {
			s.generation++
		}
		s.refreshed = true
	}
	return s.generation, s.crdChanges.Load() == s.crdChangesAtRefresh, s.err
}
