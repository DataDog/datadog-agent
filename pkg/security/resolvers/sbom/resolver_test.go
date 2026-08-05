// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// TestRefreshScanResetsStateForRescan checks that refreshing a workload clears
// its cached SBOM data, resets the SBOM to the pending state, and re-queues it
// for a scan. The state reset matters because analyzeWorkload drops any SBOM
// not in the pending state: a workload is left computed by its initial scan, so
// without the reset the refresh re-scan is silently discarded and the runtime
// properties are never recomputed.
func TestRefreshScanResetsStateForRescan(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := &Resolver{
		dataCache: dataCache,
		scanChan:  make(chan *SBOM, 10),
	}

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.state.Store(computedState)
	dataCache.Add("image:tag", &Data{})

	r.refreshScan(sbom)

	if got := sbom.state.Load(); got != pendingState {
		t.Errorf("state = %d, want pendingState (%d)", got, pendingState)
	}
	if _, ok := dataCache.Get("image:tag"); ok {
		t.Errorf("cached SBOM data was not invalidated")
	}
	select {
	case queued := <-r.scanChan:
		if queued != sbom {
			t.Errorf("queued unexpected SBOM for re-scan")
		}
	default:
		t.Errorf("workload was not re-queued for a scan")
	}
}

func newPendingFileEventsResolver() *Resolver {
	return &Resolver{
		pendingFileEvents: make(map[containerutils.ContainerID][]pendingFileEvent),
	}
}

// TestDeleteReleasesPendingFileEventsWithoutSBOM checks that a container leaving
// before an SBOM entry was created for it still releases its queued file accesses.
// Accesses are queued from the moment a container ID resolves, which is well before
// the workload selector that creates the entry.
func TestDeleteReleasesPendingFileEventsWithoutSBOM(t *testing.T) {
	sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](2, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := newPendingFileEventsResolver()
	r.sboms = sboms

	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)
	r.Delete("container-id")

	if len(r.pendingFileEvents) != 0 {
		t.Errorf("queued file accesses were not released")
	}
}

// TestEvictedSBOMReleasesPendingFileEvents checks that the file accesses queued for
// a workload are released when its SBOM leaves the cache, whether it is removed
// explicitly or evicted to make room.
func TestEvictedSBOMReleasesPendingFileEvents(t *testing.T) {
	r := newPendingFileEventsResolver()
	sboms, err := simplelru.NewLRU(1, r.onSBOMEvicted)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r.sboms = sboms

	sboms.Add("evicted-container-id", NewSBOM("evicted-container-id", nil, "image:tag"))
	r.queuePendingFileEvent("evicted-container-id", "/usr/bin/su", 04755, 0)

	sboms.Add("container-id", NewSBOM("container-id", nil, "image:tag"))
	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)

	if _, ok := r.pendingFileEvents["evicted-container-id"]; ok {
		t.Errorf("queued file accesses of the evicted SBOM were not released")
	}

	r.Delete("container-id")

	if len(r.pendingFileEvents) != 0 {
		t.Errorf("queued file accesses of the removed SBOM were not released")
	}
}

// TestAnalyzeWorkloadReusesCachedDataAsComputed checks that a workload whose data
// landed in the cache while it was queued for a scan still ends up computed. Left
// pending, every package lookup for that container queues instead of resolving and
// its queued accesses are never applied — which is the fate of every replica of an
// image but the one that gets scanned.
func TestAnalyzeWorkloadReusesCachedDataAsComputed(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := &Resolver{
		// long enough that the forwarding debouncer cannot fire during the test
		cfg:               &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Hour},
		dataCache:         dataCache,
		pendingFileEvents: make(map[containerutils.ContainerID][]pendingFileEvent),
	}

	dataCache.Add("image:tag", newData([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "shadow-utils"},
		InstalledFiles: []string{"/usr/bin/su"},
	}}, false))

	sbom := NewSBOM("container-id", nil, "image:tag")
	t.Cleanup(sbom.stop)
	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)

	if err := r.analyzeWorkload(sbom); err != nil {
		t.Fatalf("analyzeWorkload: %v", err)
	}

	if !sbom.IsComputed() {
		t.Errorf("state = %d, want computedState (%d)", sbom.state.Load(), computedState)
	}
	if len(r.pendingFileEvents) != 0 {
		t.Errorf("queued file accesses were not drained")
	}
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() {
		t.Errorf("package = %+v, want last access set from the queued accesses", pkg)
	}
}

// TestAnalyzeWorkloadSkipsStoppedWorkload checks that a workload stopped while it was
// queued for a scan is left alone. Reviving it as computed restarts a forwarding
// debouncer that can never be stopped again, since a stopped workload is no longer
// reachable from the resolver, and reports an SBOM for a container that is gone.
func TestAnalyzeWorkloadSkipsStoppedWorkload(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := &Resolver{
		cfg:               &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Hour},
		dataCache:         dataCache,
		pendingFileEvents: make(map[containerutils.ContainerID][]pendingFileEvent),
	}

	dataCache.Add("image:tag", newData([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "shadow-utils"},
		InstalledFiles: []string{"/usr/bin/su"},
	}}, false))

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.stop()
	t.Cleanup(sbom.stop)

	if err := r.analyzeWorkload(sbom); err != nil {
		t.Fatalf("analyzeWorkload: %v", err)
	}

	if got := sbom.state.Load(); got != stoppedState {
		t.Errorf("state = %d, want stoppedState (%d)", got, stoppedState)
	}
	if sbom.forwarder != nil {
		t.Errorf("a forwarding debouncer was started for a stopped workload")
	}
}

// TestQueueWorkloadAppliesQueuedAccessesOnCacheHit checks that a workload admitted
// with data already in the cache applies the accesses queued for it. They are queued
// from the moment its container ID resolves, which precedes the workload selector
// that admits it, so a workload going idle right after would otherwise never have
// them applied.
func TestQueueWorkloadAppliesQueuedAccessesOnCacheHit(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := &Resolver{
		dataCache:         dataCache,
		scanChan:          make(chan *SBOM, 1),
		pendingFileEvents: make(map[containerutils.ContainerID][]pendingFileEvent),
		sbomsCacheHit:     atomic.NewUint64(0),
		sbomsCacheMiss:    atomic.NewUint64(0),
	}

	dataCache.Add("image:tag", newData([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "shadow-utils"},
		InstalledFiles: []string{"/usr/bin/su"},
	}}, false))

	sbom := NewSBOM("container-id", nil, "image:tag")
	t.Cleanup(sbom.stop)
	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)

	r.queueWorkload(sbom)

	if !sbom.IsComputed() {
		t.Errorf("state = %d, want computedState (%d)", sbom.state.Load(), computedState)
	}
	if len(r.pendingFileEvents) != 0 {
		t.Errorf("queued file accesses were not applied")
	}
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want last access and both sticky properties set", pkg)
	}
}
