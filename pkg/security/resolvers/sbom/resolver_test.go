// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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

func newPendingFileEvents(t *testing.T) *simplelru.LRU[containerutils.ContainerID, map[string]pendingFileEvent] {
	events, err := simplelru.NewLRU[containerutils.ContainerID, map[string]pendingFileEvent](maxSBOMEntries, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	return events
}

func newPendingFileEventsResolver(t *testing.T) *Resolver {
	return &Resolver{pendingFileEvents: newPendingFileEvents(t)}
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
	r := newPendingFileEventsResolver(t)
	r.sboms = sboms

	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)
	r.Delete("container-id")

	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("queued file accesses were not released")
	}
}

// TestEvictedSBOMReleasesPendingFileEvents checks that the file accesses queued for
// a workload are released when its SBOM leaves the cache, whether it is removed
// explicitly or evicted to make room.
func TestEvictedSBOMReleasesPendingFileEvents(t *testing.T) {
	r := newPendingFileEventsResolver(t)
	sboms, err := simplelru.NewLRU(1, r.onSBOMEvicted)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r.sboms = sboms

	sboms.Add("evicted-container-id", NewSBOM("evicted-container-id", nil, "image:tag"))
	r.queuePendingFileEvent("evicted-container-id", "/usr/bin/su", 04755, 0)

	sboms.Add("container-id", NewSBOM("container-id", nil, "image:tag"))
	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)

	if _, ok := r.pendingFileEvents.Get("evicted-container-id"); ok {
		t.Errorf("queued file accesses of the evicted SBOM were not released")
	}

	r.Delete("container-id")

	if r.pendingFileEvents.Len() != 0 {
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
		pendingFileEvents: newPendingFileEvents(t),
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

	if err := r.analyzeWorkload(sbom); err != nil {
		t.Fatalf("analyzeWorkload: %v", err)
	}

	if !sbom.IsComputed() {
		t.Errorf("state = %d, want computedState (%d)", sbom.state.Load(), computedState)
	}
	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("queued file accesses were not drained")
	}
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() {
		t.Errorf("package = %+v, want last access set from the queued accesses", pkg)
	}
	if got := r.sbomsCacheHit.Load(); got != 1 {
		t.Errorf("cache hits = %d, want 1: the scan avoided while queued was not counted", got)
	}
	if got := r.sbomsCacheMiss.Load(); got != 0 {
		t.Errorf("cache misses = %d, want 0: the workload did not scan", got)
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
		pendingFileEvents: newPendingFileEvents(t),
		sbomsCacheHit:     atomic.NewUint64(0),
		sbomsCacheMiss:    atomic.NewUint64(0),
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
		pendingFileEvents: newPendingFileEvents(t),
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
	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("queued file accesses were not applied")
	}
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want last access and both sticky properties set", pkg)
	}
}

// TestPendingFileEventsAreDeduplicatedPerPath checks that repeated accesses to the
// same file collapse into a single entry with the sticky properties merged. The
// snapshot replay emits one open event per (process, mapped file) pair and runs again
// on every ruleset reload, so the shared libraries mapped by every process of a
// workload would otherwise crowd out the distinct paths worth keeping.
func TestPendingFileEventsAreDeduplicatedPerPath(t *testing.T) {
	r := newPendingFileEventsResolver(t)

	for range 3 {
		r.queuePendingFileEvent("container-id", "/usr/lib/libc.so.6", 0644, 1000)
	}
	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 1000)
	r.queuePendingFileEvent("container-id", "/usr/bin/su", 0755, 0)

	events, _ := r.pendingFileEvents.Get("container-id")
	if len(events) != 2 {
		t.Fatalf("queued %d distinct events, want 2", len(events))
	}
	if event := events["/usr/lib/libc.so.6"]; event.suidBit || event.accessedByRoot {
		t.Errorf("libc event = %+v, want no sticky property set", event)
	}
	if event := events["/usr/bin/su"]; !event.suidBit || !event.accessedByRoot {
		t.Errorf("su event = %+v, want both sticky properties merged", event)
	}
}

