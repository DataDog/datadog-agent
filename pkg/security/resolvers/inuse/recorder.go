// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package inuse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/collectorv2"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	// settleDelay is how long the open of a host file waits before it counts,
	// so that a file an installer writes and renames over its path counts
	// for the package that holds it then.
	settleDelay = 3 * time.Second
	// maxOpened bounds the opens of host files that wait settleDelay.
	maxOpened = 4096
	// rootReadInterval is how often the root mount of a container is read
	// while unknown.
	rootReadInterval = 10 * time.Second
	// driftCheckInterval is how often the package databases of a container
	// are read again, at runs of the files of its upper layer.
	driftCheckInterval = time.Second
	// maxForwardedAge is the age past which the usage the core agent holds is
	// reported again, so that it learns of a key that keeps running.
	maxForwardedAge = time.Hour
	// maxPendingRecords bounds the records of a container that wait for its
	// workload and its root mount, and maxContainers the containers.
	maxPendingRecords = 2048
	maxContainers     = 1024
)

// Config holds the bounds and the intervals of a Recorder.
type Config struct {
	// ContainerKeys and HostKeys bound the keys of a workload of containers
	// and of the host. A workload that reaches its bound reports itself
	// truncated.
	ContainerKeys int
	HostKeys      int
	// Workloads bounds the workloads, and NodeKeys their keys together.
	Workloads int
	NodeKeys  int
	// ForwardInterval is how often the workloads that changed are reported.
	ForwardInterval time.Duration
	// RunGap is the time after which a key that runs again is reported
	// again.
	RunGap time.Duration
	// Host records the usage of the host as well.
	Host bool
}

// DefaultConfig returns the bounds of the usage records: 16384 keys for a
// workload of containers, 32768 for the host, 1024 workloads and 262144 keys
// in all, about 6 MB.
func DefaultConfig() Config {
	return Config{
		ContainerKeys:   16384,
		HostKeys:        32768,
		Workloads:       1024,
		NodeKeys:        262144,
		ForwardInterval: time.Minute,
		RunGap:          30 * time.Minute,
	}
}

// workloadID names a workload: an image whose containers share their usage,
// a container whose image is unknown, or the host.
type workloadID struct {
	kind usage.KeyKind
	id   string
}

// record is the usage of a key, and the usage of the key last reported.
type record struct {
	lastSeen      int64 // unix seconds
	flags         uint8
	reportedSeen  int64
	reportedFlags uint8
}

// workload holds the usage of a workload.
type workload struct {
	id            workloadID
	keys          map[uint64]record
	containers    map[string]struct{}
	observedSince int64
	truncated     bool
	dirty         bool
	forwardedAt   time.Time
	activeAt      time.Time
}

// pendingKey is a key run on the mount mountID.
type pendingKey struct {
	key   uint64
	mount uint32
}

// container is what the recorder knows of a container: its workload, its root
// mount, and the records waiting for both.
type container struct {
	id       string
	workload workloadID
	known    bool
	root     uint32
	rootRead time.Time
	pending  map[pendingKey]record
	// databases is the fingerprint of the package databases of the container
	// when its root mount was read, checkedAt when they were last read, and
	// drifted reports that they changed since.
	databases string
	checkedAt time.Time
	drifted   bool
}

// openedFile is the open of a host file, waiting settleDelay.
type openedFile struct {
	pid   uint32
	cred  model.Credentials
	comm  string
	file  model.FileEvent
	at    time.Time
	flags uint8
}

// imageResolver returns the image of the container id, "" while unknown, and
// whether the container is known, as workloadmeta or the tags of the
// container tell.
type imageResolver func(id string) (image string, known bool)

