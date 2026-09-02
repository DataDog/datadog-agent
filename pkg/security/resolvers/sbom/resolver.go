// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sbom attributes the file accesses the probe observes to the packages
// that own them.
//
// The core agent scans a workload and publishes the file-to-component table of
// that scan. This resolver holds the table, answers the SECL package.* fields
// from it, and reports back which components were seen running. It parses no
// package database of its own: the identities in the table are the ones the
// agent put in the SBOM it sends, so nothing has to be matched up afterwards.
package sbom

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/twmb/murmur3"
	"go.uber.org/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const (
	// maxWorkloads bounds the indexes held at once, one per image in use on the
	// node plus the host.
	maxWorkloads = 1024
	// setuidBit is the set-user-ID bit of a raw Unix file mode. fs.ModeSetuid is
	// Go's portable spelling and sits in a different position, so it cannot be
	// matched against a mode the kernel reported.
	setuidBit = 0o4000
	// maxPendingPaths bounds the distinct paths queued per container while its
	// index has not arrived. The snapshot replay emits one open per (process,
	// mapped file) pair and runs again on every ruleset reload, so accesses are
	// merged per path or the shared libraries every process maps would crowd out
	// the paths worth keeping.
	maxPendingPaths = 256
)

// IndexSource is the core agent, seen from here: it publishes the file table of
// each scan it completes, and takes back the usage observed against it.
type IndexSource interface {
	// Indexes carries every index the agent publishes.
	Indexes() <-chan *usage.Index
	// Capabilities names the scan sources the agent has running. The second
	// result is false until the agent has said, so a caller can tell "no source"
	// from "not yet known".
	Capabilities() (usage.Capabilities, bool)
	// Report sends the usage observed against one exact BOM/index instance.
	Report(report *usage.Report) error
	// Refresh asks for a workload to be scanned again.
	Refresh(scan usage.ScanID, containerID containerutils.ContainerID) error
}

// observed is what the resolver saw of one component since the last report.
type observed struct {
	lastSeen time.Time
	suid     bool
	asRoot   bool
}

// pendingAccess is a file access that arrived before the index did. Accesses are
// merged per path and the drain stamps them all with one timestamp, so only the
// sticky flags have to be kept.
type pendingAccess struct {
	suid   bool
	asRoot bool
}

// workload holds the index of one scan and the usage observed against it. Usage
// is dropped whenever a new index arrives: it answered the table that instance
// replaced.
type workload struct {
	index *usage.Index

	mu    sync.Mutex
	usage map[uint32]observed
	dirty bool
}

// record notes an access to one component, and reports whether it changed
// anything worth sending.
func (w *workload) record(ref uint32, now time.Time, suid, asRoot bool, interval time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()

	previous, seen := w.usage[ref]
	// Both flags are sticky within an index instance: once a package's setuid binary
	// has run, or it has run as root, that holds until the workload is scanned
	// again, whatever is accessed next.
	current := observed{
		lastSeen: now,
		suid:     previous.suid || suid,
		asRoot:   previous.asRoot || asRoot,
	}
	w.usage[ref] = current

	if !seen || current.suid != previous.suid || current.asRoot != previous.asRoot ||
		now.Sub(previous.lastSeen) > interval {
		w.dirty = true
	}
}

// report returns the usage observed since the last call, or nil when nothing
// changed. The workload is marked clean here and put back with redirty when the
// send fails, so an access with no later event is not lost to a transport error.
func (w *workload) report() *usage.Report {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.dirty {
		return nil
	}
	w.dirty = false

	report := &usage.Report{
		Scan:       w.index.Scan,
		Generation: w.index.Generation,
		IndexID:    w.index.IndexID,
		Usage:      make([]usage.Usage, 0, len(w.usage)),
	}
	for ref, o := range w.usage {
		report.Usage = append(report.Usage, usage.Usage{
			Ref:      ref,
			LastSeen: o.lastSeen,
			Suid:     o.suid,
			AsRoot:   o.asRoot,
		})
	}
	return report
}