// TestPendingFileEventsBoundDistinctPathsPerContainer checks that a container holds
// at most maxPendingFileEvents distinct paths, and that an access to an already
// queued path is still merged once that bound is reached.
func TestPendingFileEventsBoundDistinctPathsPerContainer(t *testing.T) {
	r := newPendingFileEventsResolver(t)

	for i := range maxPendingFileEvents {
		r.queuePendingFileEvent("container-id", fmt.Sprintf("/usr/lib/lib%d.so", i), 0644, 1000)
	}
	r.queuePendingFileEvent("container-id", "/usr/lib/overflow.so", 0644, 1000)
	r.queuePendingFileEvent("container-id", "/usr/lib/lib0.so", 0644, 0)

	events, _ := r.pendingFileEvents.Get("container-id")
	if len(events) != maxPendingFileEvents {
		t.Fatalf("queued %d distinct events, want %d", len(events), maxPendingFileEvents)
	}
	if _, ok := events["/usr/lib/overflow.so"]; ok {
		t.Errorf("path queued past the maximum number of pending events")
	}
	if !events["/usr/lib/lib0.so"].accessedByRoot {
		t.Errorf("access to an already queued path was not merged")
	}
}

// TestProcessPendingFileEventsEnrichesPackages checks that draining the queue applies
// the queued accesses to the packages owning the files and marks the SBOM for
// forwarding.
func TestProcessPendingFileEventsEnrichesPackages(t *testing.T) {
	r := newPendingFileEventsResolver(t)

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.data = newData([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "shadow-utils"},
		InstalledFiles: []string{"/usr/bin/su"},
	}}, false)

	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)
	r.queuePendingFileEvent("container-id", "/usr/bin/not-in-any-package", 0644, 1000)

	r.processPendingFileEvents(sbom)

	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("pending events were not drained")
	}
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want last access and both sticky properties set", pkg)
	}
	if !sbom.invalidated {
		t.Errorf("sbom was not marked for forwarding")
	}
}

// TestSharedDataConcurrentForwardingAndResolve checks that the forwarding snapshot
// (copy of packages) and the package enrichment (LastAccess/SuidBit/AccessedByRoot
// writes) are safe when they run on different SBOMs sharing the same *Data via the
// dataCache. The SBOM lock is per-container, so without the Data-level lock the
// copy and the writes race on the shared packages slice.
func TestSharedDataConcurrentForwardingAndResolve(t *testing.T) {
	data := newData([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "shadow-utils"},
		InstalledFiles: []string{"/usr/bin/su"},
	}}, false)

	sbomA := NewSBOM("container-a", nil, "image:tag")
	sbomA.data = data
	sbomA.state.Store(computedState)

	sbomB := NewSBOM("container-b", nil, "image:tag")
	sbomB.data = data
	sbomB.state.Store(computedState)

	r := newPendingFileEventsResolver(t)

	var wg sync.WaitGroup

	// Writer: simulate ResolvePackage enriching packages on sbomA (writes
	// LastAccess/SuidBit/AccessedByRoot on the shared Data).
	wg.Go(func() {
		for range 2000 {
			r.queuePendingFileEvent("container-a", "/usr/bin/su", 04755, 0)
			sbomA.Lock()
			r.processPendingFileEvents(sbomA)
			sbomA.Unlock()
		}
	})

	// Reader: simulate triggerForwarding.func1 snapshotting packages on sbomB
	// (reads the shared Data via copy()).
	wg.Go(func() {
		for range 2000 {
			sbomB.Lock()
			if sbomB.data != nil && len(sbomB.data.packages) > 0 {
				sbomB.data.mu.RLock()
				packages := make([]sbomtypes.Package, len(sbomB.data.packages))
				copy(packages, sbomB.data.packages)
				sbomB.data.mu.RUnlock()
				_ = packages
			}
			sbomB.Unlock()
		}
	})

	wg.Wait()
}

