// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// nodeState holds the mutable fields v3 tracks per node identity. The identity itself is the map
// key (a hash); these values change as the node is seen again. flags and mode are only meaningful
// for file-open nodes.
type nodeState struct {
	lastSeen int64
	flags    uint32
	mode     uint32
}

// eventItem is one node an event touches: its identity hash plus, for file opens, the open flags
// and mode that accumulate on the node.
type eventItem struct {
	hash      [16]byte
	flags     uint32
	mode      uint32
	fileState bool
}

// NodeRefresh carries the latest last_seen for a node that recurred without producing anything new.
// It rides along on the next anomaly_detection event for the same workload (piggybacking) so the
// backend can advance the node's last_seen without the agent re-sending the whole node.
type NodeRefresh struct {
	Hash     [16]byte
	LastSeen int64
}

// ManagerV3 is an experimental Security Profile manager. It keeps no local activity tree and never
// sends a profile: it only emits anomaly_detection events for behavior it has not seen before,
// deduplicated by a per-workload hash set, and leaves profile reconstruction to the backend.
//
// It is intentionally standalone (it does not embed ManagerV2) so it can be freely hacked on for
// the PoC. It copies the small amount of plumbing it needs (tag-resolution buffering, cgroup
// tracking) rather than inheriting ManagerV2's profile/tree machinery.
type ManagerV3 struct {
	config       *config.Config
	statsdClient statsd.ClientInterface
	resolvers    *resolvers.EBPFResolvers
	// sendAnomalyDetection emits a full anomaly_detection event. nodeHashes are the identity hashes
	// of the nodes this event represents (so the backend can index node to hash); refreshes are the
	// piggybacked last_seen updates for other nodes of the same workload that recurred unchanged;
	// snapshot marks events synthesized from the startup /proc walk so the backend tags their nodes
	// with the Snapshot generation type.
	sendAnomalyDetection func(event *model.Event, nodeHashes [][16]byte, refreshes []NodeRefresh, snapshot bool)
	// newEvent builds a blank, resolver-backed event, used to synthesize snapshot events.
	newEvent func() *model.Event
	hostname string

	// reducer canonicalizes paths with deterministic rules (pids, container ids, ...) before
	// hashing, the same reduction v2 applies to file nodes. v3 deliberately does not do v2's
	// learned path pattern generalization, which is stateful and left to the backend.
	reducer           *activity_tree.PathsReducer
	differentiateArgs bool

	// Events buffered while their workload tags are still resolving.
	profilePendingEvents     map[containerutils.CGroupID]*pendingProfile
	profilePendingEventsLock sync.Mutex
	queueSize                *atomic.Uint64
	pendingProfiles          *atomic.Uint64

	resolvedCgroups     map[containerutils.CGroupID]struct{}
	resolvedCgroupsLock sync.Mutex

	// Per-workload map from node identity hash to its mutable state.
	seen map[cgroupModel.WorkloadSelector]map[[16]byte]*nodeState
	// Per-workload last_seen refreshes for nodes that recurred unchanged, waiting to piggyback on
	// the next emitted event for that workload. Guarded by seenLock.
	pendingRefresh map[cgroupModel.WorkloadSelector]map[[16]byte]int64
	seenLock       sync.Mutex
}