// redirty puts the workload back in the reporting queue after a failed send.
func (w *workload) redirty() {
	w.mu.Lock()
	w.dirty = true
	w.mu.Unlock()
}

// Resolver attributes file accesses to the components of an SBOM.
type Resolver struct {
	cfg          *config.RuntimeSecurityConfig
	statsdClient statsd.ClientInterface
	wmeta        workloadmeta.Component
	source       IndexSource

	mu sync.RWMutex
	// workloads holds one entry per scan the agent has published.
	workloads map[usage.ScanID]*workload
	// scans maps a container to the scan describing it, so an access resolves
	// without a workloadmeta lookup per event.
	scans map[containerutils.ContainerID]usage.ScanID

	pendingLock sync.Mutex
	pending     *simplelru.LRU[containerutils.ContainerID, map[string]pendingAccess]

	// capabilitiesCb is called once the core agent has answered which scan
	// sources it runs, so a rule set loaded before the answer can be reloaded
	// against it.
	capabilitiesLock sync.RWMutex
	capabilitiesCb   func()
	capabilitiesOnce sync.Once

	indexesReceived *atomic.Uint64
	reportsSent     *atomic.Uint64
	unattributed    *atomic.Uint64
}

// NewSBOMResolver returns a new instance of Resolver.
func NewSBOMResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, wmeta workloadmeta.Component, source IndexSource) (*Resolver, error) {
	pending, err := simplelru.NewLRU[containerutils.ContainerID, map[string]pendingAccess](maxWorkloads, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOM resolver: %w", err)
	}

	return &Resolver{
		cfg:             c,
		statsdClient:    statsdClient,
		wmeta:           wmeta,
		source:          source,
		workloads:       make(map[usage.ScanID]*workload),
		scans:           make(map[containerutils.ContainerID]usage.ScanID),
		pending:         pending,
		indexesReceived: atomic.NewUint64(0),
		reportsSent:     atomic.NewUint64(0),
		unattributed:    atomic.NewUint64(0),
	}, nil
}

// Start begins receiving indexes and reporting the usage observed against them.
func (r *Resolver) Start(ctx context.Context) error {
	if r.source == nil {
		seclog.Warnf("no SBOM index source: the runtime usage enrichment and the package fields are unavailable")
		return nil
	}

	go r.receive(ctx)
	go r.flush(ctx)
	go r.awaitCapabilities(ctx)
	return nil
}

// SetCapabilitiesCallback sets the callback to run once the core agent has said
// which scan sources it runs.
func (r *Resolver) SetCapabilitiesCallback(cb func()) {
	r.capabilitiesLock.Lock()
	r.capabilitiesCb = cb
	r.capabilitiesLock.Unlock()
}

// awaitCapabilities runs the callback once the handshake has completed.
//
// A rule set can load before the core agent answers, and a rule reading a
// package field is admitted while the answer is outstanding rather than rejected
// on a guess. Reloading once the answer arrives is what gives such a rule the
// status it deserves, in either direction.
func (r *Resolver) awaitCapabilities(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, known := r.source.Capabilities(); !known {
				continue
			}
			r.capabilitiesOnce.Do(func() {
				r.capabilitiesLock.RLock()
				cb := r.capabilitiesCb
				r.capabilitiesLock.RUnlock()
				if cb != nil {
					cb()
				}
			})
			return
		}
	}
}

// receive adopts each index the agent publishes.
func (r *Resolver) receive(ctx context.Context) {
	indexes := r.source.Indexes()
	for {
		select {
		case <-ctx.Done():
			return
		case index, ok := <-indexes:
			if !ok {
				return
			}
			r.setIndex(index)
		}
	}
}

