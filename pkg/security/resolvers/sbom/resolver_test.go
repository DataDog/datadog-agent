// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/skydive-project/go-debouncer"
	"go.uber.org/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

func newHostMisses(t *testing.T) *simplelru.LRU[string, hostMiss] {
	misses, err := simplelru.NewLRU[string, hostMiss](maxHostMisses, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	return misses
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
	synctest.Test(t, func(t *testing.T) {
		dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
		if err != nil {
			t.Fatalf("NewLRU: %v", err)
		}
		r := &Resolver{
			Notifier:          utils.NewNotifier[Event, *sbompkg.ScanResult](),
			cfg:               &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: 10 * time.Millisecond},
			dataCache:         dataCache,
			scanChan:          make(chan *SBOM, 1),
			pendingFileEvents: newPendingFileEvents(t),
			sbomsCacheHit:     atomic.NewUint64(0),
			sbomsCacheMiss:    atomic.NewUint64(0),
		}
		reports := make(chan *sbompkg.ScanResult, 1)
		if err := r.RegisterListener(SBOMComputed, func(result *sbompkg.ScanResult) {
			select {
			case reports <- result:
			default:
			}
		}); err != nil {
			t.Fatalf("RegisterListener: %v", err)
		}

		dataCache.Add("image:tag", newData([]sbomtypes.PackageWithInstalledFiles{{
			Package:        sbomtypes.Package{Name: "shadow-utils"},
			InstalledFiles: []string{"/usr/bin/su"},
		}}, false))

		sbom := NewSBOM("container-id", nil, "image:tag")
		sbom.status = workloadmeta.Success
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

		// The replica forwards the usage it drained, with no later event.
		select {
		case <-reports:
		case <-time.After(10 * time.Second):
			t.Errorf("the drained usage was not forwarded")
		}
	})
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
// the queued accesses to the packages owning the files and latches a forward.
func TestProcessPendingFileEventsEnrichesPackages(t *testing.T) {
	r := newPendingFileEventsResolver(t)
	r.cfg = &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Hour}

	sbom := NewSBOM("container-id", nil, "image:tag")
	t.Cleanup(sbom.stop)
	sbom.data = newData([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "shadow-utils"},
		InstalledFiles: []string{"/usr/bin/su"},
	}}, false)

	r.queuePendingFileEvent("container-id", "/usr/bin/su", 04755, 0)
	r.queuePendingFileEvent("container-id", "/usr/bin/not-in-any-package", 0644, 1000)

	sbom.Lock()
	r.processPendingFileEvents(sbom)
	sbom.Unlock()

	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("pending events were not drained")
	}
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want last access and both sticky properties set", pkg)
	}
	if sbom.forwarder == nil {
		t.Errorf("no forward was latched")
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
	sbomB.status = workloadmeta.Success
	sbomB.state.Store(computedState)

	r := newPendingFileEventsResolver(t)
	r.Notifier = utils.NewNotifier[Event, *sbompkg.ScanResult]()
	r.cfg = &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Hour}
	t.Cleanup(sbomA.stop)

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

	// Reader: forward snapshots the packages of sbomB (reads the shared Data
	// via copy()).
	wg.Go(func() {
		for range 2000 {
			r.forward(sbomB)
		}
	})

	wg.Wait()
}

// TestStopLeavesBusyDebouncers checks that stopping an SBOM returns while its
// debouncers are busy. Their callbacks take the locks the callers of stop hold,
// so waiting for them deadlocked the resolver. A debouncer started after stop
// returns stands for a busy one.
func TestStopLeavesBusyDebouncers(t *testing.T) {
	refresher := debouncer.New(time.Hour, func() {})
	forwarder := debouncer.New(time.Hour, func() {})

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.refresher = refresher
	sbom.forwarder = forwarder

	stopped := make(chan struct{})
	go func() {
		sbom.stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("stop waited for its busy debouncers")
	}

	// The debouncers take the Stops that stop left pending, and end.
	refresher.Start()
	forwarder.Start()
}

// TestForwardSkipsStoppedSBOM checks that forward returns at once for a stopped
// SBOM, as a callback still running after stop may call it.
func TestForwardSkipsStoppedSBOM(t *testing.T) {
	r := &Resolver{Notifier: utils.NewNotifier[Event, *sbompkg.ScanResult]()}

	forwarded := false
	if err := r.RegisterListener(SBOMComputed, func(*sbompkg.ScanResult) {
		forwarded = true
	}); err != nil {
		t.Fatalf("RegisterListener: %v", err)
	}

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.setReport([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "bash"},
		InstalledFiles: []string{"/usr/bin/bash"},
	}})
	sbom.status = workloadmeta.Success
	sbom.stop()

	if r.forward(sbom) {
		t.Errorf("the forward of a stopped SBOM asks for a retry")
	}
	if forwarded {
		t.Errorf("a stopped SBOM was forwarded")
	}
}

// TestRefreshScanKeepsStoppedSBOM checks that a refresh still running after its
// workload was deleted keeps the SBOM stopped and out of the scan queue, and
// keeps the cached data of the image for its other containers.
func TestRefreshScanKeepsStoppedSBOM(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := &Resolver{
		dataCache: dataCache,
		scanChan:  make(chan *SBOM, 10),
	}

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.stop()
	dataCache.Add("image:tag", &Data{})

	r.refreshScan(sbom)

	if got := sbom.state.Load(); got != stoppedState {
		t.Errorf("state = %d, want stoppedState (%d)", got, stoppedState)
	}
	if len(r.scanChan) != 0 {
		t.Errorf("the deleted workload was queued for a scan")
	}
	if _, ok := dataCache.Get("image:tag"); !ok {
		t.Errorf("the deleted workload dropped the cached data of its image")
	}
}

// failingWorkloadmeta answers every container lookup with an error, as the
// store answers for an unknown ID.
type failingWorkloadmeta struct {
	workloadmeta.Component
}

func (failingWorkloadmeta) GetContainer(id string) (*workloadmeta.Container, error) {
	return nil, fmt.Errorf("container %q not found", id)
}

// TestGetContainerSBOMWithoutWorkloadmeta checks that a resolver without
// workloadmeta, as in the functional tests, reports an error for a container.
func TestGetContainerSBOMWithoutWorkloadmeta(t *testing.T) {
	r := &Resolver{}
	if sbom, err := r.getContainerSBOM("container-id"); err == nil || sbom != nil {
		t.Errorf("getContainerSBOM() = %v, %v, want an error", sbom, err)
	}
}

// ownRoot returns the device and the inode of the root of the test process.
func ownRoot(t *testing.T) (dev, ino uint64) {
	t.Helper()

	stat, err := utils.UnixStat(utils.ProcRootPath(uint32(os.Getpid())))
	if err != nil {
		t.Fatalf("stat of the root of the test process: %v", err)
	}
	return stat.Dev, stat.Ino
}

// tempRoot returns the device and the inode of a fresh directory, standing for
// a foreign root.
func tempRoot(t *testing.T) (dev, ino uint64) {
	t.Helper()

	stat, err := utils.UnixStat(t.TempDir())
	if err != nil {
		t.Fatalf("stat of a temporary directory: %v", err)
	}
	return stat.Dev, stat.Ino
}

// newHostSBOMResolver returns a resolver whose host SBOM is computed and holds
// util-linux, the owner of /usr/bin/su and of its documentation directory. The
// host root is the root of the test process. Its forwarder waits an hour, so
// tests call forward themselves.
func newHostSBOMResolver(t *testing.T) *Resolver {
	t.Helper()

	r := &Resolver{
		Notifier: utils.NewNotifier[Event, *sbompkg.ScanResult](),
		cfg: &config.RuntimeSecurityConfig{
			SBOMResolverEnrichmentInterval: time.Minute,
			SBOMResolverForwardInterval:    time.Hour,
		},
		pendingFileEvents: newPendingFileEvents(t),
		hostMisses:        newHostMisses(t),
		wmeta:             failingWorkloadmeta{},
	}

	r.hostRootDevice, r.hostRootInode = ownRoot(t)

	r.hostSBOM = NewSBOM("", nil, "")
	r.hostSBOM.setReport([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "util-linux", Version: "2.40.4"},
		InstalledFiles: []string{"/usr/bin/su", "/usr/share/doc/util-linux"},
	}})
	r.hostSBOM.state.Store(computedState)
	t.Cleanup(r.hostSBOM.stop)

	return r
}