// Recorder records the files that processes run in each workload: the keys of
// pkg/sbom/usage, with the time each last ran and its flags. It reports the
// workloads that change to the function SetSend gives. Its methods are safe
// for concurrent use.
type Recorder struct {
	cfg          Config
	statsdClient statsd.ClientInterface
	image        imageResolver
	send         func(*usage.Report)
	now          func() time.Time

	hostRoot        string
	hostRootDevice  uint64
	hostRootInode   uint64
	hostRootOverlay bool

	kernelRelease func() (string, error)
	loadedModules func() ([]string, error)
	// readRoot returns the root mount of a container, as one of its
	// processes sees it.
	readRoot func(id string, pid uint32) uint32
	// fingerprint digests the package databases under a root.
	fingerprint func(root string) string

	mu         sync.Mutex
	containers *simplelru.LRU[string, *container]
	workloads  map[workloadID]*workload
	nodeKeys   int

	openedMu sync.Mutex
	opened   []openedFile

	droppedOpens   atomic.Int64
	forwarded      atomic.Int64
	forwardDropped atomic.Int64
}

// New returns a Recorder that names the image of a container with image, and
// sends its stats to statsdClient.
func New(cfg Config, statsdClient statsd.ClientInterface, image func(id string) (string, bool)) (*Recorder, error) {
	hostRoot := utils.ProcRootPath(1)
	stat, err := utils.UnixStat(hostRoot)
	if err != nil {
		return nil, err
	}
	var fsStat unix.Statfs_t
	if err := unix.Statfs(hostRoot, &fsStat); err != nil {
		return nil, err
	}
	r := newRecorder(cfg, image, nil, hostRoot, stat.Dev, stat.Ino, fsStat.Type == unix.OVERLAYFS_SUPER_MAGIC)
	r.statsdClient = statsdClient
	return r, nil
}

// newRecorder returns a Recorder of the host whose root is at hostRoot, on the
// device dev with the inode ino, of an overlayfs when overlay is set.
func newRecorder(cfg Config, image imageResolver, send func(*usage.Report), hostRoot string, dev, ino uint64, overlay bool) *Recorder {
	containers, _ := simplelru.NewLRU[string, *container](maxContainers, nil)
	r := &Recorder{
		cfg:             cfg,
		image:           image,
		send:            send,
		now:             time.Now,
		hostRoot:        hostRoot,
		hostRootDevice:  dev,
		hostRootInode:   ino,
		hostRootOverlay: overlay,
		kernelRelease:   kernel.Release,
		loadedModules:   LiveModules,
		fingerprint:     collectorv2.NewOSScanner().Fingerprint,
		containers:      containers,
		workloads:       make(map[workloadID]*workload),
	}
	r.readRoot = r.readContainerRoot
	return r
}

// Start records the files of the running kernel on the host, and reports the
// workloads that change every ForwardInterval until ctx is done.
func (r *Recorder) Start(ctx context.Context) {
	seclog.Infof("usage recorder started (host: %t)", r.cfg.Host)
	if r.cfg.Host {
		now := r.now().Unix()
		r.mu.Lock()
		w := r.workload(workloadID{kind: usage.KeyHost})
		for _, p := range KernelFiles(r.hostRoot, r.kernelRelease, r.loadedModules) {
			r.recordKeys(w, usage.AppendKeys(nil, usage.Clean(p)), now, usage.FlagRoot)
		}
		r.mu.Unlock()
	}

	go func() {
		forward := time.NewTicker(r.cfg.ForwardInterval)
		defer forward.Stop()
		settle := time.NewTicker(time.Second)
		defer settle.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-settle.C:
				r.recordSettled(now)
			case now := <-forward.C:
				r.retryPending(now)
				r.Forward(now)
			}
		}
	}()
}

// RecordRun records that the process of pc runs file: its executable, its
// interpreter, or a mapping it holds.
func (r *Recorder) RecordRun(pc *model.ProcessContext, file *model.FileEvent) {
	r.record(pc, file, false)
}