// NewManagerV3 returns a new ManagerV3. It keeps the same signature as NewManagerV2 so the probe
// can build either without special-casing; the storage/kernel/filter arguments are unused by v3.
func NewManagerV3(cfg *config.Config, statsdClient statsd.ClientInterface, resolvers *resolvers.EBPFResolvers, _ *kernel.Version, _ backend.ActivityDumpHandler, sendAnomalyDetection func(event *model.Event, nodeHashes [][16]byte, refreshes []NodeRefresh, snapshot bool), newEvent func() *model.Event, hostname string, _ workloadfilter.Component) (*ManagerV3, error) {
	return &ManagerV3{
		config:               cfg,
		statsdClient:         statsdClient,
		resolvers:            resolvers,
		sendAnomalyDetection: sendAnomalyDetection,
		newEvent:             newEvent,
		hostname:             hostname,
		reducer:              activity_tree.NewPathsReducer(),
		differentiateArgs:    cfg.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
		profilePendingEvents: make(map[containerutils.CGroupID]*pendingProfile),
		queueSize:            atomic.NewUint64(0),
		pendingProfiles:      atomic.NewUint64(0),
		resolvedCgroups:      make(map[containerutils.CGroupID]struct{}),
		seen:                 make(map[cgroupModel.WorkloadSelector]map[[16]byte]*nodeState),
		pendingRefresh:       make(map[cgroupModel.WorkloadSelector]map[[16]byte]int64),
	}, nil
}

// Start runs the v3 background loop: it only purges stale buffered events and cleans up state on
// cgroup deletion. v3 keeps no profiles, so there is nothing to persist, evict or clean up.
func (m *ManagerV3) Start(ctx context.Context) {
	if err := m.resolvers.CGroupResolver.RegisterListener(cgroup.CGroupDeleted, m.onCGroupDeleted); err != nil {
		seclog.Errorf("failed to register cgroup deletion listener: %v", err)
	}

	go m.snapshotExistingProcesses()

	stalePurge := time.NewTicker(10 * time.Second)
	defer stalePurge.Stop()

	seclog.Infof("security profile manager v3 started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-stalePurge.C:
			m.purgeStalePendingEvents(time.Now())
		}
	}
}

// ProcessEvent buffers events until their workload tags resolve, then emits an anomaly_detection
// event the first time a given behavior is seen for a workload. Novelty is decided by a
// per-workload hash of the event identity, not by an activity tree.
func (m *ManagerV3) ProcessEvent(event *model.Event) {
	if !slices.Contains(m.config.RuntimeSecurity.SecurityProfileV2EventTypes, model.EventType(event.Type)) {
		return
	}
	if event.ProcessContext.Process.ContainerContext.IsNull() {
		return
	}

	workloadID := getWorkloadIDFromEvent(event)
	if workloadID == nil {
		return
	}

	source := event.FieldHandlers.ResolveSource(event, &event.BaseEvent)
	metricTags := []string{"source:" + source, "event_type:" + event.GetType()}

	workloadTags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
	if err == nil && len(workloadTags) != 0 && utils.GetTagValue("image_tag", workloadTags) != "" {
		event.ProcessContext.Process.ContainerContext.Tags = workloadTags
		m.processResolved(event)
	} else {
		m.queueEventForTagResolution(event, metricTags)
	}
}

// processResolved drains any events buffered for the same cgroup while tags were pending, then
// handles the current event.
func (m *ManagerV3) processResolved(event *model.Event) {
	cgroupID := event.ProcessContext.Process.CGroup.CGroupID

	m.resolvedCgroupsLock.Lock()
	m.resolvedCgroups[cgroupID] = struct{}{}
	m.resolvedCgroupsLock.Unlock()

	m.profilePendingEventsLock.Lock()
	if pendingEvents := m.profilePendingEvents[cgroupID]; pendingEvents != nil {
		for e := pendingEvents.events.Front(); e != nil; e = e.Next() {
			queued := e.Value.(*model.Event)
			queued.ProcessContext.Process.ContainerContext.Tags = event.ProcessContext.Process.ContainerContext.Tags
			m.emitIfNovel(queued, false)
		}
		m.queueSize.Sub(uint64(pendingEvents.events.Len()))
		m.pendingProfiles.Dec()
		delete(m.profilePendingEvents, cgroupID)
	}
	m.profilePendingEventsLock.Unlock()

	m.emitIfNovel(event, false)
}