// hostAccess returns the test process, a root process with an empty container
// ID, running /usr/bin/su, setuid, as the host runs it.
func hostAccess() (*model.ProcessContext, *model.FileEvent) {
	pc := &model.ProcessContext{}
	pc.Pid = uint32(os.Getpid())

	file := &model.FileEvent{}
	file.SetPathnameStr("/usr/bin/su")
	file.Mode = 04755
	return pc, file
}

// TestResolvePackageRecordsHostUsage checks that a process with an empty
// container ID resolves against the host SBOM and records its usage there, as a
// process of a container does against the SBOM of its container.
func TestResolvePackageRecordsHostUsage(t *testing.T) {
	r := newHostSBOMResolver(t)

	pc, file := hostAccess()
	pkg := r.ResolvePackage(pc, file)
	if pkg == nil || pkg.Name != "util-linux" {
		t.Fatalf("package = %+v, want util-linux", pkg)
	}
	if pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want last access and both sticky properties set", pkg)
	}
}

// TestResolvePackageWithoutHostSBOM checks that ResolvePackage returns nil for a
// host process while the host SBOM is off, and leaves the queue of pending
// accesses empty.
func TestResolvePackageWithoutHostSBOM(t *testing.T) {
	r := newPendingFileEventsResolver(t)

	pc, file := hostAccess()
	if pkg := r.ResolvePackage(pc, file); pkg != nil {
		t.Errorf("package = %+v, want none without a host SBOM", pkg)
	}
	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("the host access was queued")
	}
}

// TestLookupPackageRecordsNoUsage checks that a lookup, as the package fields of
// a CWS event make, returns the package of a host file, leaves its usage as it
// is and leaves the forwarder of the host report idle.
func TestLookupPackageRecordsNoUsage(t *testing.T) {
	r := newHostSBOMResolver(t)

	pc, file := hostAccess()
	pkg := r.LookupPackage(pc, file)
	if pkg == nil || pkg.Name != "util-linux" {
		t.Fatalf("package = %+v, want util-linux", pkg)
	}
	if !pkg.LastAccess.IsZero() || pkg.SuidBit || pkg.AccessedByRoot {
		t.Errorf("package = %+v, want its usage as it was", pkg)
	}
	if r.hostSBOM.forwarder != nil {
		t.Errorf("the lookup armed the forwarding of the host report")
	}
}

// TestLookupPackageQueuesNoAccess checks that a lookup for a container leaves
// the queue of pending accesses empty, before the SBOM of the container exists
// and while its scan runs. The queue replays its accesses as usage.
func TestLookupPackageQueuesNoAccess(t *testing.T) {
	sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](2, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := newPendingFileEventsResolver(t)
	r.sboms = sboms

	pc, file := hostAccess()
	pc.ContainerContext.ContainerID = "container-id"
	if pkg := r.LookupPackage(pc, file); pkg != nil {
		t.Errorf("package = %+v before the SBOM of the container, want none", pkg)
	}

	r.sboms.Add("container-id", NewSBOM("container-id", nil, "workload"))
	if pkg := r.LookupPackage(pc, file); pkg != nil {
		t.Errorf("package = %+v while the scan runs, want none", pkg)
	}

	if n := r.pendingFileEvents.Len(); n != 0 {
		t.Errorf("%d containers hold queued accesses, want 0", n)
	}
}

// TestHostPackageVersion checks that a systemd service takes the version of the
// host package of its unit file. dpkg lists the units of Debian under /lib,
// which a merged /usr makes an alias of the path the tags resolver asks for.
func TestHostPackageVersion(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.hostSBOM.usrMerged = true
	r.hostSBOM.setReport([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "openssh-server", Epoch: 1, Version: "9.9p1", Release: "3"},
		InstalledFiles: []string{"/lib/systemd/system/ssh.service"},
	}})

	for path, want := range map[string]string{
		"/usr/lib/systemd/system/ssh.service":       "9.9p1",
		"/usr/lib/systemd/system/transient.service": "",
	} {
		if got := r.HostPackageVersion(path); got != want {
			t.Errorf("version of %s = %q, want %q", path, got, want)
		}
	}
	if pkg := r.hostSBOM.data.packages[0]; !pkg.LastAccess.IsZero() || pkg.AccessedByRoot {
		t.Errorf("package = %+v, want its usage as it was", pkg)
	}
}

// TestHostPackageVersionWithoutHostSBOM checks that the version is empty while
// the host index is off, which the tags resolver turns into version:latest.
func TestHostPackageVersionWithoutHostSBOM(t *testing.T) {
	r := newPendingFileEventsResolver(t)
	if got := r.HostPackageVersion("/usr/lib/systemd/system/ssh.service"); got != "" {
		t.Errorf("version = %q without a host SBOM, want empty", got)
	}
}

// TestHostForwardingSkipsImageSBOM checks that the report of the host goes out
// at once. A container report waits for the Trivy SBOM of its image, while the
// core agent keeps the report of the host for its next host scan. Here every
// workloadmeta lookup fails, so a host report that waited on an image SBOM
// would wait forever.
func TestHostForwardingSkipsImageSBOM(t *testing.T) {
	r := newHostSBOMResolver(t)

	var reports []*sbompkg.ScanResult
	if err := r.RegisterListener(SBOMComputed, func(result *sbompkg.ScanResult) {
		reports = append(reports, result)
	}); err != nil {
		t.Fatalf("RegisterListener: %v", err)
	}

	pc, file := hostAccess()
	r.ResolvePackage(pc, file)

	if r.forward(r.hostSBOM) {
		t.Errorf("the host report waits to be forwarded again")
	}
	if len(reports) != 1 {
		t.Fatalf("%d host reports forwarded, want 1", len(reports))
	}
	if id := reports[0].RequestID; id != "" {
		t.Errorf("request ID = %q, want the empty ID of the host", id)
	}
	components := reports[0].Report.ToCycloneDX().GetComponents()
	if len(components) != 1 {
		t.Fatalf("report holds %d components, want 1", len(components))
	}
	if seen, _ := propertyValue(components[0], LastAccessProperty); seen == "0" || seen == "" {
		t.Errorf("%s = %q, want util-linux seen running", LastAccessProperty, seen)
	}
	want := strconv.FormatInt(r.hostSBOM.data.observedSince.Unix(), 10)
	if got := metadataValue(reports[0].Report.ToCycloneDX(), UsageObservedSinceProperty); got != want {
		t.Errorf("%s = %q, want %s, the start of the host usage", UsageObservedSinceProperty, got, want)
	}
}

// noFingerprint gives a test package scanner the fingerprint of a root whose
// package databases are unknown.
type noFingerprint struct{}

func (noFingerprint) Fingerprint(string) string { return "" }

// rootRecorder is a package scanner that records the root it was given and
// finds the packages of report there.
type rootRecorder struct {
	noFingerprint
	root   string
	report []sbomtypes.PackageWithInstalledFiles
}

func (s *rootRecorder) ScanInstalledPackages(_ context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	s.root = root
	return s.report, nil
}