// RecordOpen records the open of file by the process of pc, when file can
// hold code and the process may run it. The open of a host file counts once
// it has waited settleDelay, and only if the file is still at its path then
// and holds code.
func (r *Recorder) RecordOpen(pc *model.ProcessContext, file *model.FileEvent) {
	if !HoldsCode(file) || ReadsCode(pc) || IsDir(file.Mode) {
		return
	}
	r.record(pc, file, true)
}

func (r *Recorder) record(pc *model.ProcessContext, file *model.FileEvent, open bool) {
	if IsPlaceholder(pc) || !file.IsPathnameStrResolved || file.PathResolutionError != nil || IsDir(file.Mode) {
		return
	}
	var flags uint8
	if fs.FileMode(file.Mode)&04000 != 0 {
		flags |= usage.FlagSetuid
	}
	if RunsAsRoot(pc) {
		flags |= usage.FlagRoot
	}

	id := string(pc.ContainerContext.ContainerID)
	if id == "" {
		if !r.cfg.Host || !r.onHostRoot(pc.Pid) {
			return
		}
		if open {
			r.openedMu.Lock()
			if len(r.opened) < maxOpened {
				r.opened = append(r.opened, openedFile{pid: pc.Pid, cred: pc.Credentials, comm: pc.Comm, file: *file, at: r.now(), flags: flags})
			} else {
				r.droppedOpens.Add(1)
			}
			r.openedMu.Unlock()
			return
		}
		r.recordHost(file.PathnameStr, r.now().Unix(), flags)
		return
	}

	path := usage.Clean(strings.TrimSuffix(file.PathnameStr, DeletedSuffix))
	keys := usage.AppendKeys(nil, path)
	now := r.now()

	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.container(id)
	r.resolve(c, pc.Pid, now)
	if !c.known || c.root == 0 {
		for _, key := range keys {
			pk := pendingKey{key: key, mount: file.MountID}
			rec, ok := c.pending[pk]
			if !ok && len(c.pending) >= maxPendingRecords {
				continue
			}
			rec.lastSeen = max(rec.lastSeen, now.Unix())
			rec.flags |= flags
			c.pending[pk] = rec
		}
		return
	}
	w := r.workload(c.workload)
	w.containers[id] = struct{}{}
	// The image holds the files of the root mount of its containers, which a
	// bind mount or a volume at a path of a package is apart from.
	if file.MountID != c.root {
		return
	}
	// A file the container wrote, as an install in the container writes it,
	// drifts from the image. A file of the image copied up as it changed mode
	// or owner still runs the build of the image, so the upper layer drifts
	// only once the package databases changed. The first import of a module
	// writes its bytecode in the upper layer, which still credits its source.
	if file.IsInUpperLayer() && r.drifted(c, pc.Pid, now) {
		source, ok := usage.SourceKey(path)
		if !ok {
			return
		}
		keys = []uint64{source}
	}
	r.recordKeys(w, keys, now.Unix(), flags)
}

// drifted reports whether the package databases of c changed since its root
// mount was read, reading them through the root of the process pid at most
// every driftCheckInterval. r.mu must be held.
func (r *Recorder) drifted(c *container, pid uint32, now time.Time) bool {
	if c.drifted || c.databases == "" || now.Sub(c.checkedAt) < driftCheckInterval {
		return c.drifted
	}
	c.checkedAt = now
	if databases := r.fingerprint(utils.ProcRootPath(pid)); databases != "" && databases != c.databases {
		c.drifted = true
	}
	return c.drifted
}

// recordHost records the run of the host file at p. r.mu must not be held.
func (r *Recorder) recordHost(p string, at int64, flags uint8) {
	keys := usage.AppendKeys(nil, usage.Clean(strings.TrimSuffix(p, DeletedSuffix)))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordKeys(r.workload(workloadID{kind: usage.KeyHost}), keys, at, flags)
}

