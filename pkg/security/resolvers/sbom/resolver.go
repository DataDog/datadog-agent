// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sbom holds sbom related files
package sbom

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cenkalti/backoff/v7"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/samber/lo"
	"github.com/skydive-project/go-debouncer"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/collectorv2"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	// state of the sboms
	pendingState int64 = iota + 1
	computedState
	stoppedState

	maxSBOMGenerationRetries = 3
	maxSBOMEntries           = 1024
	// scanQueueSize lets every SBOM entry wait for its scan.
	scanQueueSize        = maxSBOMEntries
	maxPendingFileEvents = 2048
	// rootReadInterval spaces out the reads of the root mount of a container
	// while it is unknown, as for a container that still runs on the host root.
	rootReadInterval = 10 * time.Second
	// maxForwardWait bounds how long forwarding keeps retrying while the image's
	// Trivy SBOM is still pending. It must outlast a slow overlayfs scan, yet stop
	// re-arming when no image SBOM will ever be produced (for example when
	// container image SBOM collection is disabled and the image stays pending).
	maxForwardWait = 30 * time.Minute
	// maxForwardedAge bounds the age of the usage the core agent holds for a
	// package in steady use, at the period of the SBOM check.
	maxForwardedAge = time.Hour
	// hostRescanInterval is the longest time between two scans of the host
	// packages, the default period of the host scans of the core agent.
	hostRescanInterval = time.Hour
	// settleDelay is how long the open of a host file waits before it counts
	// as usage. rpm opens each executable it removes or replaces, to clear its
	// file capabilities, right before it deletes the file or renames the new
	// one over it, which a busy disk can hold up for a while.
	settleDelay = 3 * time.Second
	// maxOpened bounds the opens of host files waiting for settleDelay.
	maxOpened = 4096
	// hostCheckInterval is the period of the checks of the host package
	// databases, which scan the host packages again once the databases changed.
	hostCheckInterval = time.Minute
	// maxHostMisses bounds the runs of host files the host index lacked that
	// wait for the scan indexing them, and hostMissTTL how long one waits.
	maxHostMisses = 1024
	hostMissTTL   = 10 * time.Minute
	// deletedSuffix ends the path procfs gives a file removed from its path
	// while a process runs or maps it, as when an upgrade renames a new build
	// over it.
	deletedSuffix = " (deleted)"
)

// hostMiss is the run of a host file the host index lacked: the usage it
// records, and the inode of the file that ran, zero when unknown.
type hostMiss struct {
	usage sbomtypes.Package
	inode uint64
}

// pendingFileEvent holds the accesses to a file made before the SBOM of its container
// was ready: the keys of the file, the latest time and the sticky properties.
type pendingFileEvent struct {
	keys           fileKeys
	lastAccess     int64 // Unix nanoseconds, which keep the map free of pointers
	suidBit        bool
	accessedByRoot bool
}

// openedFile is an open of a host file waiting for settleDelay, with the
// fields of the opening process that usage reads.
type openedFile struct {
	pid  uint32
	cred model.Credentials
	comm string
	file model.FileEvent
	at   time.Time
}

// pendingFileKey identifies the queued accesses to a file by the hash of its
// path and its mount, so that a bind mount at a package path stands apart.
type pendingFileKey struct {
	path    uint64
	mountID uint32
}

var errNoProcessForContainerID = errors.New("found no running process matching the given container ID")

// Event defines the SBOM event type
type Event int

const (
	// SBOMComputed is used to notify that a SBOM was computed
	SBOMComputed Event = iota + 1
)

// Data use the keep the result of a scan of a same workload across multiple
// container. A *Data is shared across SBOMs that run the same image (via the
// dataCache), so it needs its own lock rather than relying on each SBOM's lock.
type Data struct {
	mu            sync.RWMutex
	files         fileQuerier
	packages      []sbomtypes.Package // per-package metadata (without the plain-text installed-file lists) kept for forwarding
	forwardedAt   time.Time           // when a forward last claimed or copied the packages
	observedSince time.Time           // when the usage of packages started being recorded
}

// newData builds the cached scan Data from a freshly generated report. It keeps
// only the compact representation: per-package metadata in packages (dropping the
// plain-text InstalledFiles) and murmur3 hashes of the file paths in the file
// querier. Runtime file->package lookups use the hashes and forwarding only needs
// the package metadata, so retaining the paths would waste megabytes per workload
// for the lifetime of the data cache. The file querier stores pointers into
// packages so LastAccess updates stay visible to the forwarding path.
func newData(report []sbomtypes.PackageWithInstalledFiles, usrMerged bool) *Data {
	packages := make([]sbomtypes.Package, len(report))
	for i := range report {
		packages[i] = report[i].Package
	}

	return &Data{
		files:         newFileQuerier(report, packages, usrMerged),
		packages:      packages,
		observedSince: time.Now(),
	}
}