// TestStartIndexesHostThroughInitRoot checks that the host packages are read
// through the root of init, as those of a container are read through the root
// of one of its processes. A containerized system-probe sees its own image at
// /, and HOST_ROOT, here pointing elsewhere, may be unset.
func TestStartIndexesHostThroughInitRoot(t *testing.T) {
	t.Setenv("HOST_ROOT", t.TempDir())

	scanner := &rootRecorder{}
	r := &Resolver{
		cfg: &config.RuntimeSecurityConfig{
			SBOMResolverHostEnabled:     true,
			SBOMResolverForwardInterval: time.Hour,
		},
		sbomCollector:         scanner,
		hostMisses:            newHostMisses(t),
		sbomGenerations:       atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(r.hostSBOM.stop)

	if want := utils.ProcRootPath(1); scanner.root != want {
		t.Errorf("host scanned at %q, want %q", scanner.root, want)
	}
	if !r.hostSBOM.IsComputed() {
		t.Errorf("host SBOM left uncomputed")
	}
}

// TestOnHostRoot checks that a process counts as a host process when its root
// is the root of the host, or when it is gone before its root can be read.
func TestOnHostRoot(t *testing.T) {
	pid := uint32(os.Getpid())

	r := &Resolver{}
	r.hostRootDevice, r.hostRootInode = ownRoot(t)
	if !r.onHostRoot(pid) {
		t.Errorf("the test process, on the host root, counts as foreign")
	}
	if !r.onHostRoot(math.MaxUint32) {
		t.Errorf("a process gone before its root was read counts as foreign")
	}

	r.hostRootDevice, r.hostRootInode = tempRoot(t)
	if r.onHostRoot(pid) {
		t.Errorf("the test process counts as a host process with the host root elsewhere")
	}
}

// TestResolvePackageSkipsForeignRoot checks that ResolvePackage returns nil for
// a process with an empty container ID on a root of its own, as in a system
// container or a snap, and leaves the usage of the host packages as it was.
func TestResolvePackageSkipsForeignRoot(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.hostRootDevice, r.hostRootInode = tempRoot(t)

	pc, file := hostAccess()
	if pkg := r.ResolvePackage(pc, file); pkg != nil {
		t.Errorf("package = %+v, want none for a process on a root of its own", pkg)
	}
	if pkg := r.hostSBOM.data.packages[0]; !pkg.LastAccess.IsZero() || pkg.SuidBit || pkg.AccessedByRoot {
		t.Errorf("package = %+v, want the usage left as it was", pkg)
	}
}

// TestResolvePackageRecordsNoUsageOnDirectory checks that opening a directory a
// package owns, which rpm lists among its files, resolves to the package and
// leaves its usage as it was. A walk of the filesystem opens every directory.
func TestResolvePackageRecordsNoUsageOnDirectory(t *testing.T) {
	r := newHostSBOMResolver(t)

	pc, _ := hostAccess()
	dir := &model.FileEvent{}
	dir.SetPathnameStr("/usr/share/doc/util-linux")
	dir.Mode = syscall.S_IFDIR | 0755

	pkg := r.ResolvePackage(pc, dir)
	if pkg == nil || pkg.Name != "util-linux" {
		t.Fatalf("package = %+v, want util-linux, the owner of the directory", pkg)
	}
	if !pkg.LastAccess.IsZero() || pkg.AccessedByRoot {
		t.Errorf("package = %+v, want the usage left as it was", pkg)
	}
	if r.hostSBOM.forwarder != nil {
		t.Errorf("the directory open triggered forwarding")
	}
}

// TestResolveOpen checks that the open of a host file records usage once it
// has waited settleDelay, if the file is still at its path then, and leaves
// the usage as it is when the file is gone or replaced there, as rpm leaves
// each executable it opens to clear its file capabilities. An overlayfs file
// counts on an overlayfs host root alone: elsewhere it is the file of a
// container, seen through /proc/<pid>/root.
func TestResolveOpen(t *testing.T) {
	for _, tc := range []struct {
		name         string
		after        func(t *testing.T, path string)
		flags        int32
		overlayRoot  bool
		unknownInode bool
		records      bool
	}{
		{name: "open", records: true},
		{name: "open of a removed file", after: removeFile},
		{name: "open of a replaced file", after: replaceFile},
		{name: "open of an overlayfs file", flags: model.LowerLayer},
		{name: "open of an overlayfs file on an overlayfs host root", flags: model.LowerLayer, overlayRoot: true, records: true},
		{name: "open of a replaced overlayfs file on an overlayfs host root", after: replaceFile, flags: model.UpperLayer, overlayRoot: true, records: true},
		{name: "open of a removed file of unknown inode", after: removeFile, unknownInode: true, records: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.hostRoot = t.TempDir()
			r.hostRootOverlay = tc.overlayRoot

			pc, file := hostAccess()
			path := createFile(t, r.hostRoot+file.PathnameStr)
			if !tc.unknownInode {
				file.Inode = inode(t, path)
			}
			file.Flags = tc.flags

			r.ResolveOpen(pc, file)
			if len(r.opened) != 1 {
				t.Fatalf("%d opens wait, want the open of %s", len(r.opened), file.PathnameStr)
			}
			opened := r.opened[0].at
			if tc.after != nil {
				tc.after(t, path)
			}

			pkg := &r.hostSBOM.data.packages[0]
			r.recordSettled(opened)
			if !pkg.LastAccess.IsZero() {
				t.Errorf("usage recorded before the open waited %v", settleDelay)
			}
			r.recordSettled(opened.Add(settleDelay))
			if got := !pkg.LastAccess.IsZero(); got != tc.records {
				t.Errorf("usage recorded = %v, want %v", got, tc.records)
			}
		})
	}
}

// TestResolvePackageRecordsRunOfReplacedFile checks that a process running a
// file an upgrade replaced records usage, as it keeps running the build the
// upgrade replaced.
func TestResolvePackageRecordsRunOfReplacedFile(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.hostRoot = t.TempDir()

	pc, file := hostAccess()
	path := createFile(t, r.hostRoot+file.PathnameStr)
	file.Inode = inode(t, path)
	replaceFile(t, path)

	if pkg := r.ResolvePackage(pc, file); pkg == nil || pkg.LastAccess.IsZero() {
		t.Errorf("package = %+v, want util-linux with its usage recorded", pkg)
	}
}

// TestResolveOpenWithFullQueue checks that the open of a host file arriving
// while maxOpened opens wait for settleDelay is dropped, and leaves the usage
// of its package as it is.
func TestResolveOpenWithFullQueue(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.opened = make([]openedFile, maxOpened)

	pc, file := hostAccess()
	r.ResolveOpen(pc, file)

	if pkg := r.hostSBOM.data.packages[0]; !pkg.LastAccess.IsZero() {
		t.Errorf("package = %+v, want util-linux with its usage as it was", pkg)
	}
	if len(r.opened) != maxOpened {
		t.Errorf("%d opens wait, want %d", len(r.opened), maxOpened)
	}
}

// TestResolveOpenWaitsForPackageFiles checks that the opens of host files
// outside the host packages, and of directories, stay out of the opens waiting
// for settleDelay, while the open of a host package file waits.
func TestResolveOpenWaitsForPackageFiles(t *testing.T) {
	r := newHostSBOMResolver(t)
	pc, _ := hostAccess()

	other := &model.FileEvent{}
	other.SetPathnameStr("/tmp/report.txt")
	r.ResolveOpen(pc, other)

	dir := &model.FileEvent{}
	dir.SetPathnameStr("/usr/share/doc/util-linux")
	dir.Mode = syscall.S_IFDIR | 0755
	r.ResolveOpen(pc, dir)

	if len(r.opened) != 0 {
		t.Fatalf("%d opens wait, want none", len(r.opened))
	}

	_, file := hostAccess()
	r.ResolveOpen(pc, file)
	if len(r.opened) != 1 {
		t.Errorf("%d opens wait, want the open of /usr/bin/su", len(r.opened))
	}
}

// TestResolveOpenOfContainerFile checks that the open of a file of a container
// records usage at once, whatever stands at the same path on the host.
func TestResolveOpenOfContainerFile(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.hostRoot = t.TempDir()

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.setReport([]sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "util-linux", Version: "2.40.4"},
		InstalledFiles: []string{"/usr/bin/su"},
	}})
	sbom.state.Store(computedState)
	t.Cleanup(sbom.stop)
	sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](1, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	sboms.Add(sbom.ContainerID, sbom)
	r.sboms = sboms

	pc, file := hostAccess()
	pc.ContainerContext.ContainerID = sbom.ContainerID
	file.Inode = 1

	r.ResolveOpen(pc, file)
	if pkg := sbom.data.packages[0]; pkg.LastAccess.IsZero() {
		t.Errorf("package = %+v, want util-linux with its usage recorded", pkg)
	}
}

