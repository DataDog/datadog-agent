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
	"sync"
	"syscall"
	"testing"
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
	sbomB.status = workloadmeta.Success
	sbomB.state.Store(computedState)

	r := newPendingFileEventsResolver(t)
	r.Notifier = utils.NewNotifier[Event, *sbompkg.ScanResult]()

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
// debouncers are busy. A debouncer takes a Stop only between two runs of its
// callback, and the callbacks take the locks the callers of stop hold, or stop
// the SBOM themselves, as the refresh does when it drops the SBOM of a workload
// it fails to queue for a scan, so waiting for them deadlocked the resolver. A
// debouncer started only after stop returns stands for a busy one.
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
// SBOM, which the callback stop leaves running may still hand it.
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

// TestRefreshScanKeepsStoppedSBOM checks that the refresh of a deleted
// workload, which the callback stop leaves running may still make, keeps its
// SBOM stopped and out of the scan queue, and the cached data of its image for
// the other containers of the image.
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
}

// rootRecorder is a package scanner that records the root it was given and
// finds the packages of report there.
type rootRecorder struct {
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
// packages again, in place, and keeps the usage of the packages it finds again,
// upgraded ones included: an upgrade restarts a daemon before its package
// manager exits, which records the start on the build the upgrade replaces. The
// re-scan of a workload reads its root from the cgroup of the workload, nil for
// the host SBOM, so the host stays out of the queue of workload scans and of
// their cache.
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

// TestKeepUsageCombinesBuilds checks that a package name listed twice, once for
// each architecture or for builds installed side by side, keeps the usage of
// both.
func TestKeepUsageCombinesBuilds(t *testing.T) {
	seen := time.Unix(1700000000, 0)

	ran := sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: "55.el10"}
	ran.LastAccess, ran.AccessedByRoot = seen, true
	setuid := sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: "53.el10"}
	setuid.LastAccess, setuid.SuidBit = seen.Add(-time.Minute), true
	prev := &Data{packages: []sbomtypes.Package{ran, setuid}}

	d := newData([]sbomtypes.PackageWithInstalledFiles{
		{Package: sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: "55.el10"}},
		{Package: sbomtypes.Package{Name: "kernel-core", Version: "6.12.0", Release: "57.el10"}},
	}, false)
	d.keepUsage(prev)

	for _, pkg := range d.packages {
		if !pkg.LastAccess.Equal(seen) || !pkg.SuidBit || !pkg.AccessedByRoot {
			t.Errorf("package = %+v, want the usage of both builds", pkg)
		}
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

// TestScanHostKeepsIndexOnEmptyScan checks that a rescan that finds no package,
// as a package scanner that fails returns, leaves the index of the host and the
// usage recorded on it as they were.
func TestScanHostKeepsIndexOnEmptyScan(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)
	r.sbomCollector = &rootRecorder{}
	data := r.hostSBOM.data

	if err := r.scanHost(); err == nil {
		t.Errorf("scanHost succeeded with no package found on the host")
	}
	if r.hostSBOM.data != data {
		t.Errorf("the scan replaced the index with an empty one")
	}
}

// TestRescanHostOnTick checks that the host packages are scanned again on every
// tick until the context is done. Usage enrichment alone relies on these scans
// to keep the host index current.
func TestRescanHostOnTick(t *testing.T) {
	r := newHostSBOMResolver(t)
	r.sbomGenerations = atomic.NewUint64(0)
	r.failedSBOMGenerations = atomic.NewUint64(0)
	r.sbomCollector = &rootRecorder{report: []sbomtypes.PackageWithInstalledFiles{{
		Package:        sbomtypes.Package{Name: "gzip", Version: "1.13"},
		InstalledFiles: []string{"/usr/bin/gzip"},
	}}}

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		r.rescanHost(ctx, tick)
		close(done)
	}()

	// The second tick goes through once the scan of the first is over.
	tick <- time.Now()
	tick <- time.Now()
	cancel()
	<-done

	if n := r.sbomGenerations.Load(); n != 2 {
		t.Errorf("%d scans of the host packages, want 2", n)
	}
	if pkgs := r.hostSBOM.data.packages; len(pkgs) != 1 || pkgs[0].Name != "gzip" {
		t.Errorf("host packages = %+v, want gzip, from the new scans", pkgs)
	}
}

// blockingScanner is a package scanner that signals each scan it starts and
// finishes it, finding gzip, once release is closed.
type blockingScanner struct {
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
// so that of an hourly rescan and a rescan a database write triggers, the one
// that read the databases last swaps its index last.
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

// TestScanHostForwardsChangesAlone checks that a rescan of the host packages
// that finds the packages of the index forwards nothing. A forwarded report
// makes the next host scan of the core agent go out in full, so the hourly
// rescan of an unchanged host would end its heartbeats.
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