// recordSettled records the opens of host files that have waited settleDelay
// by now, those whose files are still at their paths and hold code.
func (r *Recorder) recordSettled(now time.Time) {
	r.openedMu.Lock()
	n := 0
	for n < len(r.opened) && now.Sub(r.opened[n].at) >= settleDelay {
		n++
	}
	settled := slices.Clone(r.opened[:n])
	r.opened = slices.Delete(r.opened, 0, n)
	r.openedMu.Unlock()

	for i := range settled {
		o := &settled[i]
		if r.replaced(&o.file) || !r.holdsHostCode(&o.file) {
			continue
		}
		r.recordHost(o.file.PathnameStr, o.at.Unix(), o.flags)
	}
}

// container returns the container id, which it adds when unknown. r.mu must
// be held.
func (r *Recorder) container(id string) *container {
	if c, ok := r.containers.Get(id); ok {
		return c
	}
	c := &container{id: id, pending: make(map[pendingKey]record)}
	r.containers.Add(id, c)
	return c
}

// resolve finds the workload and the root mount of c while unknown, the root
// from the mounts of pid, a process of c, and records what waited for them.
// r.mu must be held.
func (r *Recorder) resolve(c *container, pid uint32, now time.Time) {
	if !c.known && r.image != nil {
		if image, known := r.image(c.id); known {
			c.known = true
			c.workload = workloadID{kind: usage.KeyContainer, id: c.id}
			if image != "" {
				c.workload = workloadID{kind: usage.KeyImage, id: image}
			}
		}
	}
	if c.root == 0 && pid != 0 && now.Sub(c.rootRead) >= rootReadInterval {
		c.root, c.rootRead = r.readRoot(c.id, pid), now
		if c.root != 0 {
			c.databases, c.checkedAt = r.fingerprint(utils.ProcRootPath(pid)), now
		}
	}
	if !c.known || c.root == 0 || len(c.pending) == 0 {
		return
	}
	w := r.workload(c.workload)
	w.containers[c.id] = struct{}{}
	for pk, rec := range c.pending {
		if pk.mount == c.root {
			r.recordKeys(w, []uint64{pk.key}, rec.lastSeen, rec.flags)
		}
	}
	clear(c.pending)
}

// retryPending resolves again the containers whose records wait.
func (r *Recorder) retryPending(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range r.containers.Keys() {
		if c, ok := r.containers.Peek(id); ok && len(c.pending) > 0 {
			r.resolve(c, 0, now)
		}
	}
}

// readContainerRoot returns the ID of the root mount of the container id, as
// the process pid sees it, or 0 when pid runs elsewhere or on the root of the
// host.
func (r *Recorder) readContainerRoot(id string, pid uint32) uint32 {
	cfs := utils.DefaultCGroupFS()
	if computedID, _, _, err := cfs.FindCGroupContext(pid, pid); err != nil || string(computedID) != id || r.onHostRoot(pid) {
		return 0
	}
	mounts, err := kernel.ParseMountInfoFile(int32(pid))
	if err != nil {
		return 0
	}
	var root uint32
	for _, mnt := range mounts {
		// the last mount on / hides the ones below it
		if mnt.Mountpoint == "/" {
			root = uint32(mnt.ID)
		}
	}
	return root
}

// workload returns the workload id, which it adds when unknown, letting go of
// the one active least recently at the bound. r.mu must be held.
func (r *Recorder) workload(id workloadID) *workload {
	if w, ok := r.workloads[id]; ok {
		return w
	}
	if len(r.workloads) >= r.cfg.Workloads {
		r.evictWorkload()
	}
	now := r.now()
	w := &workload{
		id:            id,
		keys:          make(map[uint64]record),
		containers:    make(map[string]struct{}),
		observedSince: now.Unix(),
		dirty:         true,
		activeAt:      now,
	}
	r.workloads[id] = w
	return w
}

// evictWorkload reports and drops the workload of containers active least
// recently. r.mu must be held.
func (r *Recorder) evictWorkload() {
	var oldest *workload
	for _, w := range r.workloads {
		if w.id.kind != usage.KeyHost && (oldest == nil || w.activeAt.Before(oldest.activeAt)) {
			oldest = w
		}
	}
	if oldest == nil {
		return
	}
	if oldest.dirty && r.send != nil {
		r.send(r.report(oldest))
	}
	r.nodeKeys -= len(oldest.keys)
	delete(r.workloads, oldest.id)
}