// createFile creates an empty executable file at path and its directories,
// and returns path.
func createFile(t *testing.T, path string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestHoldsCode checks which files of a package can hold code that a process
// runs: executables, shared libraries, kernel modules and interpreter modules.
func TestHoldsCode(t *testing.T) {
	for _, tc := range []struct {
		path string
		perm uint16
		want bool
	}{
		{"/usr/share/zoneinfo/Europe/Paris", 0644, false},
		{"/usr/share/ca-certificates/mozilla/ISRG_Root_X1.crt", 0644, false},
		{"/usr/lib/os-release", 0644, false},
		{"/usr/lib/modprobe.d/blacklist_linux_6.8.0-111-generic.conf", 0644, false},
		{"/usr/share/crypto-policies/DEFAULT/openssl.txt", 0644, false},
		{"/usr/bin/tcpdump", 0755, true},
		{"/usr/lib/x86_64-linux-gnu/libc.so.6", 0644, true},
		{"/usr/lib/x86_64-linux-gnu/libpcap.so", 0644, true},
		{"/usr/lib/python3/dist-packages/requests/api.py", 0644, true},
		{"/usr/lib/python3/dist-packages/requests/__pycache__/api.cpython-312.pyc", 0644, true},
		{"/usr/lib/modules/6.8.0-139-generic/kernel/fs/xfs/xfs.ko.zst", 0644, true},
		{"/usr/share/java/commons-io.jar", 0644, true},
		{"/usr/lib64/libssl.so.1.0.2k", 0644, true},
		{"/usr/lib/python3/dist-packages/_cffi_backend.cpython-312-x86_64-linux-gnu.so", 0644, true},
		{"/usr/lib/erlang/lib/ssl-11.1/ebin/ssl.beam", 0644, true},
		{"/usr/lib/mono/4.5/mscorlib.dll", 0644, true},
		{"/usr/lib/modules/5.14.0-503.el9.x86_64/kernel/fs/xfs/xfs.ko.gz", 0644, true},
		{"/etc/profile.d/which2.sh", 0644, false},
		{"/etc/init.d/ssh", 0755, false},
		{"/etc/ld.so.cache", 0644, false},
		{"/usr/share/man/man8/ld.so.8.gz", 0644, false},
		{"/usr/lib64/.libgcrypt.so.20.hmac", 0644, false},
		{"/usr/lib/debug/usr/lib64/libc.so.6-2.39-22.el10.x86_64.debug", 0644, false},
		{"/usr/lib/python3/dist-packages/requests-2.31.0.dist-info/METADATA", 0644, false},
		{"/usr/lib/python3/dist-packages/requests-2.31.0.dist-info/RECORD", 0644, false},
	} {
		file := &model.FileEvent{}
		file.SetPathnameStr(tc.path)
		file.Mode = syscall.S_IFREG | tc.perm
		if got := holdsCode(file); got != tc.want {
			t.Errorf("holdsCode(%s, %o) = %v, want %v", tc.path, tc.perm, got, tc.want)
		}
	}
}

// TestResolveOpenCountsCodeFiles checks that the open of a package file records
// usage when the file can hold code, on the host as in a container, and that a
// run records usage whatever the file.
func TestResolveOpenCountsCodeFiles(t *testing.T) {
	report := []sbomtypes.PackageWithInstalledFiles{
		{Package: sbomtypes.Package{Name: "tzdata", Version: "2026c"}, InstalledFiles: []string{"/usr/share/zoneinfo/UTC"}},
		{Package: sbomtypes.Package{Name: "libc6", Version: "2.39"}, InstalledFiles: []string{"/usr/lib/x86_64-linux-gnu/libc.so.6"}},
	}
	for _, tc := range []struct {
		name      string
		container bool
		run       bool
		path      string
		records   bool
	}{
		{name: "open of a host data file", path: "/usr/share/zoneinfo/UTC"},
		{name: "open of a host shared library", path: "/usr/lib/x86_64-linux-gnu/libc.so.6", records: true},
		{name: "open of a container data file", container: true, path: "/usr/share/zoneinfo/UTC"},
		{name: "open of a container shared library", container: true, path: "/usr/lib/x86_64-linux-gnu/libc.so.6", records: true},
		{name: "run of a host data file", run: true, path: "/usr/share/zoneinfo/UTC", records: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.hostSBOM.setReport(report)
			sbom := r.hostSBOM

			pc, _ := hostAccess()
			if tc.container {
				sbom = NewSBOM("container-id", nil, "image:tag")
				sbom.setReport(report)
				sbom.state.Store(computedState)
				t.Cleanup(sbom.stop)
				sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](1, nil)
				if err != nil {
					t.Fatalf("NewLRU: %v", err)
				}
				sboms.Add(sbom.ContainerID, sbom)
				r.sboms = sboms
				pc.ContainerContext.ContainerID = sbom.ContainerID
			}

			file := &model.FileEvent{}
			file.SetPathnameStr(tc.path)
			file.Mode = syscall.S_IFREG | 0644
			if tc.run {
				r.ResolvePackage(pc, file)
			} else {
				r.ResolveOpen(pc, file)
				r.recordSettled(time.Now().Add(settleDelay))
			}

			pkg := sbom.data.files.queryFile(tc.path)
			if pkg == nil {
				t.Fatalf("no package owns %s", tc.path)
			}
			if got := !pkg.LastAccess.IsZero(); got != tc.records {
				t.Errorf("usage of %s recorded = %v, want %v", pkg.Name, got, tc.records)
			}
		})
	}
}

// TestReadsCode checks which processes read code without running it: the
// indexers, builders, scanners and inspectors of codeReaders, the children of
// a Python byte compiler, and the dynamic loader run as a program by ldd.
func TestReadsCode(t *testing.T) {
	for _, tc := range []struct {
		comm   string
		parent string
		want   bool
	}{
		{comm: "ldconfig", want: true},
		{comm: "ldconfig.real", want: true},
		{comm: "depmod", want: true},
		{comm: "cp", want: true},
		{comm: "systemd-coredum", want: true},
		{comm: "ld-linux-x86-64", want: true},
		{comm: "ld-linux-aarch6", want: true},
		{comm: "py3compile", want: true},
		{comm: "python3.12", parent: "py3compile", want: true},
		{comm: "python3.12", parent: "bash"},
		{comm: "python3.12"},
		{comm: "modprobe"},
		{comm: "rpm"},
		{comm: "sshd"},
	} {
		pc := &model.ProcessContext{}
		pc.Comm = tc.comm
		if tc.parent != "" {
			pc.Parent = &model.Process{Comm: tc.parent}
		}
		if got := readsCode(pc); got != tc.want {
			t.Errorf("readsCode(%q, parent %q) = %v, want %v", tc.comm, tc.parent, got, tc.want)
		}
	}
}

// TestResolveOpenLeavesReadsOfCode checks that the open of a shared library by a
// process that reads code without running it leaves the usage of its package
// as it is, on the host as in a container, while another process opening it
// records usage.
func TestResolveOpenLeavesReadsOfCode(t *testing.T) {
	const lib = "/usr/lib/x86_64-linux-gnu/libc.so.6"
	report := []sbomtypes.PackageWithInstalledFiles{
		{Package: sbomtypes.Package{Name: "libc6", Version: "2.39"}, InstalledFiles: []string{lib}},
	}
	for _, tc := range []struct {
		name      string
		comm      string
		container bool
		records   bool
	}{
		{name: "ldconfig on the host", comm: "ldconfig"},
		{name: "ldconfig in a container", comm: "ldconfig", container: true},
		{name: "sshd on the host", comm: "sshd", records: true},
		{name: "sshd in a container", comm: "sshd", container: true, records: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.hostSBOM.setReport(report)
			sbom := r.hostSBOM

			pc, _ := hostAccess()
			pc.Comm = tc.comm
			if tc.container {
				sbom = NewSBOM("container-id", nil, "image:tag")
				sbom.setReport(report)
				sbom.state.Store(computedState)
				t.Cleanup(sbom.stop)
				sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](1, nil)
				if err != nil {
					t.Fatalf("NewLRU: %v", err)
				}
				sboms.Add(sbom.ContainerID, sbom)
				r.sboms = sboms
				pc.ContainerContext.ContainerID = sbom.ContainerID
			}

			file := &model.FileEvent{}
			file.SetPathnameStr(lib)
			file.Mode = syscall.S_IFREG | 0644
			r.ResolveOpen(pc, file)
			r.recordSettled(time.Now().Add(settleDelay))

			if got := !sbom.data.packages[0].LastAccess.IsZero(); got != tc.records {
				t.Errorf("usage of libc6 recorded = %v, want %v", got, tc.records)
			}
		})
	}
}