// holds reports whether d holds the packages report lists, and those alone, in
// any order.
func (d *Data) holds(report []sbomtypes.PackageWithInstalledFiles) bool {
	if len(report) != len(d.packages) {
		return false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	builds := make(map[sbomtypes.Package]int, len(d.packages))
	for _, pkg := range d.packages {
		builds[packageBuild(pkg)]++
	}
	for _, pkg := range report {
		b := packageBuild(pkg.Package)
		if builds[b] == 0 {
			return false
		}
		builds[b]--
	}
	return true
}

// keepUsage copies onto the packages of d the usage recorded in prev, and the
// times it started being recorded and was last forwarded. A package keeps the
// usage of its build, combined with that of the builds of its name and
// architecture only prev holds, the builds an upgrade replaced, when d holds a
// single build of them, or of its name across a change of architecture. Kernels
// install side by side, and a removed kernel keeps its usage.
func (d *Data) keepUsage(prev *Data) {
	held := make(map[sbomtypes.Package]bool, len(d.packages))
	builds := make(map[packageLine]int)
	names := make(map[string]int)
	for _, pkg := range d.packages {
		if b := packageBuild(pkg); !held[b] {
			held[b] = true
			builds[lineOf(pkg)]++
			names[pkg.Name]++
		}
	}

	prev.mu.RLock()
	if !prev.observedSince.IsZero() {
		d.observedSince = prev.observedSince
	}
	d.forwardedAt = prev.forwardedAt
	kept := make(map[sbomtypes.Package]sbomtypes.Package, len(prev.packages))
	replaced := make(map[packageLine]sbomtypes.Package)
	replacedNames := make(map[string]sbomtypes.Package)
	for _, pkg := range prev.packages {
		if b := packageBuild(pkg); held[b] {
			kept[b] = combineUsage(kept[b], pkg)
		} else {
			replaced[lineOf(pkg)] = combineUsage(replaced[lineOf(pkg)], pkg)
			replacedNames[pkg.Name] = combineUsage(replacedNames[pkg.Name], pkg)
		}
	}
	prev.mu.RUnlock()

	for i := range d.packages {
		pkg := &d.packages[i]
		usage := kept[packageBuild(*pkg)]
		if line := lineOf(*pkg); builds[line] == 1 {
			if r, ok := replaced[line]; ok {
				usage = combineUsage(usage, r)
			} else if names[pkg.Name] == 1 {
				usage = combineUsage(usage, replacedNames[pkg.Name])
			}
		}
		pkg.LastAccess, pkg.SuidBit, pkg.AccessedByRoot = usage.LastAccess, usage.SuidBit, usage.AccessedByRoot
	}
}

// combineUsage returns usage combined with the usage recorded on pkg: the
// later of their last accesses, and each flag either one holds.
func combineUsage(usage, pkg sbomtypes.Package) sbomtypes.Package {
	if pkg.LastAccess.After(usage.LastAccess) {
		usage.LastAccess = pkg.LastAccess
	}
	usage.SuidBit = usage.SuidBit || pkg.SuidBit
	usage.AccessedByRoot = usage.AccessedByRoot || pkg.AccessedByRoot
	return usage
}

// packageLine names the builds of a package that replace one another. rpm
// installs the builds of several architectures side by side.
type packageLine struct {
	name, arch string
}

func lineOf(pkg sbomtypes.Package) packageLine {
	return packageLine{pkg.Name, pkg.Arch}
}

// packageBuild returns pkg stripped of its usage, which leaves the fields
// naming the build of the package.
func packageBuild(pkg sbomtypes.Package) sbomtypes.Package {
	pkg.LastAccess, pkg.SuidBit, pkg.AccessedByRoot = time.Time{}, false, false
	return pkg
}

// SBOM defines an SBOM
type SBOM struct {
	sync.RWMutex

	ContainerID containerutils.ContainerID

	data        *Data
	cached      bool   // data counts as a use of its key in the data cache
	fingerprint string // fingerprint of the package databases data was scanned from

	workloadKey workloadKey
	status      workloadmeta.SBOMStatus

	cgroup *cgroupModel.CacheEntry
	state  *atomic.Int64

	refresher         *debouncer.Debouncer
	forwarder         *debouncer.Debouncer // Debouncer for forwarding SBOM updates
	forwardRetryCount int

	usrMerged bool

	root     containerRoot
	rootRead time.Time // when root was last read, while unknown
}

// containerRoot is the root mount of a container and the mounts of its mount
// namespace, zero when unknown.
type containerRoot struct {
	mountID uint32
	mounts  []uint32
}

// onRoot reports whether a file on mountID may be on the root, on any mount but the
// other ones of its namespace. Another namespace copies the root under a new ID.
func (c containerRoot) onRoot(mountID uint32) bool {
	return c.mountID == 0 || mountID == 0 || mountID == c.mountID || !slices.Contains(c.mounts, mountID)
}

type workloadKey string

// dataKey returns the data cache key of the scan of a workload of key, from a
// root whose package databases have fingerprint.
func dataKey(key workloadKey, fingerprint string) workloadKey {
	if fingerprint == "" {
		return key
	}
	return key + "@" + workloadKey(fingerprint)
}

// IsComputed returns true if SBOM was successfully generated
func (s *SBOM) IsComputed() bool {
	return s.state.Load() == computedState
}

// SetReport sets the SBOM report
func (s *SBOM) setReport(pkgs []sbomtypes.PackageWithInstalledFiles) {
	// build the compact file cache and package metadata, dropping installed-file lists
	s.data = newData(pkgs, s.usrMerged)
}

func (s *SBOM) stop() {
	// mark the SBOM stopped, so that a queued scan skips it and the callbacks
	// of its debouncers return at once
	s.state.Store(stoppedState)

	// Stopping a debouncer waits for its running callback, and the callbacks
	// take the locks the callers of stop hold, so the debouncers are left to
	// stop once these are released.
	if s.refresher != nil {
		go s.refresher.Stop()

		// don't forget to set the refresher to nil otherwise it generates a memleak
		s.refresher = nil
	}

	if s.forwarder != nil {
		go s.forwarder.Stop()
		s.forwarder = nil
	}
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry, workloadKey workloadKey) *SBOM {
	return &SBOM{
		ContainerID: id,
		workloadKey: workloadKey,
		state:       atomic.NewInt64(pendingState),
		cgroup:      cgroup,
		data:        &Data{},
	}
}

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	*utils.Notifier[Event, *sbompkg.ScanResult]

	cfg *config.RuntimeSecurityConfig

	sbomsLock sync.RWMutex
	sboms     *simplelru.LRU[containerutils.ContainerID, *SBOM]

	// cache
	dataCache *dataCache // scan data per workload key

	// queue
	scanChan        chan *SBOM
	pendingScanLock sync.Mutex
	pendingScan     []containerutils.ContainerID

	// pending file events: file accesses received before the SBOM was ready, by pendingFileKey
	pendingFileEventsLock sync.Mutex
	pendingFileEvents     *simplelru.LRU[containerutils.ContainerID, map[pendingFileKey]pendingFileEvent]

	statsdClient   statsd.ClientInterface
	sbomCollector  sbomCollector
	hostRoot       string
	hostRootDevice uint64
	hostRootInode  uint64
	// hostRootOverlay is set when the host root is an overlayfs.
	hostRootOverlay bool
	hostSBOM        *SBOM
	// hostScanLock serializes the scans of the host packages. A scan reads the
	// databases before it swaps the index, so the scan that read them last
	// swaps last. It guards hostFingerprint, the fingerprint of the databases
	// the last scan read, and hostScanned, the time of that scan.
	hostScanLock    sync.Mutex
	hostFingerprint string
	hostScanned     time.Time
	// hostMisses holds the runs of the host files the host index lacked, by
	// path, for the scan indexing them. The lock of hostSBOM guards it.
	hostMisses *simplelru.LRU[string, hostMiss]
	// walkProcesses calls its argument on every process the probe knows of.
	walkProcesses func(func(*model.ProcessCacheEntry))
	// openedLock guards opened, the opens of host files waiting for
	// settleDelay, oldest first.
	openedLock sync.Mutex
	opened     []openedFile
	// kernelRelease returns the release of the running kernel, and
	// loadedModules the names of its loaded modules.
	kernelRelease func() (string, error)
	loadedModules func() ([]string, error)
	// scanWorkload reads the packages installed in the root of a workload.
	scanWorkload func(*SBOM) ([]sbomtypes.PackageWithInstalledFiles, error)

	sbomGenerations       *atomic.Uint64
	failedSBOMGenerations *atomic.Uint64
	sbomsCacheHit         *atomic.Uint64
	sbomsCacheMiss        *atomic.Uint64

	enrichedSBOMForwarded      *atomic.Uint64
	enrichedSBOMForwardDropped *atomic.Uint64

	wmeta workloadmeta.Component

	// Callback for when SBOM policies should be generated
	policyGeneratorCb func(workloadKey string, containerID containerutils.ContainerID, policyDef *rules.PolicyDef)
	policyGenLock     sync.RWMutex
}

type sbomCollector interface {
	ScanInstalledPackages(ctx context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error)
	Fingerprint(root string) string
}

// NewSBOMResolver returns a new instance of Resolver
func NewSBOMResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, wmeta workloadmeta.Component) (*Resolver, error) {
	sbomCollector := collectorv2.NewOSScanner()
	dataCache, err := newDataCache(c.SBOMResolverWorkloadsCacheSize)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	// one entry per workload waiting for its scan, so the same bound as the sboms cache
	pendingFileEvents, err := simplelru.NewLRU[containerutils.ContainerID, map[pendingFileKey]pendingFileEvent](maxSBOMEntries, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	hostMisses, err := simplelru.NewLRU[string, hostMiss](maxHostMisses, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	hostProcRootPath := utils.ProcRootPath(1)
	stat, err := utils.UnixStat(hostProcRootPath)
	if err != nil {
		return nil, fmt.Errorf("stat failed for `%s`: couldn't stat host proc root path: %w", hostProcRootPath, err)
	}
	var fsStat unix.Statfs_t
	if err := unix.Statfs(hostProcRootPath, &fsStat); err != nil {
		return nil, fmt.Errorf("statfs failed for `%s`: %w", hostProcRootPath, err)
	}

	resolver := &Resolver{
		Notifier:              utils.NewNotifier[Event, *sbompkg.ScanResult](),
		cfg:                   c,
		statsdClient:          statsdClient,
		dataCache:             dataCache,
		scanChan:              make(chan *SBOM, scanQueueSize),
		sbomCollector:         sbomCollector,
		hostRoot:              hostProcRootPath,
		hostRootDevice:        stat.Dev,
		hostRootInode:         stat.Ino,
		hostRootOverlay:       fsStat.Type == unix.OVERLAYFS_SUPER_MAGIC,
		kernelRelease:         kernel.Release,
		loadedModules:         liveModules,
		sbomGenerations:       atomic.NewUint64(0),
		sbomsCacheHit:         atomic.NewUint64(0),
		sbomsCacheMiss:        atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
		wmeta:                 wmeta,
		pendingFileEvents:     pendingFileEvents,
		hostMisses:            hostMisses,

		enrichedSBOMForwarded:      atomic.NewUint64(0),
		enrichedSBOMForwardDropped: atomic.NewUint64(0),
	}

	sboms, err := simplelru.NewLRU(maxSBOMEntries, resolver.onSBOMEvicted)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOM resolver: %w", err)
	}
	resolver.sboms = sboms
	resolver.scanWorkload = resolver.doScan

	if !c.SBOMResolverEnabled {
		return resolver, nil
	}

	return resolver, nil
}

// Start starts the goroutine of the SBOM resolver
func (r *Resolver) Start(ctx context.Context) error {
	if r.cfg.SBOMResolverHostEnabled {
		r.hostSBOM = NewSBOM("", nil, "")
		if err := r.scanHost(); err != nil {
			return err
		}

		check := time.NewTicker(hostCheckInterval)
		go func() {
			defer check.Stop()
			r.rescanHost(ctx, check.C)
		}()

		settle := time.NewTicker(settleDelay / 2)
		go func() {
			defer settle.Stop()
			r.settleOpens(ctx, settle.C)
		}()
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case sbom := <-r.scanChan:
				if _, err := backoff.Retry(ctx, func() (struct{}, error) {
					return struct{}{}, r.analyzeWorkload(sbom)
				}, backoff.WithMaxTries(maxSBOMGenerationRetries), backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond))); err != nil {
					if errors.Is(err, errNoProcessForContainerID) {
						seclog.Debugf("Couldn't generate SBOM for '%s': %v", sbom.ContainerID, err)
					} else {
						seclog.Warnf("Failed to generate SBOM for '%s': %v", sbom.ContainerID, err)
					}
				}
			}
		}
	}()

	return nil
}