// queueEventForTagResolution buffers an event while waiting for its workload tags to resolve.
func (m *ManagerV3) queueEventForTagResolution(event *model.Event, tags []string) {
	cgroupID := event.ProcessContext.Process.CGroup.CGroupID

	m.profilePendingEventsLock.Lock()
	defer m.profilePendingEventsLock.Unlock()

	pendingEvents := m.profilePendingEvents[cgroupID]
	if pendingEvents == nil {
		pendingEvents = &pendingProfile{
			firstSeen: event.Timestamp,
			events:    list.New(),
		}
		m.profilePendingEvents[cgroupID] = pendingEvents
		m.pendingProfiles.Inc()
	}

	event.ResolveEventTime()
	if event.Timestamp.Sub(pendingEvents.firstSeen) > 10*time.Second {
		if eventsLen := pendingEvents.events.Len(); eventsLen > 0 {
			m.queueSize.Sub(uint64(eventsLen))
			pendingEvents.events.Init()
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionEventsDropped, int64(eventsLen), tags, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionEventsDropped, err)
			}
		}
		return
	}

	event.ResolveFieldsForAD()
	pendingEvents.events.PushBack(event.DeepCopy())
	m.queueSize.Inc()
}

// purgeStalePendingEvents drops buffered events whose cgroup has been waiting for tags for over 60s.
func (m *ManagerV3) purgeStalePendingEvents(now time.Time) {
	m.profilePendingEventsLock.Lock()
	defer m.profilePendingEventsLock.Unlock()

	for cgroupID, pendingEvents := range m.profilePendingEvents {
		if now.Sub(pendingEvents.firstSeen) <= 60*time.Second {
			continue
		}
		if eventsLen := pendingEvents.events.Len(); eventsLen > 0 {
			m.queueSize.Sub(uint64(eventsLen))
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionEventsDropped, int64(eventsLen), []string{}, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionEventsDropped, err)
			}
		}
		delete(m.profilePendingEvents, cgroupID)
		m.pendingProfiles.Dec()
	}
}

// onCGroupDeleted drops any state we were keeping for a cgroup that no longer exists.
func (m *ManagerV3) onCGroupDeleted(cgce *cgroupModel.CacheEntry) {
	cgroupID := cgce.GetCGroupID()

	m.resolvedCgroupsLock.Lock()
	delete(m.resolvedCgroups, cgroupID)
	m.resolvedCgroupsLock.Unlock()

	m.profilePendingEventsLock.Lock()
	if pendingEvents := m.profilePendingEvents[cgroupID]; pendingEvents != nil {
		m.queueSize.Sub(uint64(pendingEvents.events.Len()))
		m.pendingProfiles.Dec()
		delete(m.profilePendingEvents, cgroupID)
	}
	m.profilePendingEventsLock.Unlock()
}

// emitIfNovel sends an anomaly_detection event the first time an event's identity is seen for its
// workload. snapshot is true for events synthesized from the startup /proc walk.
func (m *ManagerV3) emitIfNovel(event *model.Event, snapshot bool) {
	selector, err := m.buildWorkloadSelector(event)
	if err != nil {
		return
	}

	event.ResolveFieldsForAD()
	items := m.eventItems(event)
	emit, refreshes := m.record(selector, items, event.ResolveEventTime().UnixNano())
	if !emit {
		return
	}

	if !m.config.RuntimeSecurity.AnomalyDetectionEnabled {
		return
	}

	workloadID := getWorkloadIDFromEvent(event)
	imageTag := utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
	if workloadID != nil {
		m.FillProfileContextFromWorkloadID(workloadID, &event.SecurityProfileContext, imageTag)
	}

	nodeHashes := make([][16]byte, len(items))
	for i, item := range items {
		nodeHashes[i] = item.hash
	}
	m.sendAnomalyDetection(event, nodeHashes, refreshes, snapshot)
}