// removeFile deletes the file at path, as rpm deletes an executable it removes.
func removeFile(t *testing.T, path string) {
	t.Helper()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// replaceFile renames a new file over the file at path, as rpm installs each
// file of a package it upgrades or reinstalls.
func replaceFile(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path+".new", nil, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
}

// inode returns the inode of the file at path.
func inode(t *testing.T, path string) uint64 {
	t.Helper()

	stat, err := utils.UnixStat(path)
	if err != nil {
		t.Fatal(err)
	}
	return stat.Ino
}

// TestPendingFileEventsSkipDirectories checks that a directory open waiting on
// the SBOM of its container stays out of the queue, as a directory open leaves
// the usage of its package as it is.
func TestPendingFileEventsSkipDirectories(t *testing.T) {
	r := newPendingFileEventsResolver(t)

	r.queuePendingFileEvent("container-id", "/usr/share/doc", syscall.S_IFDIR|0755, 0)
	if r.pendingFileEvents.Len() != 0 {
		t.Errorf("the directory open was queued")
	}
}

// TestRefreshScanRescansHost checks that a refresh of the host scans the host
// packages again in place, outside the queue and the cache of the workload
// scans, and keeps the usage of the packages it finds again, upgraded ones
// included.
func TestRefreshScanRescansHost(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}

	r := newHostSBOMResolver(t)
	r.dataCache = dataCache
	r.scanChan = make(chan *SBOM, 1)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)

	utilLinux := sbomtypes.PackageWithInstalledFiles{
		Package:        sbomtypes.Package{Name: "util-linux", Version: "2.40.4"},
		InstalledFiles: []string{"/usr/bin/su"},
	}
	r.hostSBOM.setReport([]sbomtypes.PackageWithInstalledFiles{utilLinux, {
		Package:        sbomtypes.Package{Name: "gzip", Version: "1.12"},
		InstalledFiles: []string{"/usr/bin/gzip"},
	}})
	pc, file := hostAccess()
	r.ResolvePackage(pc, file)
	file.SetPathnameStr("/usr/bin/gzip")
	file.Mode = 0755
	r.ResolvePackage(pc, file)

	since := r.hostSBOM.data.observedSince

	r.sbomCollector = &rootRecorder{report: []sbomtypes.PackageWithInstalledFiles{utilLinux, {
		Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
		InstalledFiles: []string{"/usr/bin/gzip"},
	}}}
	r.refreshScan(r.hostSBOM)

	pkgs := r.hostSBOM.data.packages
	if len(pkgs) != 2 || pkgs[0].Name != "util-linux" || pkgs[1].Version != "1.13" {
		t.Fatalf("host packages = %+v, want util-linux and gzip 1.13, from the new scan", pkgs)
	}
	if pkg := pkgs[0]; pkg.LastAccess.IsZero() || !pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want the usage of util-linux kept", pkg)
	}
	if pkg := pkgs[1]; pkg.LastAccess.IsZero() || pkg.SuidBit || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want the upgraded gzip in use, run as root", pkg)
	}
	if got := r.hostSBOM.data.observedSince; !got.Equal(since) {
		t.Errorf("observation start = %v, want %v, kept with the usage", got, since)
	}
	if !r.hostSBOM.IsComputed() {
		t.Errorf("state = %d, want computedState (%d)", r.hostSBOM.state.Load(), computedState)
	}
	if len(r.scanChan) != 0 {
		t.Errorf("the host was queued as a workload")
	}
	if dataCache.Len() != 0 {
		t.Errorf("the host entered the workload cache")
	}
}

// TestKeepUsageKeepsObservationStart checks that the usage a rescan keeps keeps
// the time it started being recorded and the time it was last forwarded, and
// that the data of a first scan starts recording its own.
func TestKeepUsageKeepsObservationStart(t *testing.T) {
	since := time.Unix(1700000000, 0)
	report := []sbomtypes.PackageWithInstalledFiles{{Package: sbomtypes.Package{Name: "bash", Version: "5.2"}}}

	forwarded := since.Add(time.Hour)
	rescan := newData(report, false)
	rescan.keepUsage(&Data{packages: []sbomtypes.Package{{Name: "bash", Version: "5.1"}}, observedSince: since, forwardedAt: forwarded})
	if !rescan.observedSince.Equal(since) {
		t.Errorf("observation start = %v, want %v, that of the usage kept", rescan.observedSince, since)
	}
	if !rescan.forwardedAt.Equal(forwarded) {
		t.Errorf("last forward = %v, want %v, that of the usage kept", rescan.forwardedAt, forwarded)
	}

	first := newData(report, false)
	start := first.observedSince
	first.keepUsage(&Data{})
	if start.IsZero() || !first.observedSince.Equal(start) {
		t.Errorf("observation start = %v, want %v, that of the first scan", first.observedSince, start)
	}
}

// TestKeepUsage checks the usage a rescan keeps. A package keeps the usage of
// its build, combined with that of the builds of its name the rescan no longer
// finds, as an upgrade replaces them.
func TestKeepUsage(t *testing.T) {
	seen := time.Unix(1700000000, 0)
	used := func(pkg sbomtypes.Package, at time.Time, suid, root bool) sbomtypes.Package {
		pkg.LastAccess, pkg.SuidBit, pkg.AccessedByRoot = at, suid, root
		return pkg
	}
	kernel := func(release string) sbomtypes.Package {
		return sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: release}
	}
	gzip := func(version string) sbomtypes.Package {
		return sbomtypes.Package{Name: "gzip", Version: version}
	}

	tests := []struct {
		name string
		prev []sbomtypes.Package
		scan []sbomtypes.Package
		want []sbomtypes.Package
	}{
		{
			name: "builds side by side",
			prev: []sbomtypes.Package{used(kernel("55.el10"), seen, false, true), kernel("53.el10")},
			scan: []sbomtypes.Package{kernel("55.el10"), kernel("53.el10")},
			want: []sbomtypes.Package{used(kernel("55.el10"), seen, false, true), kernel("53.el10")},
		},
		{
			name: "build installed beside",
			prev: []sbomtypes.Package{used(kernel("55.el10"), seen, false, true), kernel("53.el10")},
			scan: []sbomtypes.Package{kernel("55.el10"), kernel("53.el10"), kernel("57.el10")},
			want: []sbomtypes.Package{used(kernel("55.el10"), seen, false, true), kernel("53.el10"), kernel("57.el10")},
		},
		{
			name: "upgrade",
			prev: []sbomtypes.Package{used(gzip("1.12"), seen, false, true)},
			scan: []sbomtypes.Package{gzip("1.13")},
			want: []sbomtypes.Package{used(gzip("1.13"), seen, false, true)},
		},
		{
			name: "rescan during an upgrade",
			prev: []sbomtypes.Package{used(gzip("1.12"), seen, false, true), gzip("1.13")},
			scan: []sbomtypes.Package{gzip("1.13")},
			want: []sbomtypes.Package{used(gzip("1.13"), seen, false, true)},
		},
		{
			name: "build listed twice",
			prev: []sbomtypes.Package{used(gzip("1.12"), seen, false, true), used(gzip("1.12"), seen.Add(-time.Minute), true, false)},
			scan: []sbomtypes.Package{gzip("1.12"), gzip("1.12")},
			want: []sbomtypes.Package{used(gzip("1.12"), seen, true, true), used(gzip("1.12"), seen, true, true)},
		},
		{
			name: "build removed beside others",
			prev: []sbomtypes.Package{used(kernel("55.el10"), seen, false, true), kernel("53.el10"), kernel("57.el10")},
			scan: []sbomtypes.Package{kernel("53.el10"), kernel("57.el10")},
			want: []sbomtypes.Package{kernel("53.el10"), kernel("57.el10")},
		},
		{
			name: "upgrade of a build listed twice",
			prev: []sbomtypes.Package{used(gzip("1.12"), seen, false, true), gzip("1.12")},
			scan: []sbomtypes.Package{gzip("1.13"), gzip("1.13")},
			want: []sbomtypes.Package{used(gzip("1.13"), seen, false, true), used(gzip("1.13"), seen, false, true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := make([]sbomtypes.PackageWithInstalledFiles, len(tt.scan))
			for i, pkg := range tt.scan {
				report[i].Package = pkg
			}

			d := newData(report, false)
			d.keepUsage(&Data{packages: tt.prev})

			if !slices.EqualFunc(d.packages, tt.want, func(got, want sbomtypes.Package) bool {
				return packageBuild(got) == packageBuild(want) && got.LastAccess.Equal(want.LastAccess) &&
					got.SuidBit == want.SuidBit && got.AccessedByRoot == want.AccessedByRoot
			}) {
				t.Errorf("packages = %+v, want %+v", d.packages, tt.want)
			}
		})
	}
}