// RefreshSBOM regenerates the SBOM of a container, or the SBOM of the host for
// the empty container ID.
func (r *Resolver) RefreshSBOM(containerID containerutils.ContainerID) error {
	if sbom := r.getSBOM(containerID); sbom != nil {
		seclog.Debugf("Refreshing SBOM for container %s", containerID)

		var refresher *debouncer.Debouncer

		// create a refresher debouncer on demand
		sbom.Lock()
		if sbom.state.Load() == stoppedState {
			sbom.Unlock()
			return nil
		}
		refresher = sbom.refresher
		if refresher == nil {
			refresher = debouncer.New(
				r.cfg.SBOMResolverRefreshInterval, func() {
					r.refreshScan(sbom)
				},
			)
			refresher.Start()
			sbom.refresher = refresher
		}
		sbom.Unlock()

		refresher.Call()

		return nil
	}
	return fmt.Errorf("container %s not found", containerID)
}

// refreshScan scans a workload again, its state reset to pending as analyzeWorkload
// scans pending SBOMs alone. The host SBOM is scanned again in place.
func (r *Resolver) refreshScan(sbom *SBOM) {
	if sbom.ContainerID == "" {
		if err := r.scanHost(); err != nil {
			seclog.Warnf("Failed to refresh the host SBOM: %v", err)
		}
		return
	}

	r.rescan(sbom, r.rootFingerprint(sbom))
}

// rescan queues sbom for a scan of its root, whose databases have fingerprint. The old
// scan stays cached for the image's other containers when the databases changed.
func (r *Resolver) rescan(sbom *SBOM, fingerprint string) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.state.Load() == stoppedState {
		return
	}

	if fingerprint == "" || fingerprint == sbom.fingerprint {
		r.removeSBOMData(dataKey(sbom.workloadKey, sbom.fingerprint))
	}
	sbom.state.Store(pendingState)
	r.triggerScan(sbom)
}

// rescanHost checks the host package databases on every tick until ctx is
// done, and scans the host packages again when they changed, so the host index
// follows the packages installed after it was built. With CWS on, a write to
// the package databases rescans the host at once as well. Usage enrichment
// alone relies on these checks.
func (r *Resolver) rescanHost(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick:
			if !r.hostChanged(now) {
				continue
			}
			if err := r.scanHost(); err != nil {
				seclog.Warnf("Failed to rescan the host packages: %v", err)
			}
		}
	}
}

// hostChanged reports whether the host packages may have changed at now since
// the last scan: the fingerprint of their databases changed, or the scan is
// hostRescanInterval old.
func (r *Resolver) hostChanged(now time.Time) bool {
	r.hostScanLock.Lock()
	defer r.hostScanLock.Unlock()

	return now.Sub(r.hostScanned) >= hostRescanInterval ||
		r.sbomCollector.Fingerprint(utils.ProcRootPath(1)) != r.hostFingerprint
}

// scanHost indexes the packages installed on the host into the host SBOM, and
// forwards the result when the packages changed. A package keeps the usage the
// previous index recorded on its build, and on the builds of its name the scan
// no longer finds, so a daemon seen running before the scan still counts, and
// so does a daemon its upgrade restarted, whose start was recorded on the
// build the upgrade replaced. The runs of the files of a new package made
// before the scan count as well. The host resolves against the previous index
// until the scan is over.
func (r *Resolver) scanHost() error {
	r.hostScanLock.Lock()
	defer r.hostScanLock.Unlock()

	// The host packages are read through the root of init, as those of a
	// container are read through the root of one of its processes.
	hostRoot := utils.ProcRootPath(1)

	// The databases are fingerprinted before the scan reads them, so that a
	// write while it reads them makes the next check scan again. A scan that
	// fails waits for the next write, or for hostRescanInterval.
	r.hostFingerprint, r.hostScanned = r.sbomCollector.Fingerprint(hostRoot), time.Now()
	report, err := r.generateSBOM(hostRoot)
	if err != nil {
		return err
	}
	kernelFiles := r.kernelFiles(hostRoot)

	r.hostSBOM.Lock()
	rescan := r.hostSBOM.IsComputed()
	previous := r.hostSBOM.data
	// A package scanner that fails finds no package, and the index then
	// still holds the packages of the host.
	if len(report) == 0 && len(previous.packages) > 0 {
		r.hostSBOM.Unlock()
		return errors.New("no package found on the host")
	}
	// A change of packages alone is forwarded: a report makes the next host
	// scan of the core agent go out in full, and the hourly rescan of an
	// unchanged host keeps its heartbeats.
	changed := !rescan || !previous.holds(report)
	r.hostSBOM.usrMerged = isUsrMerged(hostRoot)
	r.hostSBOM.setReport(report)
	r.hostSBOM.data.keepUsage(previous)
	markRunningKernel(r.hostSBOM.data, kernelFiles)
	r.hostSBOM.state.Store(computedState)
	if changed {
		r.creditHostMisses()
		r.triggerForwarding(r.hostSBOM)
	}
	r.hostSBOM.Unlock()

	// A package manager runs the scripts of a package before it exits, so
	// the daemon of a new package may start before the scan, when its files
	// have no entry in the index. The processes running on the host are then
	// recorded against the new index. At start the probe replays them all.
	if rescan && changed && r.walkProcesses != nil {
		r.walkProcesses(func(entry *model.ProcessCacheEntry) {
			if entry.ContainerContext.ContainerID == "" && entry.ExitTime.IsZero() {
				r.ResolvePackage(&entry.ProcessContext, &entry.FileEvent)
			}
		})
	}

	return nil
}

// kernelFiles returns the paths of the files of the running kernel, those of
// the host packages under root that run outside of any process: its image,
// the list of the modules built into it, and its loaded modules.
func (r *Resolver) kernelFiles(root string) []string {
	if r.kernelRelease == nil {
		return nil
	}
	release, err := r.kernelRelease()
	if err != nil {
		seclog.Debugf("Failed to read the release of the running kernel: %v", err)
		return nil
	}
	dir := "/lib/modules/" + release
	files := []string{"/boot/vmlinuz-" + release, dir + "/vmlinuz", dir + "/modules.builtin"}
	if r.loadedModules == nil {
		return files
	}

	loaded, err := r.loadedModules()
	if err != nil {
		seclog.Debugf("Failed to list the loaded kernel modules: %v", err)
		return files
	}
	paths, err := modulePaths(root, release)
	if err != nil {
		seclog.Debugf("Failed to read the modules of kernel %s: %v", release, err)
		return files
	}
	for _, name := range loaded {
		if path, ok := paths[name]; ok {
			files = append(files, path)
		}
	}
	return files
}

// modulePaths returns the paths of the modules of kernel release under root,
// by module name, from its modules.dep. The modules of weak-updates link to
// the modules of another kernel, whose package owns them.
func modulePaths(root, release string) (map[string]string, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootFS.Close()

	var dir string
	var dep []byte
	for _, dir = range []string{"usr/lib/modules/" + release, "lib/modules/" + release} {
		if dep, err = rootFS.ReadFile(dir + "/modules.dep"); err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	paths := make(map[string]string)
	for line := range strings.Lines(string(dep)) {
		module, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		path := module
		if !filepath.IsAbs(path) {
			path = "/lib/modules/" + release + "/" + module
			if strings.HasPrefix(module, "weak-updates/") {
				if target, err := rootFS.Readlink(dir + "/" + module); err == nil {
					if !filepath.IsAbs(target) {
						target = filepath.Join(filepath.Dir(path), target)
					}
					path = target
				}
			}
		}
		paths[moduleName(module)] = path
	}
	return paths, nil
}

// moduleName returns the name of the kernel module in the file at path, as
// /proc/modules lists it.
func moduleName(path string) string {
	name := filepath.Base(path)
	if i := strings.Index(name, ".ko"); i >= 0 {
		name = name[:i]
	}
	return strings.ReplaceAll(name, "-", "_")
}

// liveModules returns the names of the loaded kernel modules.
func liveModules() ([]string, error) {
	modules, err := utils.FetchLoadedModules()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(modules))
	for name, module := range modules {
		if module.State == "Live" {
			names = append(names, name)
		}
	}
	return names, nil
}

// markRunningKernel records as run by root the packages of d that own files,
// the files of the running kernel. The kernel runs outside of the processes
// whose execs and opens record usage.
func markRunningKernel(d *Data, files []string) {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, path := range files {
		if pkg := d.files.queryFile(path); pkg != nil {
			pkg.LastAccess = now
			pkg.AccessedByRoot = true
		}
	}
}

// addHostMiss keeps the run of file by the process of pc, a host file the host
// index lacks, for the scan indexing it. Must be called with hostSBOM.Lock()
// already held.
func (r *Resolver) addHostMiss(pc *model.ProcessContext, file *model.FileEvent) {
	if !r.onHostRoot(pc.Pid) {
		return
	}
	// An overlayfs file carries the inode of its layer, which can differ from
	// the inode stat reports.
	inode := file.Inode
	if file.IsInLowerLayer() || file.IsInUpperLayer() {
		inode = 0
	}
	run := sbomtypes.Package{
		LastAccess:     time.Now(),
		SuidBit:        fs.FileMode(file.Mode)&04000 != 0,
		AccessedByRoot: runsAsRoot(pc),
	}
	if miss, ok := r.hostMisses.Peek(file.PathnameStr); ok && miss.inode == inode {
		run = combineUsage(miss.usage, run)
	}
	r.hostMisses.Add(file.PathnameStr, hostMiss{usage: run, inode: inode})
}