// setIndex adopts a new BOM/index instance, releasing usage observed against the
// one it supersedes, and replays accesses that arrived while it was missing.
func (r *Resolver) setIndex(index *usage.Index) {
	if index == nil {
		return
	}
	switch index.Status {
	case usage.Gone:
		r.mu.Lock()
		delete(r.workloads, index.Scan)
		for id, scan := range r.scans {
			if scan == index.Scan {
				delete(r.scans, id)
			}
		}
		r.mu.Unlock()
		return
	case usage.Failed:
		// The agent will produce no table for this workload, so release the
		// accesses waiting for one instead of holding them until eviction.
		seclog.Debugf("no SBOM will be produced for %s", index.Scan)
		r.dropPendingFor(index.Scan)
		return
	}

	r.indexesReceived.Inc()
	// The first interval reports an empty baseline even when no indexed file was
	// opened. Without it the core cannot distinguish a measured-idle workload
	// from one for which system-probe has not answered yet.
	w := &workload{index: index, usage: make(map[uint32]observed), dirty: true}

	r.mu.Lock()
	r.workloads[index.Scan] = w
	containers := make([]containerutils.ContainerID, 0, 1)
	for id, scan := range r.scans {
		if scan == index.Scan {
			containers = append(containers, id)
		}
	}
	// A scan of one container's own filesystem was read because the container
	// diverged from its image, so it supersedes the image index for it and is the
	// only table naming what has been installed since.
	if index.Scan.IsContainer() {
		id := containerutils.ContainerID(strings.TrimPrefix(string(index.Scan), "container:"))
		r.scans[id] = index.Scan
		if !slices.Contains(containers, id) {
			containers = append(containers, id)
		}
	}
	r.mu.Unlock()

	seclog.Infof("adopted SBOM index for %s: %d components, %d files, generation %d, index %q",
		index.Scan, len(index.Components), len(index.Refs), index.Generation, index.IndexID)

	for _, id := range containers {
		r.replayPending(id, w)
	}
}

// flush sends the usage observed for every workload on each tick.
func (r *Resolver) flush(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.SBOMResolverEnrichmentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.RLock()
			workloads := make([]*workload, 0, len(r.workloads))
			for _, w := range r.workloads {
				workloads = append(workloads, w)
			}
			r.mu.RUnlock()

			for _, w := range workloads {
				report := w.report()
				if report == nil {
					continue
				}
				if err := r.source.Report(report); err != nil {
					seclog.Warnf("could not report SBOM usage for %s: %v", report.Scan, err)
					w.redirty()
					continue
				}
				r.reportsSent.Inc()
			}
		}
	}
}

// ResolvePackage returns the component that owns the given file, or nil when no
// index attributes it. It records the access, so the component is reported in
// use on the next flush.
//
// A path can belong to several components, every module compiled into one Go
// binary being the case that matters. All of them are recorded, and the one
// returned is the containing artifact where the index names it, since the binary
// is a more useful answer than an arbitrary one of its modules.
func (r *Resolver) ResolvePackage(pc *model.ProcessContext, file *model.FileEvent) *usage.Component {
	if !file.IsPathnameStrResolved {
		return nil
	}

	containerID := pc.Process.ContainerContext.ContainerID
	w := r.workloadFor(containerID)
	if w == nil {
		r.queuePending(containerID, file.PathnameStr, file.Mode, pc.Process.Credentials.UID)
		return nil
	}

	suid := file.Mode&setuidBit != 0
	asRoot := pc.Process.Credentials.UID == 0

	refs := w.index.Lookup(murmur3.StringSum64(file.PathnameStr))
	if len(refs) == 0 {
		r.unattributed.Inc()
		return nil
	}

	now := time.Now()
	for _, ref := range refs {
		if comp := w.index.Component(ref); comp != nil && comp.Reportable {
			w.record(ref, now, suid, asRoot, r.cfg.SBOMResolverEnrichmentInterval)
		}
	}

	comp := w.index.Component(refs[0])
	seclog.Tracef("file '%s' belongs to %s in container '%s'", file.PathnameStr, comp.Name, containerID)
	return comp
}