// TestScanHostRecordsRunningProcesses checks that a rescan of the host packages
// that finds a new package records the processes running on the host against
// the new index, as the daemon of the package may have started before the scan.
func TestScanHostRecordsRunningProcesses(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)
	r.sbomCollector = &rootRecorder{report: []sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
		InstalledFiles: []string{"/usr/bin/gzip"},
	}}}

	daemon := &model.ProcessCacheEntry{}
	daemon.Pid = uint32(os.Getpid())
	daemon.FileEvent.SetPathnameStr("/usr/bin/gzip")
	daemon.FileEvent.Mode = 0755
	r.SetProcessWalker(func(walk func(*model.ProcessCacheEntry)) {
		walk(daemon)
	})

	if err := r.scanHost(); err != nil {
		t.Fatalf("scanHost: %v", err)
	}

	pkgs := r.hostSBOM.data.packages
	if len(pkgs) != 1 || pkgs[0].Name != "gzip" {
		t.Fatalf("host packages = %+v, want gzip, from the new scan", pkgs)
	}
	if pkg := pkgs[0]; pkg.LastAccess.IsZero() || !pkg.AccessedByRoot {
		t.Errorf("package = %+v, want gzip in use by its running daemon", pkg)
	}
}

// TestScanHostMarksRunningKernel checks that a scan of the host records the
// packages of the running kernel as run by root, its image and the list of its
// built-in modules, and leaves another installed kernel as it is.
func TestScanHostMarksRunningKernel(t *testing.T) {
	for _, tc := range []struct {
		name    string
		release string
		report  []sbomtypes.PackageWithInstalledFiles
		running []string
	}{
		{
			name:    "dpkg",
			release: "6.8.0-139-generic",
			report: []sbomtypes.PackageWithInstalledFiles{
				{Package: sbomtypes.Package{Name: "linux-image-6.8.0-139-generic", Version: "6.8.0-139.139"}, InstalledFiles: []string{"/boot/vmlinuz-6.8.0-139-generic"}},
				{Package: sbomtypes.Package{Name: "linux-modules-6.8.0-139-generic", Version: "6.8.0-139.139"}, InstalledFiles: []string{"/lib/modules/6.8.0-139-generic/modules.builtin"}},
				{Package: sbomtypes.Package{Name: "linux-image-6.8.0-111-generic", Version: "6.8.0-111.111"}, InstalledFiles: []string{"/boot/vmlinuz-6.8.0-111-generic"}},
			},
			running: []string{"linux-image-6.8.0-139-generic", "linux-modules-6.8.0-139-generic"},
		},
		{
			name:    "rpm",
			release: "6.12.0-211.61.1.el10_2.x86_64",
			report: []sbomtypes.PackageWithInstalledFiles{
				{Package: sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: "211.61.1.el10_2"}, InstalledFiles: []string{"/lib/modules/6.12.0-211.61.1.el10_2.x86_64/vmlinuz"}},
				{Package: sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: "55.9.1.el10_0"}, InstalledFiles: []string{"/lib/modules/6.12.0-55.9.1.el10_0.x86_64/vmlinuz"}},
			},
			running: []string{"kernel-core@211.61.1.el10_2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.sbomGenerations = atomic.NewUint64(0)
			r.failedSBOMGenerations = atomic.NewUint64(0)
			r.sbomCollector = &rootRecorder{report: tc.report}
			r.kernelRelease = func() (string, error) { return tc.release, nil }

			if err := r.scanHost(); err != nil {
				t.Fatalf("scanHost: %v", err)
			}

			for _, pkg := range r.hostSBOM.data.packages {
				want := slices.Contains(tc.running, pkg.Name) || slices.Contains(tc.running, pkg.Name+"@"+pkg.Release)
				if got := !pkg.LastAccess.IsZero() && pkg.AccessedByRoot; got != want {
					t.Errorf("%s %s run by root = %v, want %v", pkg.Name, pkg.Release, got, want)
				}
			}
		})
	}
}

// TestKernelFilesListsLoadedModules checks that the files of the running kernel
// hold its loaded modules, found through its modules.dep under either module
// directory, a module of weak-updates at the file it links to, and that
// marking them records the packages owning them, and those alone, as run by
// root.
func TestKernelFilesListsLoadedModules(t *testing.T) {
	const release = "6.12.0-211.61.1.el10_2.x86_64"
	const other = "6.12.0-55.9.1.el10_0.x86_64"
	modules := "/lib/modules/" + release

	for _, dir := range []string{"usr/lib/modules", "lib/modules"} {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			moduleDir := filepath.Join(root, dir, release)
			if err := os.MkdirAll(filepath.Join(moduleDir, "weak-updates"), 0755); err != nil {
				t.Fatal(err)
			}
			dep := "kernel/fs/xfs/xfs.ko.xz: kernel/lib/libcrc32c.ko.xz\n" +
				"kernel/lib/libcrc32c.ko.xz:\n" +
				"kernel/drivers/net/ena/ena.ko.xz:\n" +
				"weak-updates/nvidia-uvm.ko.xz:\n"
			if err := os.WriteFile(filepath.Join(moduleDir, "modules.dep"), []byte(dep), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../../"+other+"/extra/nvidia-uvm.ko.xz", filepath.Join(moduleDir, "weak-updates/nvidia-uvm.ko.xz")); err != nil {
				t.Fatal(err)
			}

			r := &Resolver{
				kernelRelease: func() (string, error) { return release, nil },
				loadedModules: func() ([]string, error) { return []string{"xfs", "libcrc32c", "nvidia_uvm", "vfat"}, nil },
			}
			d := newData([]sbomtypes.PackageWithInstalledFiles{
				{Package: sbomtypes.Package{Name: "kernel-core", Release: "211"}, InstalledFiles: []string{modules + "/vmlinuz"}},
				{Package: sbomtypes.Package{Name: "kernel-modules-core", Release: "211"}, InstalledFiles: []string{modules + "/kernel/fs/xfs/xfs.ko.xz", modules + "/kernel/lib/libcrc32c.ko.xz"}},
				{Package: sbomtypes.Package{Name: "kernel-modules", Release: "211"}, InstalledFiles: []string{modules + "/kernel/drivers/net/ena/ena.ko.xz"}},
				{Package: sbomtypes.Package{Name: "kmod-nvidia", Release: "565"}, InstalledFiles: []string{"/lib/modules/" + other + "/extra/nvidia-uvm.ko.xz"}},
				{Package: sbomtypes.Package{Name: "kernel-modules-core", Release: "55"}, InstalledFiles: []string{"/lib/modules/" + other + "/kernel/fs/xfs/xfs.ko.xz"}},
			}, false)
			markRunningKernel(d, r.kernelFiles(root))

			running := []string{"kernel-core@211", "kernel-modules-core@211", "kmod-nvidia@565"}
			for _, pkg := range d.packages {
				want := slices.Contains(running, pkg.Name+"@"+pkg.Release)
				if got := !pkg.LastAccess.IsZero() && pkg.AccessedByRoot; got != want {
					t.Errorf("%s %s run by root = %v, want %v", pkg.Name, pkg.Release, got, want)
				}
			}
		})
	}
}