// creditHostMisses records the runs of host files the host index lacked, made
// in the last hostMissTTL, on the packages of the new index that own the files,
// and forgets them. A run counts if the file that ran is still at its path.
// Must be called with hostSBOM.Lock() already held.
func (r *Resolver) creditHostMisses() {
	data := r.hostSBOM.data
	data.mu.Lock()
	defer data.mu.Unlock()

	now := time.Now()
	for _, path := range r.hostMisses.Keys() {
		miss, _ := r.hostMisses.Peek(path)
		if now.Sub(miss.usage.LastAccess) > hostMissTTL {
			r.hostMisses.Remove(path)
			continue
		}
		pkg := data.files.queryFile(path)
		if pkg == nil {
			continue
		}
		if r.isFileAt(path, miss.inode) {
			*pkg = combineUsage(*pkg, miss.usage)
		}
		r.hostMisses.Remove(path)
	}
}

// isFileAt reports whether the host file of inode is at path. An unknown inode
// counts as the file at path.
func (r *Resolver) isFileAt(path string, inode uint64) bool {
	if inode == 0 {
		return true
	}
	var stat syscall.Stat_t
	return syscall.Lstat(r.hostRoot+path, &stat) == nil && stat.Ino == inode
}

// SetProcessWalker sets walk, which calls its argument on every process the
// probe knows of, for the scans of the host packages to record the processes
// running on the host.
func (r *Resolver) SetProcessWalker(walk func(func(*model.ProcessCacheEntry))) {
	r.walkProcesses = walk
}

// getContainerSBOM returns the SBOM workloadmeta holds for the image of the
// container containerID. The functional tests run without workloadmeta.
func (r *Resolver) getContainerSBOM(containerID containerutils.ContainerID) (*workloadmeta.CompressedSBOM, error) {
	if r.wmeta == nil {
		return nil, fmt.Errorf("No workloadmeta to get the image of container '%s' from", containerID)
	}
	container, err := r.wmeta.GetContainer(string(containerID))
	if err != nil {
		return nil, fmt.Errorf("Failed to get container metadata for '%s': %v", containerID, err)
	}

	imageID := container.Image.ID
	if imageID == "" {
		return nil, fmt.Errorf("Container '%s' has no image ID, cannot forward SBOM", containerID)
	}

	existingImage, err := r.wmeta.GetImage(imageID)
	if err != nil || existingImage == nil {
		// Kubelet reports Image.ID as the manifest/repo digest (e.g. "docker.io/foo@sha256:9fb3...")
		// but images are stored by config digest. Fall back to a linear search on RepoDigests.
		for _, img := range r.wmeta.ListImages() {
			for _, digest := range img.RepoDigests {
				if digest == imageID {
					existingImage = img
					break
				}
			}
			if existingImage != nil {
				break
			}
		}
	}

	if existingImage == nil {
		return nil, fmt.Errorf("Image metadata for '%s' not found in workloadmeta, cannot forward SBOM for container '%s'", imageID, containerID)
	}

	if existingImage.SBOM == nil {
		return nil, fmt.Errorf("Image '%s' has no SBOM, cannot forward SBOM for container '%s'", imageID, containerID)
	}

	return existingImage.SBOM, nil
}

// triggerForwarding triggers the forwarding debouncer to send updated SBOM with LastAccess to remote collector
// This function assumes sbom is already locked by the caller
func (r *Resolver) triggerForwarding(sbom *SBOM) {
	// Create forwarder debouncer on demand. Its callback re-arms the debouncer
	// it holds, as stop clears sbom.forwarder while the callback may still run.
	if sbom.forwarder == nil {
		var forwarder *debouncer.Debouncer
		forwarder = debouncer.New(
			r.cfg.SBOMResolverForwardInterval, func() {
				if r.forward(sbom) {
					forwarder.Call()
				}
				r.refreshChanged(sbom, func() string { return r.rootFingerprint(sbom) })
			},
		)
		forwarder.Start()
		sbom.forwarder = forwarder
	}

	// forward on the next tick of the debouncer
	sbom.forwarder.Call()
}

// forward hands the current SBOM data with LastAccess to the listeners, which
// send it to the remote collector, and reports whether to forward it again
// later, as it does while the Trivy SBOM of the image is pending. It returns at
// once for a stopped SBOM.
func (r *Resolver) forward(sbom *SBOM) bool {
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.state.Load() == stoppedState {
		return false
	}

	// The report of a container waits for the Trivy SBOM of its image. The
	// core agent keeps the report of the host for its next host scan.
	if sbom.ContainerID != "" && (sbom.status == workloadmeta.Pending || sbom.status == "") {
		imageSBOM, err := r.getContainerSBOM(sbom.ContainerID)
		if err != nil || imageSBOM == nil {
			seclog.Debugf("Failed to get image SBOM for container '%s': %v", sbom.ContainerID, err)
		} else {
			sbom.status = imageSBOM.Status
		}
	}

	if sbom.ContainerID != "" && (sbom.status == workloadmeta.Pending || sbom.status == "") {
		// Retry until the image's Trivy SBOM is ready: an idle workload may
		// produce no further file accesses to re-trigger forwarding, and the
		// overlayfs scan can take several minutes. Bound the total wait so a
		// permanently-pending image (e.g. container image SBOM collection
		// disabled) stops re-arming instead of looping forever.
		sbom.forwardRetryCount++
		if time.Duration(sbom.forwardRetryCount)*r.cfg.SBOMResolverForwardInterval > maxForwardWait {
			seclog.Warnf("Giving up forwarding SBOM for container '%s': image SBOM still pending after %s", sbom.ContainerID, maxForwardWait)
			return false
		}
		return true
	}
	sbom.forwardRetryCount = 0

	if sbom.data == nil || len(sbom.data.packages) == 0 {
		return false
	}

	seclog.Debugf("Forwarding SBOM with LastAccess for container %s (%d packages)", sbom.ContainerID, len(sbom.data.packages))

	// Snapshot the package metadata: the forwarded report outlives the
	// lock and the backing slice keeps being mutated (LastAccess) at runtime.
	// Take the Data lock because *Data is shared across SBOMs via the data
	// cache, so the SBOM lock alone does not protect the packages slice.
	sbom.data.mu.Lock()
	packages := make([]sbomtypes.Package, len(sbom.data.packages))
	copy(packages, sbom.data.packages)
	observedSince := sbom.data.observedSince
	sbom.data.forwardedAt = time.Now()
	sbom.data.mu.Unlock()

	// Create SBOM report and notify listeners
	packagesReport := NewPackagesReport(packages, sbom.ContainerID)
	packagesReport.observedSince = observedSince
	scanResult := &sbompkg.ScanResult{
		Report:           packagesReport,
		CreatedAt:        time.Now(),
		GenerationMethod: "security-agent",
		RequestID:        string(sbom.ContainerID),
	}
	r.Notifier.NotifyListeners(SBOMComputed, scanResult)

	return false
}

// generateSBOM calls the collector to generate the SBOM of a sbom
func (r *Resolver) generateSBOM(root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	report, err := r.sbomCollector.ScanInstalledPackages(context.Background(), root)
	if err != nil {
		r.failedSBOMGenerations.Inc()
		return nil, fmt.Errorf("failed to generate SBOM for %s: %w", root, err)
	}

	seclog.Infof("SBOM successfully generated from %s with %d packages", root, len(report))

	return report, nil
}

// isUsrMerged reports whether root uses the merged-/usr layout, where /bin is a
// symlink into /usr/bin. There the package database and a resolved exec path may
// disagree on the /bin vs /usr/bin prefix for the same file.
func isUsrMerged(root string) bool {
	fi, err := os.Lstat(root + "/bin")
	return err == nil && fi.Mode()&fs.ModeSymlink != 0
}