// hostResolver builds a resolver holding a computed host index over the given
// packages, the state Start leaves behind when the host SBOM is enabled.
func hostResolver(t *testing.T, cfg *config.RuntimeSecurityConfig, report []sbomtypes.PackageWithInstalledFiles) *Resolver {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](2, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}

	r := &Resolver{
		Notifier:              utils.NewNotifier[Event, *sbompkg.ScanResult](),
		cfg:                   cfg,
		dataCache:             dataCache,
		sboms:                 sboms,
		scanChan:              make(chan *SBOM, 10),
		pendingFileEvents:     newPendingFileEvents(t),
		sbomGenerations:       atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
		sbomsCacheHit:         atomic.NewUint64(0),
		sbomsCacheMiss:        atomic.NewUint64(0),
	}
	r.hostSBOM = NewSBOM("", nil, hostWorkloadKey)
	r.hostSBOM.setReport(report)
	r.hostSBOM.state.Store(computedState)
	return r
}

var hostReport = []sbomtypes.PackageWithInstalledFiles{{
	Package:        sbomtypes.Package{Name: "shadow-utils", Version: "4.15.1"},
	InstalledFiles: []string{"/usr/bin/su"},
}}

// TestResolvePackageEnrichesHostPackages checks that a file access from a process
// with no container resolves against the host index and records the usage on the
// owning package. The core agent merges those properties onto the host SBOM it
// scans itself, so an access that stopped at the container check would leave the
// host reported as running nothing.
func TestResolvePackageEnrichesHostPackages(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{
		SBOMResolverEnrichmentInterval: time.Minute,
		SBOMResolverForwardInterval:    time.Hour,
	}, hostReport)
	defer r.hostSBOM.stop()

	pc := &model.ProcessContext{Process: model.Process{Credentials: model.Credentials{UID: 0}}}
	file := &model.FileEvent{
		FileFields:            model.FileFields{Mode: 04755},
		PathnameStr:           "/usr/bin/su",
		IsPathnameStrResolved: true,
	}

	pkg := r.ResolvePackage(pc, file)
	if pkg == nil {
		t.Fatalf("no package resolved for a host file access")
	}
	if pkg.Name != "shadow-utils" {
		t.Errorf("package = %q, want shadow-utils", pkg.Name)
	}
	if pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want last access and both sticky properties set", *pkg)
	}
}

// TestResolvePackageWithoutHostIndex checks that a host file access resolves to
// nothing and queues nothing when the host index is disabled, which is the state
// of an agent collecting container SBOMs alone.
func TestResolvePackageWithoutHostIndex(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{}, hostReport)
	r.hostSBOM = nil

	// The probe reads this to skip resolving host events entirely, which is what
	// keeps a container-only deployment off the host path.
	if r.HostEnabled() {
		t.Errorf("HostEnabled reports the host is indexed while it is disabled")
	}

	pc := &model.ProcessContext{}
	file := &model.FileEvent{
		FileFields:            model.FileFields{Mode: 0755},
		PathnameStr:           "/usr/bin/su",
		IsPathnameStrResolved: true,
	}

	if pkg := r.ResolvePackage(pc, file); pkg != nil {
		t.Errorf("package = %+v, want none", *pkg)
	}
	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("host file access was queued, and nothing would ever drain it")
	}
}