// recordKeys records keys as run at at with flags in w, and marks w to report
// when that is news to the core agent. r.mu must be held.
func (r *Recorder) recordKeys(w *workload, keys []uint64, at int64, flags uint8) {
	limit := r.cfg.ContainerKeys
	if w.id.kind == usage.KeyHost {
		limit = r.cfg.HostKeys
	}
	w.activeAt = r.now()
	for _, key := range keys {
		rec, ok := w.keys[key]
		if !ok {
			if len(w.keys) >= limit || r.nodeKeys >= r.cfg.NodeKeys {
				if !w.truncated {
					w.truncated, w.dirty = true, true
				}
				continue
			}
			r.nodeKeys++
		}
		rec.lastSeen = max(rec.lastSeen, at)
		rec.flags |= flags
		w.keys[key] = rec
		if !ok || rec.flags != rec.reportedFlags || time.Duration(rec.lastSeen-rec.reportedSeen)*time.Second > r.cfg.RunGap {
			w.dirty = true
		}
		if rec.lastSeen > rec.reportedSeen && w.activeAt.Sub(w.forwardedAt) > maxForwardedAge {
			w.dirty = true
		}
	}
}

// SetSend makes r report to send.
func (r *Recorder) SetSend(send func(*usage.Report)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.send = send
}

// Forward sends the report of each workload that changed since its last
// report.
func (r *Recorder) Forward(now time.Time) {
	r.mu.Lock()
	send := r.send
	var reports []*usage.Report
	for _, w := range r.workloads {
		if w.dirty {
			reports = append(reports, r.report(w))
			w.forwardedAt = now
		}
	}
	r.mu.Unlock()
	if send == nil {
		return
	}
	for _, report := range reports {
		send(report)
	}
}

// Resend marks every workload to report, as a new stream of the core agent
// holds none of them.
func (r *Recorder) Resend() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.workloads {
		w.dirty = true
	}
}