// escapeFilePathForRule escapes special characters in file paths for use in rule expressions
// This prevents filenames with quotes, backslashes, or other special characters from breaking the syntax
func escapeFilePathForRule(path string) string {
	// Escape backslashes first (must be done before escaping quotes)
	path = strings.ReplaceAll(path, `\`, `\\`)
	// Escape double quotes
	path = strings.ReplaceAll(path, `"`, `\"`)
	return path
}

// generateSBOMPolicyDef generates a policy definition from SBOM packages
func (r *Resolver) generateSBOMPolicyDef(containerID containerutils.ContainerID, packages []sbomtypes.PackageWithInstalledFiles) *rules.PolicyDef {
	var sbomMacros []*rules.MacroDefinition
	var sbomRules []*rules.RuleDefinition
	fileCount := 0

	for _, pkg := range packages {
		if len(pkg.InstalledFiles) == 0 {
			continue
		}

		fileCount += len(pkg.InstalledFiles)
		pkgName := strings.ReplaceAll(pkg.Package.Name, "-", "_")
		pkgName = strings.ReplaceAll(pkgName, ".", "_")
		macroName := "sbom_pkg_files_" + strings.ToLower(pkgName)

		// Build the macro expression with the list of files
		// Escape filenames to prevent special characters (e.g., ", \) from breaking the expression
		macroExpression := "[ " + strings.Join(lo.Map(pkg.InstalledFiles, func(file string, _ int) string {
			return `"` + escapeFilePathForRule(file) + `"`
		}), ", ") + " ]"

		sbomMacros = append(sbomMacros, &rules.MacroDefinition{
			ID:         macroName,
			Expression: macroExpression,
		})

		// Create a rule that detects when files from this package are opened
		ruleName := "sbom_detect_file_in_pkg_" + strings.ToLower(pkgName)
		sbomRules = append(sbomRules, &rules.RuleDefinition{
			ID:         ruleName,
			Expression: "open.file.path in " + macroName,
			Silent:     true, // These are internal tracking rules
			Tags: map[string]string{
				"sbom_container":       string(containerID),
				"sbom_package_name":    pkg.Package.Name,
				"sbom_package_version": pkg.Package.Version,
			},
		})

		seclog.Debugf("Generated SBOM macro: %s with %d files", macroName, len(pkg.InstalledFiles))
	}

	if len(sbomMacros) == 0 {
		return nil
	}

	seclog.Infof("Generated %d SBOM macros and %d rules for container %s (%d total files)",
		len(sbomMacros), len(sbomRules), containerID, fileCount)

	policyDef := &rules.PolicyDef{
		Macros: sbomMacros,
		Rules:  sbomRules,
	}

	return policyDef
}

// isProcRootGone reports whether err is the expected outcome of reaching for the proc root of a
// process that is exiting: the entry is already reaped (ENOENT) or the task has dropped its
// address space and the kernel no longer resolves it (ESRCH).
func isProcRootGone(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

// sortedPIDs returns the PIDs of cgroup in increasing order, so that the scan and
// the fingerprint of a container read the root of the same process.
func sortedPIDs(cgroup *cgroupModel.CacheEntry) []uint32 {
	pids := cgroup.GetPIDs()
	slices.Sort(pids)
	return pids
}

// onHostRoot reports whether the root of the process pid is the root of the
// host. A process of a system container or a snap runs on a root of its own
// with an empty container ID. A process whose root cannot be read, most often
// one already gone, counts as a host process.
func (r *Resolver) onHostRoot(pid uint32) bool {
	stat, err := utils.UnixStat(utils.ProcRootPath(pid))
	if err != nil {
		return true
	}
	return stat.Dev == r.hostRootDevice && stat.Ino == r.hostRootInode
}

// replaced reports whether the file of an event is other than the host file at
// its path: gone from there, replaced there by another file, or a file of an
// overlayfs over a host root of another filesystem, as a host process sees the
// files of a container through /proc/<pid>/root. An overlayfs file carries the
// inode of its layer, which can differ from the inode stat reports, so on an
// overlayfs host root it counts as in place, as does a file of unknown path or
// inode.
func (r *Resolver) replaced(file *model.FileEvent) bool {
	if !file.IsPathnameStrResolved || file.Inode == 0 {
		return false
	}
	if file.IsInLowerLayer() || file.IsInUpperLayer() {
		return !r.hostRootOverlay
	}
	var stat syscall.Stat_t
	if err := syscall.Lstat(r.hostRoot+file.PathnameStr, &stat); err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return stat.Ino != file.Inode
}

// codeSuffixes are the suffixes of the files that loaders and interpreters
// run, beside executables and shared libraries.
var codeSuffixes = []string{
	".py", ".pyc", ".pyo", ".pl", ".pm", ".rb", ".js", ".mjs", ".cjs", ".node",
	".jar", ".class", ".lua", ".php", ".phar", ".tcl", ".sh", ".beam", ".dll",
	".wasm", ".ko", ".ko.gz", ".ko.xz", ".ko.zst",
}

// sharedLibrary matches the names of shared libraries: libfoo.so, and with a
// version, libfoo.so.1 or libssl.so.1.0.2k.
var sharedLibrary = regexp.MustCompile(`\.so(\.[0-9][0-9A-Za-z_]*)*$`)

// holdsCode reports whether file can hold code that a process runs: an
// executable, a shared library, a kernel module or a module of an interpreter.
// Files under /etc hold configuration, the shell snippets of profile.d that
// every login sources included.
func holdsCode(file *model.FileEvent) bool {
	if strings.HasPrefix(file.PathnameStr, "/etc/") {
		return false
	}
	if file.Mode&0111 != 0 {
		return true
	}
	name := path.Base(file.PathnameStr)
	if sharedLibrary.MatchString(name) {
		return true
	}
	return slices.ContainsFunc(codeSuffixes, func(suffix string) bool {
		return strings.HasSuffix(name, suffix)
	})
}

// codeReaders are the names of the processes that read code without running
// it: the indexers of shared libraries and kernel modules, the builders of
// initramfs images and archives, the verifiers and scanners of files, and the
// tools that hash or inspect binaries. The kernel cuts a process name to 15
// bytes.
var codeReaders = []string{
	"ldconfig", "ldconfig.real", "depmod", "modinfo", "weak-modules",
	"dracut-install", "cpio", "cp", "install", "tar", "rsync",
	"rpmverify", "debsums", "aide", "fapolicyd", "clamscan", "clamd",
	"wazuh-syscheckd", "ossec-syscheckd", "oscap", "oscap-io",
	"md5sum", "sha1sum", "sha224sum", "sha256sum", "sha384sum", "sha512sum",
	"b2sum", "cksum", "file", "strings", "readelf", "objdump", "nm", "gdb",
	"perf", "systemd-coredum",
}

// bytecodeCompilers are the names of the processes that compile the Python
// modules of a package as it installs, in children that read its sources.
var bytecodeCompilers = []string{"py3compile", "pycompile", "pypy3compile"}

// loaderNames are the prefixes of the names of the dynamic loader run as a
// program, as ldd runs it to list the libraries of a binary.
var loaderNames = []string{"ld-linux", "ld-musl", "ld64.so"}

// readsCode reports whether the process of pc reads code without running it.
func readsCode(pc *model.ProcessContext) bool {
	if slices.Contains(codeReaders, pc.Comm) || slices.Contains(bytecodeCompilers, pc.Comm) {
		return true
	}
	if pc.Parent != nil && slices.Contains(bytecodeCompilers, pc.Parent.Comm) {
		return true
	}
	return slices.ContainsFunc(loaderNames, func(prefix string) bool {
		return strings.HasPrefix(pc.Comm, prefix)
	})
}

// isDir reports whether mode, the mode of the file of an event, is that of a
// directory.
func isDir(mode uint16) bool {
	return mode&syscall.S_IFMT == syscall.S_IFDIR
}

func (r *Resolver) doScan(sbom *SBOM) ([]sbomtypes.PackageWithInstalledFiles, error) {
	var (
		lastErr error
		scanned bool
		report  []sbomtypes.PackageWithInstalledFiles
	)

	cfs := utils.DefaultCGroupFS()

	for _, rootCandidatePID := range sortedPIDs(sbom.cgroup) {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, _, _, err := cfs.FindCGroupContext(rootCandidatePID, rootCandidatePID)
		if err != nil {
			continue
		}
		if computedID != sbom.ContainerID {
			continue
		}

		containerProcRootPath := utils.ProcRootPath(rootCandidatePID)
		if sbom.ContainerID != "" {
			stat, err := utils.UnixStat(containerProcRootPath)
			if err != nil {
				if isProcRootGone(err) {
					continue
				}
				return nil, fmt.Errorf("stat failed for `%s`: couldn't stat container proc root path: %w", containerProcRootPath, err)
			}
			if stat.Dev == r.hostRootDevice {
				return nil, fmt.Errorf("couldn't generate sbom: filesystem of container '%s' matches the host root filesystem", sbom.ContainerID)
			}
		}

		if report, lastErr = r.generateSBOM(containerProcRootPath); lastErr == nil {
			sbom.usrMerged = isUsrMerged(containerProcRootPath)
			scanned = true
			break
		}

		if isProcRootGone(lastErr) {
			lastErr = nil
			continue
		}

		seclog.Errorf("couldn't generate SBOM: %v", lastErr)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if !scanned {
		return nil, errNoProcessForContainerID
	}
	return report, nil
}

func (r *Resolver) removeSBOMData(key workloadKey) {
	r.dataCache.remove(key)
}

// addPendingScan records that the workload of containerID waits for a scan. It
// reports whether the workload waited already, and false for ok on a full queue.
func (r *Resolver) addPendingScan(containerID containerutils.ContainerID) (waiting, ok bool) {
	r.pendingScanLock.Lock()
	defer r.pendingScanLock.Unlock()

	if slices.Contains(r.pendingScan, containerID) {
		return true, true
	}
	if len(r.pendingScan) >= scanQueueSize {
		return false, false
	}
	r.pendingScan = append(r.pendingScan, containerID)

	return false, true
}

func (r *Resolver) removePendingScan(containerID containerutils.ContainerID) {
	r.pendingScanLock.Lock()
	defer r.pendingScanLock.Unlock()

	r.pendingScan = slices.DeleteFunc(r.pendingScan, func(v containerutils.ContainerID) bool {
		return v == containerID
	})
}

// analyzeWorkload generates the SBOM of the provided sbom and send it to the security agent
func (r *Resolver) analyzeWorkload(sb *SBOM) error {
	sb.Lock()
	defer sb.Unlock()

	seclog.Infof("analyzing sbom '%s'", sb.ContainerID)

	if currentState := sb.state.Load(); currentState != pendingState {
		r.removePendingScan(sb.ContainerID)

		if currentState != stoppedState {
			// should not append, ignore
			seclog.Warnf("trying to analyze a sbom not in pending state for '%s': %d", sb.ContainerID, currentState)
		}

		// a stopped workload is unreachable from the resolver, so not returning here would start a new
		// forwarding debouncer go routine that we can never stop
		return nil
	}

	// bail out if the workload has been analyzed while queued up
	fingerprint := r.rootFingerprint(sb)
	if r.acquireData(sb, fingerprint) {
		sb.state.Store(computedState)

		r.sbomsCacheHit.Inc()

		r.removePendingScan(sb.ContainerID)
		r.processPendingFileEvents(sb)

		return nil
	}

	// Only count a cache miss when we actually do the scan
	r.sbomsCacheMiss.Inc()

	report, scanErr := r.scanWorkload(sb)
	if scanErr != nil {
		return scanErr
	}

	data := r.setScan(sb, report, fingerprint)

	// mark the SBOM as successful
	sb.state.Store(computedState)

	r.removePendingScan(sb.ContainerID)

	seclog.Infof("new sbom generated for '%s': %d files added", sb.ContainerID, data.files.len())

	// replay any file accesses that arrived before the SBOM was ready
	r.processPendingFileEvents(sb)
	if len(report) > 0 {
		r.triggerForwarding(sb)
	}

	// NOTE: Don't forward SBOM to remote collector immediately, since LastAccess is not yet populated.
	// The SBOM will be forwarded later when packages are accessed at runtime (via the forwarder debouncer).

	// Notify policy generator if callback is set
	r.policyGenLock.RLock()
	cb := r.policyGeneratorCb
	r.policyGenLock.RUnlock()

	if cb != nil {
		// Generate policy definition from SBOM packages
		policyDef := r.generateSBOMPolicyDef(sb.ContainerID, report)
		if policyDef != nil {
			cb(string(sb.workloadKey), sb.ContainerID, policyDef)
		}
	}

	return nil
}

func (r *Resolver) getSBOM(containerID containerutils.ContainerID) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	sbom := r.hostSBOM
	if containerID != "" {
		sbom, _ = r.sboms.Peek(containerID)
	}
	return sbom
}

// ResolvePackage returns the package that owns file, which the process of pc
// runs or opens, and records the access as usage of the package. The path of
// file must be resolved. It returns nil for a file procfs names with
// deletedSuffix, after recording its usage.
func (r *Resolver) ResolvePackage(pc *model.ProcessContext, file *model.FileEvent) *sbomtypes.Package {
	if !file.IsPathnameStrResolved || file.PathResolutionError != nil {
		return nil
	}

	// procfs names a file replaced under a running process by its path
	// followed by deletedSuffix, in the executables and the mappings the probe
	// reads at start and on ruleset reloads. Such a file counts for the package
	// that owns its path. That package may hold the build that replaced it by
	// now, so ResolvePackage returns nil for the file.
	lookupPath, replaced := strings.CutSuffix(file.PathnameStr, deletedSuffix)

	sbom := r.getSBOM(pc.ContainerContext.ContainerID)
	if sbom == nil {
		seclog.Debugf("no sbom found for container '%s'", pc.ContainerContext.ContainerID)
		r.queuePendingFileEvent(pc.ContainerContext.ContainerID, lookupPath, file.Mode, runsAsRoot(pc), file.MountID)
		return nil
	}

	sbom.Lock()

	// If the SBOM scan hasn't completed yet, queue this access so it is replayed once
	// the scan finishes (processPendingFileEvents is called at the end of analyzeWorkload).
	if !sbom.IsComputed() {
		sbom.Unlock()
		r.queuePendingFileEvent(pc.ContainerContext.ContainerID, lookupPath, file.Mode, runsAsRoot(pc), file.MountID)
		return nil
	}

	defer sbom.Unlock()

	// replay any file accesses that arrived before the SBOM was ready
	r.processPendingFileEvents(sbom)

	// The image holds the files of the root mount of its containers, which a bind
	// mount or a ConfigMap at a package path is apart from.
	r.refreshRoot(sbom)
	if !sbom.root.onRoot(file.MountID) {
		return nil
	}

	seclog.Tracef("file '%s' accessed by '%s' in container '%s'", file.PathnameStr, pc.Process.Comm, sbom.ContainerID)

	pkg := r.owner(sbom, pc.Pid, lookupPath, true)

	// rpm lists the directories a package owns among its files. A directory
	// resolves to its package, and opening it, as a listing or a walk of the
	// filesystem does, leaves the usage of the package as it is.
	if pkg != nil && !isDir(file.Mode) {
		seclog.Tracef("file '%s' found in sbom for container '%s'", file.PathnameStr, sbom.ContainerID)

		sbom.data.mu.Lock()
		previous := pkg.LastAccess
		suidBit, accessedByRoot := pkg.SuidBit, pkg.AccessedByRoot

		// Update LastAccess timestamp, SuidBit and AccessedByRoot fields. SuidBit
		// and AccessedByRoot are sticky within a scan generation: once a package's
		// setuid binary has run, or it has run as root, that holds until the next
		// SBOM refresh, regardless of later accesses to its non-setuid files.
		pkg.LastAccess = time.Now()
		pkg.SuidBit = pkg.SuidBit || fs.FileMode(file.Mode)&04000 != 0
		pkg.AccessedByRoot = pkg.AccessedByRoot || runsAsRoot(pc)

		latch := r.latches(sbom.data, previous, pkg.LastAccess) ||
			pkg.SuidBit != suidBit || pkg.AccessedByRoot != accessedByRoot
		sbom.data.mu.Unlock()

		if latch {
			r.triggerForwarding(sbom)
		}
	}

	// The executable of a host process the host index lacks may belong to a
	// package installed since the last scan. Its run waits for the scan
	// indexing it, if the executable is still at its path.
	if pkg == nil && !replaced && sbom.ContainerID == "" && file.PathnameStr == pc.Process.FileEvent.PathnameStr {
		r.addHostMiss(pc, file)
	}

	if replaced {
		return nil
	}
	return pkg
}

// latches reports whether an access at now latches a forward of d: after a gap
// since previous or on old forwarded usage, which it claims. d.mu must be held.
func (r *Resolver) latches(d *Data, previous, now time.Time) bool {
	if now.Sub(previous) > r.cfg.SBOMResolverEnrichmentInterval {
		return true
	}
	if now.Sub(d.forwardedAt) > maxForwardedAge {
		d.forwardedAt = now
		return true
	}
	return false
}

// runsAsRoot reports whether a process runs as root, by its real or effective
// UID, as a setuid-root binary run by another user does.
func runsAsRoot(pc *model.ProcessContext) bool {
	return pc.UID == 0 || pc.EUID == 0
}

// LookupPackage returns the package that owns file, as ResolvePackage does,
// and leaves the usage of the package as it is. The package fields of CWS
// events read it, for the files a process runs or opens as well as for the
// target of a chmod or an unlink.
func (r *Resolver) LookupPackage(pc *model.ProcessContext, file *model.FileEvent) *sbomtypes.Package {
	return r.lookup(pc, file, false)
}

// lookup returns the package owning file, or the package a use of file credits
// when usage is set, and leaves its usage as it is.
func (r *Resolver) lookup(pc *model.ProcessContext, file *model.FileEvent, usage bool) *sbomtypes.Package {
	if !file.IsPathnameStrResolved || file.PathResolutionError != nil {
		return nil
	}

	sbom := r.getSBOM(pc.ContainerContext.ContainerID)
	if sbom == nil {
		return nil
	}

	sbom.RLock()
	defer sbom.RUnlock()

	if !sbom.IsComputed() {
		return nil
	}
	return r.owner(sbom, pc.Pid, file.PathnameStr, usage)
}

// owner returns the package of sbom owning path, or the one a use of it credits
// with usage set. A match in the host SBOM needs pid on the host root.
func (r *Resolver) owner(sbom *SBOM, pid uint32, path string, usage bool) *sbomtypes.Package {
	var pkg *sbomtypes.Package
	if usage {
		pkg = sbom.data.files.queryUsage(path)
	} else {
		pkg = sbom.data.files.queryFile(path)
	}
	if pkg != nil && sbom.ContainerID == "" && !r.onHostRoot(pid) {
		return nil
	}
	return pkg
}

// HostPackageVersion returns the version of the host package that owns path,
// and the empty string for a path outside the host packages. The usage of the
// package stays as it is.
func (r *Resolver) HostPackageVersion(path string) string {
	sbom := r.getSBOM("")
	if sbom == nil {
		return ""
	}

	sbom.RLock()
	defer sbom.RUnlock()

	if !sbom.IsComputed() {
		return ""
	}
	if pkg := sbom.data.files.queryFile(path); pkg != nil {
		return pkg.Version
	}
	return ""
}

// ResolveOpen records the open of file by the process of pc as usage of the
// package that owns it, as ResolvePackage does, when file can hold code and
// the process may run it. The open of a host package file counts once it has
// waited settleDelay, and only if the file is still at its path then. An open
// arriving while maxOpened opens wait is dropped.
func (r *Resolver) ResolveOpen(pc *model.ProcessContext, file *model.FileEvent) {
	if !holdsCode(file) || readsCode(pc) {
		return
	}
	if pc.ContainerContext.ContainerID != "" {
		r.ResolvePackage(pc, file)
		return
	}
	// lookup reads the root of the process while it lives. With usage set,
	// bytecode a package leaves unlisted counts for its source.
	if isDir(file.Mode) || r.lookup(pc, file, true) == nil {
		return
	}

	r.openedLock.Lock()
	defer r.openedLock.Unlock()
	if len(r.opened) < maxOpened {
		r.opened = append(r.opened, openedFile{pid: pc.Pid, cred: pc.Credentials, comm: pc.Comm, file: *file, at: time.Now()})
	}
}

// settleOpens records the opens of host files that have waited settleDelay,
// on every tick until ctx is done.
func (r *Resolver) settleOpens(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick:
			r.recordSettled(now)
		}
	}
}