// TestScanHostOnEmptyScan checks a rescan that finds no package. Over an index
// holding packages it fails, as a package scanner that fails finds none, and
// leaves the index as it was. Over the empty index of a host without package
// databases it succeeds. Either way, the next check leaves the host packages
// until their databases change.
func TestScanHostOnEmptyScan(t *testing.T) {
	tests := []struct {
		name    string
		indexed []sbomtypes.PackageWithInstalledFiles
		wantErr bool
	}{
		{
			name: "packages indexed",
			indexed: []sbomtypes.PackageWithInstalledFiles{{
				Package:        sbomtypes.Package{Name: "util-linux", Version: "2.40.4"},
				InstalledFiles: []string{"/usr/bin/su"},
			}},
			wantErr: true,
		},
		{name: "no package database"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.sbomGenerations = atomic.NewUint64(0)
			r.failedSBOMGenerations = atomic.NewUint64(0)
			r.sbomCollector = &rootRecorder{}
			r.hostSBOM.setReport(tt.indexed)
			data := r.hostSBOM.data

			err := r.scanHost()
			if (err != nil) != tt.wantErr {
				t.Errorf("scanHost() = %v, want an error: %t", err, tt.wantErr)
			}
			if tt.wantErr && r.hostSBOM.data != data {
				t.Errorf("the scan replaced the index with an empty one")
			}
			if r.hostChanged(time.Now()) {
				t.Errorf("the next check scans again, with the package databases unchanged")
			}
		})
	}
}

// fingerprinted is a package scanner finding the packages of report in a root
// whose package databases have fingerprint.
type fingerprinted struct {
	rootRecorder
	fingerprint string
}

func (s *fingerprinted) Fingerprint(string) string { return s.fingerprint }

// TestRescanHostOnCheck checks that a check of the host package databases scans
// the host packages again when the fingerprint of the databases changed since
// the last scan, or when that scan is an hour old, and leaves them otherwise.
// Usage enrichment alone relies on these checks to keep the host index current.
func TestRescanHostOnCheck(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		fingerprint string    // fingerprint of the databases the last scan read
		scanned     time.Time // time of the last scan
		wantScans   uint64
	}{
		{name: "unchanged", fingerprint: "dbs", scanned: now.Add(-time.Minute)},
		{name: "databases changed", fingerprint: "old", scanned: now.Add(-time.Minute), wantScans: 1},
		{name: "hour old", fingerprint: "dbs", scanned: now.Add(-hostRescanInterval), wantScans: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.sbomGenerations = atomic.NewUint64(0)
			r.failedSBOMGenerations = atomic.NewUint64(0)
			r.sbomCollector = &fingerprinted{
				rootRecorder: rootRecorder{report: []sbomtypes.PackageWithInstalledFiles{{
					Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
					InstalledFiles: []string{"/usr/bin/gzip"},
				}}},
				fingerprint: "dbs",
			}
			r.hostFingerprint, r.hostScanned = tt.fingerprint, tt.scanned

			ctx, cancel := context.WithCancel(context.Background())
			tick := make(chan time.Time)
			done := make(chan struct{})
			go func() {
				r.rescanHost(ctx, tick)
				close(done)
			}()
			tick <- now
			cancel()
			<-done

			if n := r.sbomGenerations.Load(); n != tt.wantScans {
				t.Errorf("%d scans of the host packages, want %d", n, tt.wantScans)
			}
			if r.hostFingerprint != "dbs" {
				t.Errorf("fingerprint = %q, want %q, that of the databases", r.hostFingerprint, "dbs")
			}
		})
	}
}

// writingScanner is a package scanner finding gzip in a root whose package
// databases change while it reads them.
type writingScanner struct {
	fingerprint string
}

func (s *writingScanner) Fingerprint(string) string { return s.fingerprint }

func (s *writingScanner) ScanInstalledPackages(context.Context, string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	s.fingerprint = "written"
	return []sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
		InstalledFiles: []string{"/usr/bin/gzip"},
	}}, nil
}

// TestScanHostFingerprintsBeforeScan checks that a scan of the host packages
// keeps the fingerprint the package databases had before it read them, so that
// a write while it reads them makes the next check scan again.
func TestScanHostFingerprintsBeforeScan(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)
	r.sbomCollector = &writingScanner{fingerprint: "read"}

	if err := r.scanHost(); err != nil {
		t.Fatalf("scanHost: %v", err)
	}
	if r.hostFingerprint != "read" {
		t.Errorf("fingerprint = %q, want %q, from before the scan read the databases", r.hostFingerprint, "read")
	}
	if !r.hostChanged(time.Now()) {
		t.Errorf("the check missed the write made while the scan read the databases")
	}
}

// TestScanHostCreditsMissedRuns checks that a scan finding a new package credits
// it the runs of its files by host processes made before the scan, in the last
// hostMissTTL, at the time of each run. Opens, processes on a root of their own
// and processes of containers leave it as it is, and the run of a file no
// package owns waits for a later scan.
func TestScanHostCreditsMissedRuns(t *testing.T) {
	tests := []struct {
		name      string
		path      string        // path of the file the process accesses
		open      bool          // the process opens the file instead of running it
		foreign   bool          // the process runs on a root of its own
		container bool          // the process runs in a container
		age       time.Duration // age of the run at the scan
		identity  bool          // the run carries the inode of a file under the host root
		replaced  bool          // a new file takes the path of the file before the scan
		credited  bool          // the scan credits the run to gzip
		kept      bool          // the run waits for a later scan
	}{
		{name: "run", path: "/usr/bin/gzip", credited: true},
		{name: "run of the file at its path", path: "/usr/bin/gzip", identity: true, credited: true},
		{name: "run of a file replaced since", path: "/usr/bin/gzip", identity: true, replaced: true},
		{name: "open", path: "/usr/bin/gzip", open: true},
		{name: "foreign root", path: "/usr/bin/gzip", foreign: true},
		{name: "container", path: "/usr/bin/gzip", container: true},
		{name: "expired", path: "/usr/bin/gzip", age: hostMissTTL + time.Minute},
		{name: "unowned", path: "/usr/local/bin/gzip", kept: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newHostSBOMResolver(t)
			r.sbomGenerations = atomic.NewUint64(0)
			r.failedSBOMGenerations = atomic.NewUint64(0)
			r.sbomCollector = &rootRecorder{report: []sbomtypes.PackageWithInstalledFiles{{
				Package:        sbomtypes.Package{Name: "util-linux", Version: "2.40.4"},
				InstalledFiles: []string{"/usr/bin/su"},
			}, {
				Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
				InstalledFiles: []string{"/usr/bin/gzip"},
			}}}

			pc := &model.ProcessContext{}
			pc.Pid = uint32(os.Getpid())
			pc.Process.FileEvent.SetPathnameStr(tt.path)
			pc.Process.FileEvent.Mode = 04755
			file := &pc.Process.FileEvent
			if tt.open {
				pc.Process.FileEvent.SetPathnameStr("/usr/bin/zcat")
				file = &model.FileEvent{}
				file.SetPathnameStr(tt.path)
				file.Mode = 04755
			}
			if tt.foreign {
				r.hostRootDevice, r.hostRootInode = tempRoot(t)
			}
			if tt.container {
				sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](maxSBOMEntries, nil)
				if err != nil {
					t.Fatalf("NewLRU: %v", err)
				}
				r.sboms = sboms
				sbom := r.newSBOM("container-id", nil, "image")
				t.Cleanup(sbom.stop)
				sbom.state.Store(computedState)
				pc.ContainerContext.ContainerID = "container-id"
			}

			hostPath := ""
			if tt.identity {
				r.hostRoot = t.TempDir()
				hostPath = filepath.Join(r.hostRoot, tt.path)
				if err := os.MkdirAll(filepath.Dir(hostPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(hostPath, nil, 0755); err != nil {
					t.Fatal(err)
				}
				stat, err := utils.UnixStat(hostPath)
				if err != nil {
					t.Fatal(err)
				}
				file.Inode = stat.Ino
			}

			before := time.Now()
			r.ResolvePackage(pc, file)
			after := time.Now()
			if tt.replaced {
				if err := os.WriteFile(hostPath+".new", nil, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(hostPath+".new", hostPath); err != nil {
					t.Fatal(err)
				}
			}
			if tt.age > 0 {
				miss, _ := r.hostMisses.Peek(tt.path)
				miss.usage.LastAccess = miss.usage.LastAccess.Add(-tt.age)
				r.hostMisses.Add(tt.path, miss)
			}
			r.hostRootDevice, r.hostRootInode = ownRoot(t)

			if err := r.scanHost(); err != nil {
				t.Fatalf("scanHost: %v", err)
			}

			gzip := r.hostSBOM.data.packages[1]
			if credited := !gzip.LastAccess.IsZero(); credited != tt.credited {
				t.Errorf("gzip = %+v, want the run credited: %t", gzip, tt.credited)
			}
			if tt.credited && (gzip.LastAccess.Before(before) || gzip.LastAccess.After(after) || !gzip.SuidBit || !gzip.AccessedByRoot) {
				t.Errorf("gzip = %+v, want the setuid run as root, between %v and %v", gzip, before, after)
			}
			if kept := r.hostMisses.Len() > 0; kept != tt.kept {
				t.Errorf("%d runs wait for a later scan, want some: %t", r.hostMisses.Len(), tt.kept)
			}
		})
	}
}

// blockingScanner is a package scanner that signals each scan it starts and
// finishes it, finding gzip, once release is closed.
type blockingScanner struct {
	noFingerprint
	started chan struct{}
	release chan struct{}
}

func (s *blockingScanner) ScanInstalledPackages(context.Context, string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	s.started <- struct{}{}
	<-s.release
	return []sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
		InstalledFiles: []string{"/usr/bin/gzip"},
	}}, nil
}