// record updates the stored state for every node an event touches. It returns whether the event
// should be emitted (a new node identity appeared, or a file-open node gained flag/mode bits it had
// not reported before), and the batch of piggybacked last_seen refreshes to attach if it is.
//
// last_seen is advanced in place on every node. For a node that recurs with nothing new, that
// advance is staged in pendingRefresh and travels to the backend on the next emitted event for the
// workload. When the event is emitted its own nodes are cleared from pendingRefresh, since the full
// event already carries their current last_seen.
func (m *ManagerV3) record(selector cgroupModel.WorkloadSelector, items []eventItem, now int64) (bool, []NodeRefresh) {
	if len(items) == 0 {
		return false, nil
	}

	m.seenLock.Lock()
	defer m.seenLock.Unlock()

	set := m.seen[selector]
	if set == nil {
		set = make(map[[16]byte]*nodeState)
		m.seen[selector] = set
	}
	staged := m.pendingRefresh[selector]

	emit := false
	for _, item := range items {
		state := set[item.hash]
		if state == nil {
			set[item.hash] = &nodeState{lastSeen: now, flags: item.flags, mode: item.mode}
			emit = true
			continue
		}

		state.lastSeen = now
		if item.fileState && (item.flags&^state.flags != 0 || item.mode&^state.mode != 0) {
			state.flags |= item.flags
			state.mode |= item.mode
			emit = true
			continue
		}

		if staged == nil {
			staged = make(map[[16]byte]int64)
			m.pendingRefresh[selector] = staged
		}
		staged[item.hash] = now
	}

	if !emit {
		return false, nil
	}

	for _, item := range items {
		delete(staged, item.hash)
	}
	var refreshes []NodeRefresh
	if len(staged) > 0 {
		refreshes = make([]NodeRefresh, 0, len(staged))
		for hash, lastSeen := range staged {
			refreshes = append(refreshes, NodeRefresh{Hash: hash, LastSeen: lastSeen})
		}
		delete(m.pendingRefresh, selector)
	}
	return true, refreshes
}

// buildWorkloadSelector keys v3's seen-set by image name AND tag, unlike v2 which keys profiles by
// image only. Per-tag keying makes each tag re-emit its full behavior set so the backend can
// rebuild a complete profile per host + image tag.
func (m *ManagerV3) buildWorkloadSelector(event *model.Event) (cgroupModel.WorkloadSelector, error) {
	imageName := utils.GetTagValue("image_name", event.ProcessContext.Process.ContainerContext.Tags)
	imageTag := utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
	return cgroupModel.NewWorkloadSelector(imageName, imageTag)
}