// recordSettled records as usage the opens of host files that have waited
// settleDelay by now, those whose files are still at their paths.
func (r *Resolver) recordSettled(now time.Time) {
	r.openedLock.Lock()
	n := 0
	for n < len(r.opened) && now.Sub(r.opened[n].at) >= settleDelay {
		n++
	}
	settled := slices.Clone(r.opened[:n])
	r.opened = slices.Delete(r.opened, 0, n)
	r.openedLock.Unlock()

	for i := range settled {
		o := &settled[i]
		if r.replaced(&o.file) {
			continue
		}
		var pc model.ProcessContext
		pc.Pid, pc.Credentials, pc.Comm = o.pid, o.cred, o.comm
		r.ResolvePackage(&pc, &o.file)
	}
}

// queuePendingFileEvent stores a file access that arrived before the SBOM for the
// given container was ready, keeping up to maxPendingFileEvents distinct files per
// container. Accesses are merged per path: the snapshot replay emits one open event
// per (process, mapped file) pair and runs again on every ruleset reload, so without
// deduplication the shared libraries mapped by every process of a workload crowd out
// the distinct paths worth keeping. Directory opens, which leave the usage of their
// package as it is, stay out of the queue.
func (r *Resolver) queuePendingFileEvent(containerID containerutils.ContainerID, filePath string, fileMode uint16, root bool, mountID uint32) {
	if containerID == "" || isDir(fileMode) {
		return
	}

	event := pendingFileEvent{
		keys:           newUsageKeys(filePath),
		lastAccess:     time.Now().UnixNano(),
		suidBit:        fs.FileMode(fileMode)&04000 != 0,
		accessedByRoot: root,
	}
	key := pendingFileKey{path: event.keys.path, mountID: mountID}

	r.pendingFileEventsLock.Lock()
	defer r.pendingFileEventsLock.Unlock()

	events, ok := r.pendingFileEvents.Get(containerID)
	if !ok {
		events = make(map[pendingFileKey]pendingFileEvent)
		r.pendingFileEvents.Add(containerID, events)
	}

	if previous, ok := events[key]; ok {
		event.suidBit = event.suidBit || previous.suidBit
		event.accessedByRoot = event.accessedByRoot || previous.accessedByRoot
	} else if len(events) >= maxPendingFileEvents {
		seclog.Debugf("dropping pending file event '%s' for container '%s': too many pending events", filePath, containerID)
		return
	}

	events[key] = event
}