// TestLookupPackageRecordsNothing checks that the lookup used to read package
// metadata for its own sake, such as resolving the version of a systemd unit,
// leaves the runtime usage properties alone. Owning a file is not running it.
func TestLookupPackageRecordsNothing(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{}, hostReport)

	pkg := r.LookupPackage("", "/usr/bin/su")
	if pkg == nil {
		t.Fatalf("no package resolved for /usr/bin/su")
	}
	if pkg.Version != "4.15.1" {
		t.Errorf("version = %q, want 4.15.1", pkg.Version)
	}
	if !pkg.LastAccess.IsZero() || pkg.SuidBit || pkg.AccessedByRoot {
		t.Errorf("package = %+v, want no usage recorded", *pkg)
	}
}

// TestForwardHostSBOMWithoutImageStatus checks that the host report is forwarded
// straight away. The container path waits for the image's Trivy SBOM to leave the
// pending state, and the host has no image to wait for, so sharing that wait would
// hold the report until maxForwardWait expired and then drop it.
func TestForwardHostSBOMWithoutImageStatus(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Millisecond}, hostReport)

	results := make(chan *sbompkg.ScanResult, 1)
	if err := r.RegisterListener(SBOMComputed, func(result *sbompkg.ScanResult) {
		results <- result
	}); err != nil {
		t.Fatalf("RegisterListener: %v", err)
	}

	sbom := r.hostSBOM
	sbom.Lock()
	r.triggerForwarding(sbom)
	sbom.Unlock()
	defer sbom.stop()

	select {
	case result := <-results:
		report, ok := result.Report.(*PackagesReport)
		if !ok {
			t.Fatalf("report type = %T, want *PackagesReport", result.Report)
		}
		if !report.IsHost() {
			t.Errorf("report is not marked as the host's, so it would be merged onto a container image")
		}
		if result.RequestID != "" {
			t.Errorf("RequestID = %q, want empty", result.RequestID)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("host report was not forwarded")
	}
}

// TestDoScanHostUsesHostRoot checks that rescanning the host reads the host root.
// A host process writing the package database fires the bundled refresh_sbom rule
// with no container id, and the host SBOM has no cgroup to take a scan root from.
func TestDoScanHostUsesHostRoot(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{}, hostReport)
	r.hostRoot = t.TempDir()

	scanned := ""
	r.sbomCollector = scanFunc(func(_ context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
		scanned = root
		return hostReport, nil
	})

	report, err := r.doScan(r.hostSBOM)
	if err != nil {
		t.Fatalf("doScan: %v", err)
	}
	if scanned != r.hostRoot {
		t.Errorf("scanned %q, want the host root %q", scanned, r.hostRoot)
	}
	if len(report) != 1 {
		t.Errorf("report has %d packages, want 1", len(report))
	}
}

// TestDropQueuedHostSBOMKeepsIndex checks that a full scan queue leaves the host
// index in place. A container SBOM is dropped from the cache, which releases
// everything indexed by its container ID, while the host lives outside that cache
// and would lose the properties of every package it already knows.
func TestDropQueuedHostSBOMKeepsIndex(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{}, hostReport)
	r.hostSBOM.state.Store(pendingState)

	r.dropQueuedSBOM(r.hostSBOM)

	if !r.hostSBOM.IsComputed() {
		t.Errorf("host SBOM state = %d, want computed", r.hostSBOM.state.Load())
	}
	if r.hostSBOM.data == nil || len(r.hostSBOM.data.packages) != 1 {
		t.Errorf("host index was dropped along with the queued scan")
	}
}

// scanFunc adapts a function to the sbomCollector interface.
type scanFunc func(ctx context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error)

func (f scanFunc) ScanInstalledPackages(ctx context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	return f(ctx, root)
}