// eventItems returns the node identities an event touches in v2's activity tree: one type-agnostic
// process-node item for the process lineage, plus one leaf item per node the event's type keys
// (file path, dns name+type, bind tuple, imds struct, and the per-item sets for syscalls,
// capabilities and network flows). For file opens, flags and mode are not part of the identity;
// they ride along as mutable state the node accumulates. Paths are canonicalized with the same
// rule-based reducer v2 uses, but not v2's learned pattern generalization. Hashes are SHA-256
// truncated to 128 bits so a different real behavior cannot be crafted to collide with an
// already-seen one.
func (m *ManagerV3) eventItems(event *model.Event) []eventItem {
	var base bytes.Buffer
	for entry := event.ProcessCacheEntry; entry != nil; entry = activity_tree.GetNextAncestorBinaryOrArgv0(&entry.ProcessContext) {
		io.WriteString(&base, m.reducePath(entry.Process.FileEvent.PathnameStr, &entry.Process.FileEvent, entry.Process.Pid))
		base.WriteByte(1)
		if sprocess.IsBusybox(entry.Process.FileEvent.PathnameStr) {
			argv0, _ := sprocess.GetProcessArgv0(&entry.Process)
			io.WriteString(&base, argv0)
		}
		base.WriteByte(2)
		if m.differentiateArgs {
			argv, _ := sprocess.GetProcessArgv(&entry.Process)
			for _, arg := range argv {
				io.WriteString(&base, arg)
				base.WriteByte(3)
			}
		}
		base.WriteByte(4)
	}
	prefix := base.Bytes()

	eventType := event.GetType()

	// Exit creates no node in v2, but v3 has no profile, so it emits one exit item per process
	// lineage to carry the exit time to the backend. It deliberately does not contribute the
	// process-node item, so a process whose only event is an exit is not reported (matching v2,
	// which never emits on exit).
	if event.GetEventType() == model.ExitEventType {
		return []eventItem{{hash: leafHash(prefix, func(w io.Writer) {
			io.WriteString(w, eventType)
		})}}
	}

	// Every event carries the process-node item so that the first event to surface a process
	// lineage (usually exec) emits, and later events on the same lineage do not re-emit it. For
	// event types v2 does not key further (exec, connect, anything not handled below) this is the
	// only item, matching v2 which yields only process-node novelty for them.
	items := []eventItem{{hash: leafHash(prefix, nil)}}

	switch event.GetEventType() {
	case model.FileOpenEventType:
		filePath := event.FieldHandlers.ResolveFilePath(event, &event.Open.File)
		items = append(items, eventItem{
			hash: leafHash(prefix, func(w io.Writer) {
				io.WriteString(w, eventType)
				io.WriteString(w, m.reducePath(filePath, &event.Open.File, event.ProcessContext.Process.Pid))
			}),
			flags:     event.Open.Flags,
			mode:      event.Open.Mode,
			fileState: true,
		})
	case model.DNSEventType:
		items = append(items, eventItem{hash: leafHash(prefix, func(w io.Writer) {
			io.WriteString(w, eventType)
			io.WriteString(w, event.DNS.Question.Name)
			io.WriteString(w, "\x00")
			io.WriteString(w, strconv.Itoa(int(event.DNS.Question.Type)))
		})})
	case model.BindEventType:
		items = append(items, eventItem{hash: leafHash(prefix, func(w io.Writer) {
			io.WriteString(w, eventType)
			io.WriteString(w, model.AddressFamily(event.Bind.AddrFamily).String())
			io.WriteString(w, "\x00")
			io.WriteString(w, utils.GetIPStringFromIPNet(event.Bind.Addr.IPNet))
			io.WriteString(w, "\x01")
			io.WriteString(w, strconv.Itoa(int(event.Bind.Addr.Port)))
			io.WriteString(w, "\x02")
			io.WriteString(w, strconv.Itoa(int(event.Bind.Protocol)))
		})})
	case model.IMDSEventType:
		items = append(items, eventItem{hash: leafHash(prefix, func(w io.Writer) {
			io.WriteString(w, eventType)
			fmt.Fprintf(w, "%+v", event.IMDS)
		})})
	case model.SyscallsEventType:
		for _, syscall := range event.Syscalls.Syscalls {
			items = append(items, eventItem{hash: leafHash(prefix, func(w io.Writer) {
				io.WriteString(w, eventType)
				io.WriteString(w, strconv.Itoa(int(syscall)))
			})})
		}
	case model.CapabilitiesEventType:
		for capability := 0; capability < 64; capability++ {
			if event.CapabilitiesUsage.Attempted&(1<<uint(capability)) == 0 {
				continue
			}
			capable := event.CapabilitiesUsage.Used&(1<<uint(capability)) != 0
			items = append(items, eventItem{hash: leafHash(prefix, func(w io.Writer) {
				io.WriteString(w, eventType)
				io.WriteString(w, strconv.Itoa(capability))
				io.WriteString(w, "\x00")
				io.WriteString(w, strconv.FormatBool(capable))
			})})
		}
	case model.NetworkFlowMonitorEventType:
		for i := range event.NetworkFlowMonitor.Flows {
			flow := event.NetworkFlowMonitor.Flows[i]
			items = append(items, eventItem{hash: leafHash(prefix, func(w io.Writer) {
				io.WriteString(w, eventType)
				fmt.Fprintf(w, "%+v", event.NetworkFlowMonitor.Device)
				io.WriteString(w, "\x00")
				fmt.Fprintf(w, "%+v", flow.GetFiveTuple())
			})})
		}
	}

	return items
}