// report returns the report of w, which it marks reported. r.mu must be held.
func (r *Recorder) report(w *workload) *usage.Report {
	keys := make([]uint64, 0, len(w.keys))
	for key := range w.keys {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	report := &usage.Report{
		Keys:          keys,
		LastSeen:      make([]int64, len(keys)),
		Flags:         make([]uint8, len(keys)),
		ObservedSince: w.observedSince,
		Truncated:     w.truncated,
		KeyKind:       w.id.kind,
	}
	for i, key := range keys {
		rec := w.keys[key]
		report.LastSeen[i], report.Flags[i] = rec.lastSeen, rec.flags
		rec.reportedSeen, rec.reportedFlags = rec.lastSeen, rec.flags
		w.keys[key] = rec
	}
	switch w.id.kind {
	case usage.KeyImage:
		report.ImageID = w.id.id
		for id := range w.containers {
			report.ContainerIDs = append(report.ContainerIDs, id)
		}
		slices.Sort(report.ContainerIDs)
	case usage.KeyContainer:
		report.ContainerIDs = []string{w.id.id}
	}
	w.dirty = false
	return report
}

// OnCGroupDeleted forgets the container of cgroup once its last cgroup goes.
// The workload of an image keeps its usage, as other containers may run the
// image, and the usage of a container whose image is unknown goes with it,
// after its last report.
func (r *Recorder) OnCGroupDeleted(cgroup *cgroupModel.CacheEntry, remaining func(containerutils.ContainerID) bool) {
	if cgroup.IsContainerContextNull() {
		return
	}
	id := string(cgroup.GetContainerContext().ContainerID)
	if remaining != nil && remaining(containerutils.ContainerID(id)) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.containers.Peek(id)
	if !ok {
		return
	}
	r.containers.Remove(id)
	w, ok := r.workloads[c.workload]
	if !ok {
		return
	}
	delete(w.containers, id)
	if w.id.kind == usage.KeyContainer {
		if w.dirty && r.send != nil {
			r.send(r.report(w))
		}
		r.nodeKeys -= len(w.keys)
		delete(r.workloads, w.id)
	}
}

// CountForwarded counts a report sent to the core agent.
func (r *Recorder) CountForwarded() {
	r.forwarded.Add(1)
}

// CountForwardDropped counts n reports dropped before they reached the core
// agent.
func (r *Recorder) CountForwardDropped(n int) {
	r.forwardDropped.Add(int64(n))
}

// Stats holds the counts of a Recorder.
type Stats struct {
	Workloads        int
	Keys             int
	Truncated        int
	PendingRecords   int
	SettlingOpens    int
	PendingWorkloads int
	Drifted          int
}

// SendStats sends the counts of r.
func (r *Recorder) SendStats() error {
	if r.statsdClient == nil {
		return nil
	}
	s := r.Stats()
	for _, gauge := range []struct {
		name  string
		value int
	}{
		{metrics.MetricSBOMUsageWorkloads, s.Workloads},
		{metrics.MetricSBOMUsageKeys, s.Keys},
		{metrics.MetricSBOMUsageTruncatedWorkloads, s.Truncated},
		{metrics.MetricSBOMUsagePendingRecords, s.PendingRecords},
		{metrics.MetricSBOMUsageDriftedContainers, s.Drifted},
	} {
		if err := r.statsdClient.Gauge(gauge.name, float64(gauge.value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s: %w", gauge.name, err)
		}
	}
	for _, count := range []struct {
		name  string
		value *atomic.Int64
	}{
		{metrics.MetricSBOMUsageDroppedOpens, &r.droppedOpens},
		{metrics.MetricSBOMResolverEnrichedSBOMForwarded, &r.forwarded},
		{metrics.MetricSBOMResolverEnrichedSBOMForwardDropped, &r.forwardDropped},
	} {
		if value := count.value.Swap(0); value > 0 {
			if err := r.statsdClient.Count(count.name, value, nil, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s: %w", count.name, err)
			}
		}
	}
	return nil
}

// Stats returns the counts of r.
func (r *Recorder) Stats() Stats {
	r.mu.Lock()
	s := Stats{Workloads: len(r.workloads), Keys: r.nodeKeys}
	for _, w := range r.workloads {
		if w.truncated {
			s.Truncated++
		}
	}
	for _, id := range r.containers.Keys() {
		c, ok := r.containers.Peek(id)
		if !ok {
			continue
		}
		if len(c.pending) > 0 {
			s.PendingWorkloads++
			s.PendingRecords += len(c.pending)
		}
		if c.drifted {
			s.Drifted++
		}
	}
	r.mu.Unlock()
	r.openedMu.Lock()
	s.SettlingOpens = len(r.opened)
	r.openedMu.Unlock()
	return s
}

// onHostRoot reports whether the root of the process pid is the root of the
// host. A process of a system container or a snap runs on a root of its own
// with an empty container ID. A process whose root cannot be read, most often
// one already gone, counts as a host process.
func (r *Recorder) onHostRoot(pid uint32) bool {
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
func (r *Recorder) replaced(file *model.FileEvent) bool {
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

// holdsHostCode reports whether the host file of an open holds code, as its
// name tells or its start as an ELF file or a script does. Some packages give
// data files the mode of an executable. On a read error, the mode decides.
func (r *Recorder) holdsHostCode(file *model.FileEvent) bool {
	if CodeName(path.Base(file.PathnameStr)) {
		return true
	}
	f, err := os.OpenFile(r.hostRoot+file.PathnameStr, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return true
	}
	defer f.Close()
	head := make([]byte, 4)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return strings.HasPrefix(string(head[:n]), "\x7fELF") || strings.HasPrefix(string(head[:n]), "#!")
}