// LookupPackage returns the component that owns the given path in the host index
// without recording an access. It answers a question about a file rather than
// about something that ran, which is what tagging a service from its unit file
// needs.
func (r *Resolver) LookupPackage(path string) *usage.Component {
	w := r.workloadFor("")
	if w == nil {
		return nil
	}
	if refs := w.index.Lookup(murmur3.StringSum64(path)); len(refs) > 0 {
		return w.index.Component(refs[0])
	}
	return nil
}

// workloadFor returns the index covering the given container, or the host index
// for an event with no container.
func (r *Resolver) workloadFor(containerID containerutils.ContainerID) *workload {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if containerID == "" {
		return r.workloads[usage.Host]
	}
	scan, ok := r.scans[containerID]
	if !ok {
		return nil
	}
	return r.workloads[scan]
}

// queuePending stores an access that arrived before the index it needs.
func (r *Resolver) queuePending(containerID containerutils.ContainerID, path string, mode uint16, uid uint32) {
	if containerID == "" {
		return
	}

	access := pendingAccess{
		suid:   mode&setuidBit != 0,
		asRoot: uid == 0,
	}

	r.pendingLock.Lock()
	defer r.pendingLock.Unlock()

	accesses, ok := r.pending.Get(containerID)
	if !ok {
		accesses = make(map[string]pendingAccess)
		r.pending.Add(containerID, accesses)
	}

	if previous, ok := accesses[path]; ok {
		access.suid = access.suid || previous.suid
		access.asRoot = access.asRoot || previous.asRoot
	} else if len(accesses) >= maxPendingPaths {
		seclog.Debugf("dropping queued access '%s' for container '%s': too many queued", path, containerID)
		return
	}

	accesses[path] = access
}

// replayPending applies the accesses queued for a container against its index.
func (r *Resolver) replayPending(containerID containerutils.ContainerID, w *workload) {
	r.pendingLock.Lock()
	accesses, ok := r.pending.Peek(containerID)
	r.pending.Remove(containerID)
	r.pendingLock.Unlock()

	if !ok || len(accesses) == 0 {
		return
	}

	seclog.Debugf("replaying %d queued accesses for container '%s'", len(accesses), containerID)
	now := time.Now()
	for path, access := range accesses {
		for _, ref := range w.index.Lookup(murmur3.StringSum64(path)) {
			w.record(ref, now, access.suid, access.asRoot, r.cfg.SBOMResolverEnrichmentInterval)
		}
	}
}

// dropPendingFor releases the accesses queued for every container of a scan.
func (r *Resolver) dropPendingFor(scan usage.ScanID) {
	r.mu.RLock()
	var containers []containerutils.ContainerID
	for id, s := range r.scans {
		if s == scan {
			containers = append(containers, id)
		}
	}
	r.mu.RUnlock()

	r.pendingLock.Lock()
	defer r.pendingLock.Unlock()
	for _, id := range containers {
		r.pending.Remove(id)
	}
}

// RefreshSBOM asks the core agent to scan a container again, because its package
// database changed and the index no longer describes it. The agent owns the scan,
// so it is the only place that can bring the SBOM and the index back in step.
func (r *Resolver) RefreshSBOM(containerID containerutils.ContainerID) error {
	if r.source == nil {
		return nil
	}

	r.mu.RLock()
	scan, ok := r.scans[containerID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("container %s has no SBOM to refresh", containerID)
	}

	seclog.Debugf("asking for a rescan of container %s: its packages changed", containerID)
	return r.source.Refresh(scan, containerID)
}