// TestScanHostHoldsScanLock checks that a scan of the host packages holds the
// scan lock from the read of the package databases to the swap of the index,
// so the scan that read last swaps last.
func TestScanHostHoldsScanLock(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)
	scanner := &blockingScanner{started: make(chan struct{}), release: make(chan struct{})}
	r.sbomCollector = scanner

	done := make(chan error)
	go func() {
		done <- r.scanHost()
	}()
	<-scanner.started

	if r.hostScanLock.TryLock() {
		r.hostScanLock.Unlock()
		t.Errorf("the scan lock was free while the scan read the package databases")
	}

	close(scanner.release)
	if err := <-done; err != nil {
		t.Fatalf("scanHost: %v", err)
	}
	if !r.hostScanLock.TryLock() {
		t.Fatalf("the scan kept the scan lock")
	}
	r.hostScanLock.Unlock()
}

// TestScanHostForwardsChangesAlone checks that a rescan finding the packages of
// the index leaves the forwarder idle. A forwarded report makes the next host
// scan of the core agent go out in full, which would end the heartbeats of an
// unchanged host every hour.
func TestScanHostForwardsChangesAlone(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)
	r.sbomCollector = &rootRecorder{report: []sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "util-linux", Version: "2.40.4"},
		InstalledFiles: []string{"/usr/bin/su", "/usr/share/doc/util-linux"},
	}}}

	if err := r.scanHost(); err != nil {
		t.Fatalf("scanHost: %v", err)
	}

	if r.hostSBOM.forwarder != nil {
		t.Errorf("the rescan of unchanged packages triggered forwarding")
	}
}

// TestProcessPendingFileEventsKeepsAccessTime checks that a queued access keeps
// the time it happened once the scan of its container ends.
func TestProcessPendingFileEventsKeepsAccessTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newPendingFileEventsResolver(t)
		r.cfg = &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Hour}

		sbom := NewSBOM("container-id", nil, "image:tag")
		t.Cleanup(sbom.stop)
		sbom.data = newData([]sbomtypes.PackageWithInstalledFiles{{
			Package:        sbomtypes.Package{Name: "bash"},
			InstalledFiles: []string{"/usr/bin/bash"},
		}}, false)

		r.queuePendingFileEvent("container-id", "/usr/bin/bash", 0755, 1000)
		queued := time.Now()
		time.Sleep(2 * time.Millisecond)
		sbom.Lock()
		r.processPendingFileEvents(sbom)
		sbom.Unlock()

		if last := sbom.data.packages[0].LastAccess; !last.Equal(queued) {
			t.Errorf("last access = %v, want the time of the access, %v", last, queued)
		}
	})
}

// TestProcessPendingFileEventsKeepsNewerAccess checks that a queued access
// leaves a later access another container of the image recorded as it is.
func TestProcessPendingFileEventsKeepsNewerAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newPendingFileEventsResolver(t)
		r.cfg = &config.RuntimeSecurityConfig{SBOMResolverForwardInterval: time.Hour}

		sbom := NewSBOM("container-id", nil, "image:tag")
		t.Cleanup(sbom.stop)
		sbom.data = newData([]sbomtypes.PackageWithInstalledFiles{{
			Package:        sbomtypes.Package{Name: "bash"},
			InstalledFiles: []string{"/usr/bin/bash"},
		}}, false)

		r.queuePendingFileEvent("container-id", "/usr/bin/bash", 0755, 1000)
		time.Sleep(2 * time.Millisecond)
		later := time.Now()
		sbom.data.packages[0].LastAccess = later

		sbom.Lock()
		r.processPendingFileEvents(sbom)
		sbom.Unlock()

		if last := sbom.data.packages[0].LastAccess; !last.Equal(later) {
			t.Errorf("last access = %v, want the later access, at %v", last, later)
		}
	})
}

// TestLatches checks which accesses latch a forward: the first, one after a gap,
// and one in steady use once the forwarded usage is maxForwardedAge old.
func TestLatches(t *testing.T) {
	r := &Resolver{cfg: &config.RuntimeSecurityConfig{SBOMResolverEnrichmentInterval: time.Minute}}
	now := time.Now()

	for _, tc := range []struct {
		name                string
		previous, forwarded time.Time
		want                bool
	}{
		{"first access", time.Time{}, time.Time{}, true},
		{"after a gap", now.Add(-2 * time.Minute), now.Add(-2 * time.Minute), true},
		{"steady use", now.Add(-time.Second), now.Add(-time.Minute), false},
		{"steady use with old forwarded usage", now.Add(-time.Second), now.Add(-maxForwardedAge - time.Second), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.latches(&Data{forwardedAt: tc.forwarded}, tc.previous, now); got != tc.want {
				t.Errorf("latches = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLatchesClaimsTheHour checks that, of the containers sharing data in steady
// use, the first to access it once the forwarded usage is old latches a forward.
func TestLatchesClaimsTheHour(t *testing.T) {
	r := &Resolver{cfg: &config.RuntimeSecurityConfig{SBOMResolverEnrichmentInterval: time.Minute}}
	now := time.Now()
	shared := &Data{forwardedAt: now.Add(-maxForwardedAge - time.Second)}

	if !r.latches(shared, now.Add(-time.Second), now) {
		t.Fatalf("the first access past the hour latched no forward")
	}
	if r.latches(shared, now, now.Add(time.Second)) {
		t.Errorf("a second container latched its own forward of the shared data")
	}
}

// TestResolvePackageRefreshesSteadyUsage checks that a package in steady use goes
// out again once its forwarded usage is maxForwardedAge old.
func TestResolvePackageRefreshesSteadyUsage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newPendingFileEventsResolver(t)
		r.Notifier = utils.NewNotifier[Event, *sbompkg.ScanResult]()
		r.cfg = &config.RuntimeSecurityConfig{
			SBOMResolverEnrichmentInterval: time.Minute,
			SBOMResolverForwardInterval:    10 * time.Millisecond,
		}
		reports := make(chan *sbompkg.ScanResult, 1)
		if err := r.RegisterListener(SBOMComputed, func(result *sbompkg.ScanResult) {
			select {
			case reports <- result:
			default:
			}
		}); err != nil {
			t.Fatalf("RegisterListener: %v", err)
		}

		sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](1, nil)
		if err != nil {
			t.Fatalf("NewLRU: %v", err)
		}
		r.sboms = sboms
		sbom := NewSBOM("container-id", nil, "image:tag")
		sbom.setReport([]sbomtypes.PackageWithInstalledFiles{{
			Package:        sbomtypes.Package{Name: "bash"},
			InstalledFiles: []string{"/usr/bin/bash"},
		}})
		sbom.status = workloadmeta.Success
		sbom.state.Store(computedState)
		t.Cleanup(sbom.stop)
		sboms.Add(sbom.ContainerID, sbom)

		now := time.Now()
		sbom.data.packages[0].LastAccess = now.Add(-time.Second)
		sbom.data.forwardedAt = now.Add(-maxForwardedAge - time.Minute)

		pc := &model.ProcessContext{}
		pc.ContainerContext.ContainerID = sbom.ContainerID
		pc.UID, pc.EUID = 1000, 1000
		file := &model.FileEvent{}
		file.SetPathnameStr("/usr/bin/bash")
		r.ResolvePackage(pc, file)

		select {
		case <-reports:
		case <-time.After(10 * time.Second):
			t.Errorf("the steady usage was not forwarded")
		}
	})
}