// processPendingFileEvents applies the accesses queued for sbom to its data and
// latches a forward when one matched. Must be called with sbom.Lock() already held.
func (r *Resolver) processPendingFileEvents(sbom *SBOM) {
	r.pendingFileEventsLock.Lock()
	events, ok := r.pendingFileEvents.Peek(sbom.ContainerID)
	r.pendingFileEvents.Remove(sbom.ContainerID)
	r.pendingFileEventsLock.Unlock()

	if !ok || len(events) == 0 {
		return
	}
	r.refreshRoot(sbom)

	seclog.Debugf("processing %d pending file events for container '%s'", len(events), sbom.ContainerID)

	recorded := false
	sbom.data.mu.Lock()
	for key, event := range events {
		if !sbom.root.onRoot(key.mountID) {
			continue
		}
		pkg := sbom.data.files.queryKeys(event.keys)
		if pkg == nil {
			continue
		}
		if at := time.Unix(0, event.lastAccess); at.After(pkg.LastAccess) {
			pkg.LastAccess = at
		}
		pkg.SuidBit = pkg.SuidBit || event.suidBit
		pkg.AccessedByRoot = pkg.AccessedByRoot || event.accessedByRoot
		recorded = true
	}
	sbom.data.mu.Unlock()

	if recorded {
		r.triggerForwarding(sbom)
	}
}

// newSBOM (thread unsafe) creates a new SBOM entry for the sbom designated by the provided process cache
// entry
func (r *Resolver) newSBOM(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry, workloadKey workloadKey) *SBOM {
	sbom := NewSBOM(id, cgroup, workloadKey)
	r.sboms.Add(id, sbom)
	return sbom
}

// refreshRoot reads the root mount of sbom while unknown, at most every
// rootReadInterval, as processes start on the host root. sbom.Lock() must be held.
func (r *Resolver) refreshRoot(sbom *SBOM) {
	if sbom.ContainerID == "" || sbom.root.mountID != 0 || time.Since(sbom.rootRead) < rootReadInterval {
		return
	}
	sbom.root, sbom.rootRead = r.readContainerRoot(sbom.ContainerID, sbom.cgroup), time.Now()
}

// readContainerRoot returns the root mount of container id and the mounts of its
// namespace, read from one of the processes of cgroup off the root of the host.
func (r *Resolver) readContainerRoot(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry) containerRoot {
	if cgroup == nil {
		return containerRoot{}
	}
	cfs := utils.DefaultCGroupFS()
	for _, pid := range cgroup.GetPIDs() {
		if computedID, _, _, err := cfs.FindCGroupContext(pid, pid); err != nil || computedID != id || r.onHostRoot(pid) {
			continue
		}
		mounts, err := kernel.ParseMountInfoFile(int32(pid))
		if err != nil {
			continue
		}
		root := containerRoot{mounts: make([]uint32, 0, len(mounts))}
		for _, mnt := range mounts {
			root.mounts = append(root.mounts, uint32(mnt.ID))
			// the last mount on / hides the ones below it
			if mnt.Mountpoint == "/" {
				root.mountID = uint32(mnt.ID)
			}
		}
		if root.mountID != 0 {
			return root
		}
	}
	return containerRoot{}
}

// queueWorkload inserts the provided sbom in a SBOM resolver chan, it will be inserted in the scanChan or the
// delayerChan depending on the tags that have been resolved
func (r *Resolver) queueWorkload(sbom *SBOM) {
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.state.Load() != pendingState {
		// this sbom was deleted before we could scan it, ignore it
		return
	}

	// check if this sbom has been scanned before
	if r.acquireData(sbom, r.rootFingerprint(sbom)) {
		sbom.state.Store(computedState)

		r.sbomsCacheHit.Inc()

		// file accesses are queued from the moment a container ID resolves, which
		// happens before the workload selector that got us here.
		r.processPendingFileEvents(sbom)

		return
	}
	// Do not increment sbomsCacheMiss here since the SBOM can still hit
	// the cache after it has been dequeued.

	r.triggerScan(sbom)
}

func (r *Resolver) triggerScan(sbom *SBOM) {
	waiting, ok := r.addPendingScan(sbom.ContainerID)
	if !ok {
		r.deleteSBOM(sbom)
		return
	}
	// the scan of a waiting workload reads its package databases when it runs
	if waiting {
		return
	}

	// push sbom to the scanner chan
	select {
	case r.scanChan <- sbom:
	default:
		r.removePendingScan(sbom.ContainerID)
		r.deleteSBOM(sbom)
	}
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (r *Resolver) OnWorkloadSelectorResolvedEvent(workload *tags.Workload) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	if workload == nil {
		return
	}

	id := workload.GCroupCacheEntry.GetContainerID()
	// The host SBOM comes from scanHost.
	if len(id) == 0 {
		return
	}

	_, ok := r.sboms.Get(id)
	if !ok {
		sbom := r.newSBOM(id, workload.GCroupCacheEntry, r.workloadKeyOf(id, workload))
		if imageSBOM, err := r.getContainerSBOM(id); err == nil && imageSBOM != nil {
			sbom.status = imageSBOM.Status
		}
		r.queueWorkload(sbom)
	}
}