// OnWorkloadSelectorResolvedEvent ties a container to the scan describing it, as
// soon as its tags name the image it runs.
func (r *Resolver) OnWorkloadSelectorResolvedEvent(w *tags.Workload) {
	if w == nil {
		return
	}
	containerID := w.GCroupCacheEntry.GetContainerID()
	if containerID == "" {
		return
	}

	scan, ok := r.scanFor(containerID)
	if !ok {
		seclog.Debugf("no image known for container %s: its accesses stay queued", containerID)
		return
	}

	r.mu.Lock()
	r.scans[containerID] = scan
	target := r.workloads[scan]
	r.mu.Unlock()

	if target != nil {
		r.replayPending(containerID, target)
	}
}

// scanFor resolves the scan describing a container. A scan of the container's own
// filesystem takes precedence, since it is there only because the container
// diverged from its image; otherwise it is the entity ID of the image it runs.
func (r *Resolver) scanFor(containerID containerutils.ContainerID) (usage.ScanID, bool) {
	own := usage.ContainerScan(string(containerID))
	r.mu.RLock()
	_, diverged := r.workloads[own]
	r.mu.RUnlock()
	if diverged {
		return own, true
	}

	container, err := r.wmeta.GetContainer(string(containerID))
	if err != nil {
		return "", false
	}

	imageID := container.Image.ID
	if imageID == "" {
		return "", false
	}

	if image, err := r.wmeta.GetImage(imageID); err == nil && image != nil {
		return usage.ImageScan(image.EntityID.ID), true
	}

	// The kubelet reports Image.ID as the manifest digest while images are
	// stored by config digest, so fall back to matching a repo digest.
	for _, image := range r.wmeta.ListImages() {
		for _, digest := range image.RepoDigests {
			if digest == imageID {
				return usage.ImageScan(image.EntityID.ID), true
			}
		}
	}
	return "", false
}

// OnCGroupDeletedEvent releases what a finished container left behind.
func (r *Resolver) OnCGroupDeletedEvent(cgroup *cgroupModel.CacheEntry) {
	if !cgroup.IsContainerContextNull() {
		r.Delete(cgroup.GetContainerContext().ContainerID)
	}
}

// Delete releases the state held for a container. The index itself stays: it
// describes an image, which other containers may still run.
func (r *Resolver) Delete(containerID containerutils.ContainerID) {
	r.mu.Lock()
	delete(r.scans, containerID)
	r.mu.Unlock()

	r.pendingLock.Lock()
	r.pending.Remove(containerID)
	r.pendingLock.Unlock()
}

// SendStats sends stats.
func (r *Resolver) SendStats() error {
	r.mu.RLock()
	workloads := float64(len(r.workloads))
	r.mu.RUnlock()

	if workloads > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverActiveSBOMs, workloads, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverActiveSBOMs: %w", err)
		}
	}

	for _, stat := range []struct {
		name  string
		value *atomic.Uint64
	}{
		{metrics.MetricSBOMResolverIndexesReceived, r.indexesReceived},
		{metrics.MetricSBOMResolverReportsSent, r.reportsSent},
		{metrics.MetricSBOMResolverUnattributedFiles, r.unattributed},
	} {
		if value := int64(stat.value.Swap(0)); value > 0 {
			if err := r.statsdClient.Count(stat.name, value, []string{}, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s: %w", stat.name, err)
			}
		}
	}

	return nil
}

// LacksWorkloadIndexes reports whether the core agent has said it scans none of
// the workloads this resolver watches. It answers from the capability handshake
// rather than from a configuration key belonging to the other process, so a
// source that is configured but failed to come up reads as absent.
//
// An unanswered handshake is not an answer. The core agent may start after
// system-probe, or a connection may be retrying, and rules load meanwhile, so
// until the answer arrives this reports false and the package fields are treated
// as available.
func (r *Resolver) LacksWorkloadIndexes() bool {
	if r.source == nil {
		return true
	}
	capabilities, known := r.source.Capabilities()
	return known && !capabilities.Workloads()
}
