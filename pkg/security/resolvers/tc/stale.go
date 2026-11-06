// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
)

// detachInactiveHook detaches an inactive entry, and returns true when its TCX link was stale. The TCX link of a
// deleted device reports ifindex 0, so a TCX entry whose link still reports one is stale: its device left the namespace
// of the entry. Needs to be called with the lock held.
func (tcr *Resolver) detachInactiveHook(key ProgramKey, entry programEntry, m *manager.Manager) bool {
	var stale bool
	if entry.attachMode == manager.TCAttachModeTCX {
		if ifIndex, err := entry.probe.TCXLinkIfIndex(); err == nil && ifIndex != 0 {
			tcr.staleLinks.Add(1)
			stale = true
		}
	}
	tcr.detachHook(key, entry, m)
	return stale
}

// DetachMovedDevice detaches the TCX entries of a device that moved to the namespace netNS, which only knows its
// ifindex in its previous namespace. The entries of other devices sharing that ifindex stay attached. It returns the
// number of stale TCX links it detached.
func (tcr *Resolver) DetachMovedDevice(ifIndex uint32, netNS uint32, m *manager.Manager) int {
	if tcr.attachMode != manager.TCAttachModeTCX {
		return 0
	}

	tcr.Lock()
	defer tcr.Unlock()

	var staleLinks int
	for key, entry := range tcr.programs {
		if key.IfIndex == ifIndex && key.NetNS != netNS && !entry.probe.IsTCFilterActive() {
			if tcr.detachInactiveHook(key, entry, m) {
				staleLinks++
			}
		}
	}
	return staleLinks
}

// AttachedIfIndexes returns the ifindexes of the devices of a namespace that hold at least one classifier
func (tcr *Resolver) AttachedIfIndexes(netNS uint32) map[uint32]struct{} {
	tcr.RLock()
	defer tcr.RUnlock()

	ifIndexes := make(map[uint32]struct{})
	for key := range tcr.programs {
		if key.NetNS == netNS {
			ifIndexes[key.IfIndex] = struct{}{}
		}
	}
	return ifIndexes
}

// sendStaleLinksStats flushes the count of stale TCX links
func (tcr *Resolver) sendStaleLinksStats(statsdClient statsd.ClientInterface) {
	if val := tcr.staleLinks.Swap(0); val > 0 {
		_ = statsdClient.Count(metrics.MetricTCStaleLinks, val, []string{}, 1.0)
	}
}