// workloadKeyOf returns the key under which containers share a scan: the image
// ID from the tags or from workloadmeta, or else the container ID.
func (r *Resolver) workloadKeyOf(id containerutils.ContainerID, workload *tags.Workload) workloadKey {
	if imageID := utils.GetTagValue("image_id", workload.Tags); imageID != "" {
		return workloadKey(imageID)
	}
	if r.wmeta == nil {
		return workloadKey(id)
	}
	if container, err := r.wmeta.GetContainer(string(id)); err == nil && container.Image.ID != "" {
		return workloadKey(container.Image.ID)
	}
	return workloadKey(id)
}

// GetWorkload returns the sbom of a provided ID
func (r *Resolver) GetWorkload(id containerutils.ContainerID) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	if id == "" {
		return r.hostSBOM
	}

	sbom, _ := r.sboms.Peek(id)
	return sbom
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(cgroup *cgroupModel.CacheEntry) {
	if !cgroup.IsContainerContextNull() {
		r.Delete(cgroup.GetContainerContext().ContainerID)
	}
}

// Delete removes the SBOM of the provided cgroup id
func (r *Resolver) Delete(id containerutils.ContainerID) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	sbom, ok := r.sboms.Peek(id)
	if !ok {
		// file accesses are queued as soon as a container ID is resolved, which
		// happens well before the workload selector that creates the SBOM entry, so
		// a container that dies in between has accesses to release and nothing else
		r.deletePendingFileEvents(id)
		return
	}
	sbom.Lock()
	defer sbom.Unlock()

	// Remove this SBOM
	r.deleteSBOM(sbom)
}

// deleteSBOM delete all data indexed by the provided container ID
// must be called with sbomsLock held
func (r *Resolver) deleteSBOM(sbom *SBOM) {
	seclog.Infof("deleting SBOM entry for '%s'", sbom.ContainerID)

	// should be called under sbom.Lock and sbomsLock.Lock
	// the eviction callback releases everything else indexed by the container ID
	r.stopSBOM(sbom)
	r.sboms.Remove(sbom.ContainerID)
}

// stopSBOM stops sbom and releases its data.
// Must be called with sbom.Lock() already held.
func (r *Resolver) stopSBOM(sbom *SBOM) {
	sbom.stop()
	r.setData(sbom, nil, false, "")
}

// acquireData gives sbom the cached scan of its workload on databases of fingerprint,
// if any, and reports whether it did. Must be called with sbom.Lock() already held.
func (r *Resolver) acquireData(sbom *SBOM, fingerprint string) bool {
	data, ok := r.dataCache.acquire(dataKey(sbom.workloadKey, fingerprint))
	if ok {
		r.setData(sbom, data, true, fingerprint)
	}
	return ok
}

// setScan gives sbom the data of its fresh scan report, cached unless empty, as
// a failing scanner returns. Must be called with sbom.Lock() already held.
func (r *Resolver) setScan(sbom *SBOM, report []sbomtypes.PackageWithInstalledFiles, fingerprint string) *Data {
	data := newData(report, sbom.usrMerged)
	if len(report) > 0 {
		r.dataCache.add(dataKey(sbom.workloadKey, fingerprint), data)
	}
	r.setData(sbom, data, len(report) > 0, fingerprint)
	return data
}

// setData gives sbom data scanned from databases of fingerprint, a cache use when cached
// is set, and releases the data it held. Must be called with sbom.Lock() already held.
func (r *Resolver) setData(sbom *SBOM, data *Data, cached bool, fingerprint string) {
	if sbom.cached {
		r.dataCache.release(dataKey(sbom.workloadKey, sbom.fingerprint))
	}
	sbom.data = data
	sbom.cached = cached
	sbom.fingerprint = fingerprint
}

// rootFingerprint returns the fingerprint of the package databases in the root
// of the container of sbom, read through the process doScan scans, if any.
func (r *Resolver) rootFingerprint(sbom *SBOM) string {
	if sbom.ContainerID == "" || sbom.cgroup == nil {
		return ""
	}
	cfs := utils.DefaultCGroupFS()
	for _, pid := range sortedPIDs(sbom.cgroup) {
		if id, _, _, err := cfs.FindCGroupContext(pid, pid); err != nil || id != sbom.ContainerID || r.onHostRoot(pid) {
			continue
		}
		if fingerprint := r.sbomCollector.Fingerprint(utils.ProcRootPath(pid)); fingerprint != "" {
			return fingerprint
		}
	}
	return ""
}

// refreshChanged refreshes the SBOM of a container whose databases changed since its
// scan, as the CWS refresh rules would. fingerprint reads their current fingerprint.
func (r *Resolver) refreshChanged(sbom *SBOM, fingerprint func() string) {
	sbom.RLock()
	scanned := sbom.fingerprint
	computed := sbom.state.Load() == computedState
	sbom.RUnlock()
	if !computed || scanned == "" {
		return
	}
	if current := fingerprint(); current == "" || current == scanned {
		return
	}
	if err := r.RefreshSBOM(sbom.ContainerID); err != nil {
		seclog.Debugf("failed to refresh the SBOM of %s after a package change: %v", sbom.ContainerID, err)
	}
}

// onSBOMEvicted releases everything indexed by the container ID of an SBOM leaving
// the cache, removed by deleteSBOM or evicted as the least recently used.
func (r *Resolver) onSBOMEvicted(_ containerutils.ContainerID, sbom *SBOM) {
	r.removePendingScan(sbom.ContainerID)
	r.deletePendingFileEvents(sbom.ContainerID)

	// Add evicts an SBOM without its lock, which a scan may hold for long, so
	// it is stopped once that lock is free.
	if sbom.state.Load() != stoppedState {
		go func() {
			sbom.Lock()
			r.stopSBOM(sbom)
			sbom.Unlock()
		}()
	}
}

// deletePendingFileEvents drops the file accesses queued for the provided container ID
func (r *Resolver) deletePendingFileEvents(containerID containerutils.ContainerID) {
	r.pendingFileEventsLock.Lock()
	r.pendingFileEvents.Remove(containerID)
	r.pendingFileEventsLock.Unlock()
}

// SetPolicyGeneratorCallback sets a callback to be called when an SBOM is computed
// This callback can be used to generate policies based on the SBOM data
func (r *Resolver) SetPolicyGeneratorCallback(cb func(workloadKey string, containerID containerutils.ContainerID, policyDef *rules.PolicyDef)) {
	r.policyGenLock.Lock()
	defer r.policyGenLock.Unlock()
	r.policyGeneratorCb = cb
}

// CountEnrichedSBOMForwarded records an enriched SBOM that was successfully
// handed over to its consumer.
func (r *Resolver) CountEnrichedSBOMForwarded() {
	r.enrichedSBOMForwarded.Inc()
}

// CountEnrichedSBOMForwardDropped records an enriched SBOM that was computed but
// couldn't be handed over to its consumer. Listeners are notified synchronously,
// so a slow consumer drops rather than blocks the debouncer.
func (r *Resolver) CountEnrichedSBOMForwardDropped() {
	r.enrichedSBOMForwardDropped.Inc()
}

// SendStats sends stats
func (r *Resolver) SendStats() error {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()
	if val := float64(r.sboms.Len()); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverActiveSBOMs, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverActiveSBOMs: %w", err)
		}
	}

	if val := r.sbomGenerations.Swap(0); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMGenerations, int64(val), []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMGenerations: %w", err)
		}
	}

	used, unused := r.dataCache.lens()
	for _, l := range []struct {
		state string
		len   int
	}{{"used", used}, {"unused", unused}} {
		if l.len > 0 {
			if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverSBOMCacheLen, float64(l.len), []string{"state:" + l.state}, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheLen: %w", err)
			}
		}
	}

	if val := int64(r.sbomsCacheHit.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheHit, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheHit: %w", err)
		}
	}

	if val := int64(r.sbomsCacheMiss.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheMiss, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheMiss: %w", err)
		}
	}

	if val := int64(r.failedSBOMGenerations.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverFailedSBOMGenerations, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverFailedSBOMGenerations: %w", err)
		}
	}

	if val := int64(r.enrichedSBOMForwarded.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverEnrichedSBOMForwarded, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverEnrichedSBOMForwarded: %w", err)
		}
	}

	if val := int64(r.enrichedSBOMForwardDropped.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverEnrichedSBOMForwardDropped, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverEnrichedSBOMForwardDropped: %w", err)
		}
	}

	return nil
}