// leafHash hashes the lineage prefix plus an optional leaf. A nil leaf yields the type-agnostic
// process-node item; a non-nil leaf writes its own event-type tag so leaves of different types
// never collide. The nil/non-nil split keeps the process item distinct from any leaf item.
func leafHash(prefix []byte, writeLeaf func(io.Writer)) [16]byte {
	h := sha256.New()
	if writeLeaf == nil {
		h.Write([]byte{0})
	} else {
		h.Write([]byte{1})
	}
	h.Write(prefix)
	if writeLeaf != nil {
		writeLeaf(h)
	}
	var out [16]byte
	copy(out[:], h.Sum(nil))
	return out
}

// reducePath canonicalizes a path with the rule-based reducer. The reducer only reads the pid
// from the process node (for its /proc/<pid>/ rule), so a throwaway node carrying just the pid is
// enough.
func (m *ManagerV3) reducePath(path string, fileEvent *model.FileEvent, pid uint32) string {
	var pn activity_tree.ProcessNode
	pn.Process.Pid = pid
	return m.reducer.ReducePath(path, fileEvent, &pn)
}

// FillProfileContextFromWorkloadID stamps the emitted event with the workload's image tags so the
// backend can key the rebuilt profile by host + image tag. v3 has no local profile to read from.
func (m *ManagerV3) FillProfileContextFromWorkloadID(id containerutils.WorkloadID, ctx *model.SecurityProfileContext, imageTag string) {
	workloadTags, err := m.resolvers.TagsResolver.ResolveWithErr(id)
	if err != nil {
		return
	}
	ctx.Name = utils.GetTagValue("image_name", workloadTags)
	ctx.Version = imageTag
	ctx.Tags = workloadTags
}

// SendStats is a no-op for the PoC.
func (m *ManagerV3) SendStats() error { return nil }

// SyncTracedCgroups is a no-op: v3 does not manage kernel-space traced cgroups.
func (m *ManagerV3) SyncTracedCgroups() {}

// HandleCGroupTracingEvent is a no-op: v3 does not use kernel-space cgroup tracing events.
func (m *ManagerV3) HandleCGroupTracingEvent(_ *model.CgroupTracingEvent) {}

// HandleSampleRefresh is a no-op in V3: it keeps no activity dump sampling state.
func (m *ManagerV3) HandleSampleRefresh(_ uint64) {}

// LookupEventInProfiles is a no-op: v3 keeps no local profiles to filter against.
func (m *ManagerV3) LookupEventInProfiles(_ *model.Event) {}

// HasActiveActivityDump always returns false: v3 has no activity dumps.
func (m *ManagerV3) HasActiveActivityDump(_ *model.Event) bool { return false }

// ListSecurityProfiles returns nothing: v3 keeps no local profiles.
func (m *ManagerV3) ListSecurityProfiles(_ *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	return nil, nil
}

// SaveSecurityProfile is a no-op: v3 keeps no local profiles.
func (m *ManagerV3) SaveSecurityProfile(_ *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	return nil, nil
}

// GenerateTranscoding is a no-op in v3.
func (m *ManagerV3) GenerateTranscoding(_ *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	return nil, nil
}

// ListActivityDumps is a no-op in v3.
func (m *ManagerV3) ListActivityDumps(_ *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	return nil, nil
}

// StopActivityDump is a no-op in v3.
func (m *ManagerV3) StopActivityDump(_ *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	return nil, nil
}

// DumpActivity is a no-op in v3.
func (m *ManagerV3) DumpActivity(_ *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	return nil, nil
}