// TestStartWithoutReadableHostRoot checks that a host whose packages cannot be
// read leaves the probe running. Start is called from Init, so returning the
// error would stop system-probe from starting at all, taking the rest of what it
// does down with the enrichment.
func TestStartWithoutReadableHostRoot(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{SBOMResolverHostEnabled: true}, hostReport)
	r.hostSBOM = nil
	r.hostRoot = "/does-not-exist"
	r.sbomCollector = scanFunc(func(_ context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
		return nil, fmt.Errorf("cannot read the package database under %s", root)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := r.Start(ctx); err != nil {
		t.Errorf("Start = %v, want the host scan failure to be absorbed", err)
	}
	// The entry is kept so a later refresh can read the database that was
	// unreadable now, and it stays uncomputed so nothing resolves against it.
	if r.hostSBOM == nil {
		t.Fatalf("the host entry was dropped, so no refresh can ever retry the scan")
	}
	if r.hostSBOM.IsComputed() {
		t.Errorf("the host index is marked computed while its scan failed")
	}
	pc := &model.ProcessContext{}
	file := &model.FileEvent{PathnameStr: "/usr/bin/su", IsPathnameStrResolved: true}
	if pkg := r.ResolvePackage(pc, file); pkg != nil {
		t.Errorf("package = %+v, want none from an index that was never computed", *pkg)
	}
}

// TestRefreshHostPeriodicallyQueuesRescan checks that the host index is re-queued
// for a scan on the configured interval. A container is scanned whenever its
// workload appears, while the host is scanned once at startup, so without this a
// package upgraded afterwards stops matching the index and everything it owns is
// reported as never running.
func TestRefreshHostPeriodicallyQueuesRescan(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{
		SBOMResolverHostEnabled:         true,
		SBOMResolverHostRefreshInterval: time.Millisecond,
	}, hostReport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.refreshHostPeriodically(ctx)

	select {
	case queued := <-r.scanChan:
		if queued != r.hostSBOM {
			t.Errorf("queued %v, want the host SBOM", queued)
		}
		if queued.IsComputed() {
			t.Errorf("the host SBOM was queued while still marked computed, so analyzeWorkload would drop the rescan")
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the host SBOM was never re-queued for a scan")
	}
}

// TestDropQueuedHostSBOMWithoutIndexStaysPending checks that giving up on a
// queued scan leaves an index that was never computed in the pending state. A
// host whose first scan failed has an empty index, and marking it computed would
// have every file access resolve against it and find nothing.
func TestDropQueuedHostSBOMWithoutIndexStaysPending(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{}, nil)
	r.hostSBOM = NewSBOM("", nil, hostWorkloadKey)

	r.dropQueuedSBOM(r.hostSBOM)

	if r.hostSBOM.IsComputed() {
		t.Errorf("an index that was never computed is marked computed")
	}
}

// TestTerminalScanFailureReleasesPendingScan checks that a scan which exhausts
// its retries releases its pending entry. addPendingScan refuses a workload
// already listed, so a stale entry would leave the host's periodic refresh
// unable to queue anything for the lifetime of the process.
func TestTerminalScanFailureReleasesPendingScan(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{
		SBOMResolverHostEnabled:         true,
		SBOMResolverHostRefreshInterval: time.Millisecond,
	}, hostReport)
	r.hostRoot = "/does-not-exist"
	r.sbomCollector = scanFunc(func(_ context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
		return nil, fmt.Errorf("cannot read the package database under %s", root)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The refresher and the scan loop both run, so a released entry shows up as a
	// second queued scan following the first failure.
	for range 2 {
		select {
		case <-r.scanChan:
		case <-time.After(15 * time.Second):
			t.Fatalf("a failed scan kept its pending entry, so no later refresh could be queued")
		}
		r.removePendingScan("")
	}
}

// TestRefreshHostPeriodicallyOffByInterval checks that a zero interval leaves the
// host index alone, so the refresh can be turned off.
func TestRefreshHostPeriodicallyOffByInterval(t *testing.T) {
	r := hostResolver(t, &config.RuntimeSecurityConfig{SBOMResolverHostEnabled: true}, hostReport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { r.refreshHostPeriodically(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("refreshHostPeriodically did not return for a zero interval")
	}
	if len(r.scanChan) != 0 {
		t.Errorf("a scan was queued while the refresh is off")
	}
}
