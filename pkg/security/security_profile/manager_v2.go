// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
	ebpfmanager "github.com/DataDog/ebpf-manager"
)

type pendingProfile struct {
	firstSeen time.Time
	events    *list.List
}

type ManagerV2 struct {
	config        *config.Config
	statsdClient  statsd.ClientInterface
	resolvers     *resolvers.EBPFResolvers
	kernelVersion *kernel.Version
	newEvent      func() *model.Event
	dumpHandler   backend.ActivityDumpHandler
	ipc           ipc.Component

	sendAnomalyDetection func(*model.Event)

	hostname string

	profiles     map[cgroupModel.WorkloadSelector]*profile.Profile
	profilesLock sync.Mutex
	pathsReducer *activity_tree.PathsReducer

	eventFiltering map[eventFilteringEntry]*atomic.Uint64

	// storage
	localStorage              *storage.Directory
	remoteStorage             *storage.ActivityDumpRemoteStorageForwarder
	configuredStorageRequests map[config.StorageFormat][]config.StorageRequest

	profilePendingEvents map[containerutils.CGroupID]*pendingProfile
	pendingTimeout       *atomic.Uint64
	queueSize            *atomic.Uint64
	pendingProfiles      *atomic.Uint64
}

func NewManagerV2(cfg *config.Config, statsdClient statsd.ClientInterface, ebpf *ebpfmanager.Manager, resolvers *resolvers.EBPFResolvers, kernelVersion *kernel.Version, newEvent func() *model.Event, dumpHandler backend.ActivityDumpHandler, ipc ipc.Component, sendAnomalyDetection func(*model.Event)) (*ManagerV2, error) {

	localStorage, err := storage.NewDirectory(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory, cfg.RuntimeSecurity.ActivityDumpLocalStorageMaxDumpsCount)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the local storage: %w", err)
	}

	remoteStorage, err := storage.NewActivityDumpRemoteStorageForwarder(dumpHandler)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the remote storage forwarder: %w", err)
	}

	var configuredStorageRequests []config.StorageRequest
	for _, format := range cfg.RuntimeSecurity.ActivityDumpLocalStorageFormats {
		configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
			config.LocalStorage,
			format,
			cfg.RuntimeSecurity.ActivityDumpLocalStorageCompression,
			cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory,
		))
	}

	configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
		config.RemoteStorage,
		config.Protobuf,
		true, // force remote compression
		"",
	))

	hostname, err := hostnameutils.GetHostname(ipc)
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	return &ManagerV2{
		config:                    cfg,
		statsdClient:              statsdClient,
		resolvers:                 resolvers,
		kernelVersion:             kernelVersion,
		ipc:                       ipc,
		profilePendingEvents:      make(map[containerutils.CGroupID]*pendingProfile),
		pendingTimeout:            atomic.NewUint64(0),
		queueSize:                 atomic.NewUint64(0),
		pendingProfiles:           atomic.NewUint64(0),
		pathsReducer:              activity_tree.NewPathsReducer(),
		profiles:                  make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		localStorage:              localStorage,
		remoteStorage:             remoteStorage,
		configuredStorageRequests: perFormatStorageRequests(configuredStorageRequests),
		hostname:                  hostname,
		sendAnomalyDetection:      sendAnomalyDetection,
		eventFiltering:            make(map[eventFilteringEntry]*atomic.Uint64),
	}, nil
}

func (m *ManagerV2) Start(ctx context.Context) {
	var sendTickerChan <-chan time.Time

	if m.config.RuntimeSecurity.SecurityProfileEnabled {
		sendTicker := time.NewTicker(m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout)
		defer sendTicker.Stop()
		sendTickerChan = sendTicker.C
	} else {
		sendTickerChan = make(chan time.Time)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	seclog.Infof("security profile manager started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-sendTickerChan:
			for _, p := range m.profiles {
				format := config.Protobuf
				requests := m.configuredStorageRequests[format]

				// encode profile
				data, err := p.Encode(format)
				if err != nil {
					seclog.Errorf("couldn't encode profile [%s] to %s format: %v", p.GetSelectorStr(), format, err)
					continue
				}

				for _, request := range requests {
					var storage storage.ActivityDumpStorage
					switch request.Type {
					case config.LocalStorage:
						storage = m.localStorage
					case config.RemoteStorage:
						storage = m.remoteStorage
					default:
						seclog.Errorf("couldn't persist [%s] to %s format: unknown storage type: %s", p.GetSelectorStr(), format, request.Type)
						continue
					}

					if err := storage.Persist(request, p, data); err != nil {
						seclog.Errorf("couldn't persist [%s] to %s storage: %v", p.GetSelectorStr(), request.Type, err)
					} else {
						tags := []string{"format:" + request.Format.String(), "storage_type:" + request.Type.String(), fmt.Sprintf("compression:%v", request.Compression)}
						if err := m.statsdClient.Count(metrics.MetricActivityDumpSizeInBytes, int64(data.Len()), tags, 1.0); err != nil {
							seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpSizeInBytes, err)
						}
						if err := m.statsdClient.Count(metrics.MetricActivityDumpPersistedDumps, 1, tags, 1.0); err != nil {
							seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpPersistedDumps, err)
						}
					}
				}

				p.SetHasAlreadyBeenSent()
			}
		}
	}
}

func (m *ManagerV2) ProcessEvent(event *model.Event) {
	if !event.IsActivityDumpSample() {
		return
	}

	processEvent := func(event *model.Event) {
		if profile, inserted := m.handleEvent(event); inserted && profile.HasAlreadyBeenSent() {
			var workloadID containerutils.WorkloadID
			var imageTag string
			if containerID := event.FieldHandlers.ResolveContainerID(event, &event.ProcessContext.Process.ContainerContext); containerID != "" {
				workloadID = containerID
				imageTag = utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
			} else if cgroupID := event.FieldHandlers.ResolveCGroupID(event, &event.ProcessContext.Process.CGroup); cgroupID != "" {
				workloadID = containerutils.CGroupID(cgroupID)
				tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
				if err != nil {
					seclog.Errorf("failed to resolve tags for cgroup %s: %v", workloadID, err)
					return
				}
				imageTag = utils.GetTagValue("version", tags)
			}

			if workloadID != nil {
				m.FillProfileContextFromWorkloadID(workloadID, &event.SecurityProfileContext, imageTag)
			}

			if m.config.RuntimeSecurity.AnomalyDetectionEnabled {
				m.sendAnomalyDetection(event)
			}
		}
	}

	// purge silent cgroups
	for cgroupID, pendingEvents := range m.profilePendingEvents {
		if event.Timestamp.Sub(pendingEvents.firstSeen) > 60*time.Second {
			delete(m.profilePendingEvents, cgroupID)
			m.pendingProfiles.Dec()
		}
	}

	pendingEvents := m.profilePendingEvents[event.ProcessContext.Process.CGroup.CGroupID]

	// tags already resolved, no need to queue the event
	event.FieldHandlers.ResolveContainerTags(event, &event.ProcessContext.Process.ContainerContext)
	if len(event.ProcessContext.Process.ContainerContext.Tags) != 0 {
		// dequeue the events
		if pendingEvents != nil {

			l := pendingEvents.events.Len()
			for e := pendingEvents.events.Front(); e != nil; e = e.Next() {
				processEvent(e.Value.(*model.Event))
				l--
			}
			m.queueSize.Sub(uint64(l))

			delete(m.profilePendingEvents, event.ProcessContext.Process.CGroup.CGroupID)
		}

		processEvent(event)

		return
	}

	if pendingEvents == nil {
		pendingEvents = &pendingProfile{
			firstSeen: event.Timestamp,
			events:    list.New(),
		}
		m.profilePendingEvents[event.ProcessContext.Process.CGroup.CGroupID] = pendingEvents

		m.pendingProfiles.Inc()
	}

	event.ResolveEventTime()

	if event.Timestamp.Sub(pendingEvents.firstSeen) > 10*time.Second {
		// ignore the event, it is already too late.
		// keep the pending struct entry a bit to avoid reintroducing late events in the queue

		if pendingEvents.events.Len() > 0 {
			pendingEvents.events.Init()
			m.pendingTimeout.Inc()
		}
		return
	}

	// resolve the fields to prepare the copy, required to put the event in the queue
	event.ResolveFieldsForAD()

	// marshal the event in JSON
	cpy := event.DeepCopy()

	pendingEvents.events.PushBack(cpy)

	m.queueSize.Inc()
}

func (m *ManagerV2) SendStats() error {
	valuePendingTimeout := m.pendingTimeout.Swap(0)
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionTimeout, int64(valuePendingTimeout), []string{}, 1.0); err != nil {
		return err
	}

	valuePendingProfiles := m.pendingProfiles.Load()
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2ProfilePending, float64(valuePendingProfiles), []string{}, 1.0); err != nil {
		return err
	}

	valueQueueSize := m.queueSize.Load()
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2QueueSize, float64(valueQueueSize), []string{}, 1.0); err != nil {
		return err
	}

	return nil
}

func (m *ManagerV2) handleEvent(event *model.Event) (*profile.Profile, bool) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return nil, false
	}

	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", event.ProcessContext.Process.ContainerContext.Tags), "*")
	if err != nil {
		return nil, false
	}

	m.profilesLock.Lock()

	secprof := m.profiles[selector]
	if secprof == nil {
		secprof = profile.New(
			profile.WithPathsReducer(m.pathsReducer),
			profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
			profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
			profile.WithEventTypes(m.config.RuntimeSecurity.ActivityDumpTracedEventTypes),
			profile.WithWorkloadSelector(selector),
		)
		secprof.SetTreeType(secprof, "security_profile")

		secprof.Metadata = mtdt.Metadata{
			AgentVersion:      version.AgentVersion,
			AgentCommit:       version.Commit,
			KernelVersion:     m.kernelVersion.Code.String(),
			LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
			Arch:              utils.RuntimeArch(),

			Name:              fmt.Sprintf("activity-dump-%s", utils.RandString(10)),
			ProtobufVersion:   profile.ProtobufVersion,
			DifferentiateArgs: m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
			ContainerID:       event.ProcessContext.Process.ContainerContext.ContainerID,
			CGroupContext:     *&event.ProcessContext.Process.CGroup,
			Start:             event.ResolveEventTime(),
			End:               event.ResolveEventTime(),
		}
		secprof.Header.Host = m.hostname
		secprof.Header.Source = ActivityDumpSource

		var workloadID any
		if len(secprof.Metadata.ContainerID) > 0 {
			workloadID = containerutils.ContainerID(secprof.Metadata.ContainerID)
		} else if len(secprof.Metadata.CGroupContext.CGroupID) > 0 {
			workloadID = secprof.Metadata.CGroupContext.CGroupID
		}

		if workloadID != nil {
			tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
			if err != nil {
				return nil, false

			}
			secprof.AddTags(tags)
		}

		m.profiles[selector] = secprof
	}

	m.profilesLock.Unlock()

	if secprof.ActivityTree.Stats.ApproximateSize() >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
		m.incrementEventFilteringStat(event.GetEventType(), model.ProfileAtMaxSize, NA)
		return nil, false
	}

	if _, ok := secprof.GetVersionContext(selector.Tag); !ok {
		now := time.Now()
		nowNano := uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(now))
		tags := secprof.GetTags()
		vCtx := &profile.VersionContext{
			FirstSeenNano:  nowNano,
			LastSeenNano:   nowNano,
			EventTypeState: make(map[model.EventType]*profile.EventTypeState),
			Syscalls:       secprof.ComputeSyscallsList(),
			Tags:           make([]string, len(tags)),
		}
		copy(vCtx.Tags, tags)

		secprof.AddVersionContext(selector.Tag, vCtx)
	}

	imageTag := secprof.GetTagValue("image_tag")
	inserted, err := secprof.Insert(event, true, imageTag, activity_tree.Runtime, m.resolvers)
	if err != nil {
		return nil, false
	}

	return secprof, inserted
}

// FillProfileContextFromWorkloadID fills the given ctx with workload id infos
func (m *ManagerV2) FillProfileContextFromWorkloadID(id containerutils.WorkloadID, ctx *model.SecurityProfileContext, imageTag string) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, profile := range m.profiles {
		profile.InstancesLock.Lock()
		for _, instance := range profile.Instances {
			instance.Lock()
			if instance.GetWorkloadID() == id {
				ctx.Name = profile.Metadata.Name
				profileContext, ok := profile.GetVersionContext(imageTag)
				if ok { // should always be the case
					ctx.Tags = profileContext.Tags
				}
			}
			instance.Unlock()
		}
		profile.InstancesLock.Unlock()
	}
}

func (m *ManagerV2) incrementEventFilteringStat(eventType model.EventType, state model.EventFilteringProfileState, result EventFilteringResult) {
	if entry, ok := m.eventFiltering[eventFilteringEntry{eventType, state, result}]; ok {
		entry.Inc()
	}
}

// LookupEventInProfiles lookups event in profiles
// In V2, this is a no-op as event filtering is handled differently through ProcessEvent
func (m *ManagerV2) LookupEventInProfiles(_ *model.Event) {
	// V2 handles event processing differently - events are processed through ProcessEvent
	// which builds profiles from activity dump samples. The profile lookup/filtering
	// logic from V1 is not applicable to the V2 lifecycle.
}

// HasActiveActivityDump returns true if the given event has an active dump
// In V2, we don't manage activity dumps the traditional way, so always return false
func (m *ManagerV2) HasActiveActivityDump(_ *model.Event) bool {
	// V2 doesn't use the traditional activity dump mechanism with kernel-space traced cgroups.
	// Instead, it builds profiles directly from activity dump samples.
	return false
}

// HandleCGroupTracingEvent handles a cgroup tracing event
// In V2, this is a no-op as we don't manage cgroup tracing the traditional way
func (m *ManagerV2) HandleCGroupTracingEvent(_ *model.CgroupTracingEvent) {
	// V2 doesn't use cgroup tracing events from kernel space.
	// Profiles are built from activity dump samples instead.
}

// SyncTracedCgroups recovers lost CGroup tracing events by going through the kernel space map of cgroups
// In V2, this is a no-op as we don't manage traced cgroups maps
func (m *ManagerV2) SyncTracedCgroups() {
	// V2 doesn't manage kernel-space traced cgroups maps.
	// This method is kept for interface compatibility with V1.
}

// evictUnusedNodes performs periodic eviction of non-touched nodes from all active profiles
func (m *ManagerV2) evictUnusedNodes() {
	if m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout <= 0 {
		return
	}

	evictionTime := time.Now().Add(-m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout)
	totalEvicted := 0

	filepathsInProcessCache := m.GetNodesInProcessCache()

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for selector, profile := range m.profiles {
		if profile == nil {
			continue
		}

		profile.Lock()
		if profile.ActivityTree == nil {
			profile.Unlock()
			continue
		}
		evicted := profile.ActivityTree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, selector.Image, selector.Tag)
		if evicted > 0 {
			totalEvicted += evicted
			seclog.Debugf("evicted %d unused process nodes from profile [%s] ", evicted, selector.String())
		}
		profile.Unlock()
	}

	if totalEvicted > 0 {
		seclog.Infof("evicted %d total unused process nodes across all profiles", totalEvicted)
	}
}

// GetNodesInProcessCache returns a map with ImageProcessKey as key and bool as value for all filepaths in the process cache
func (m *ManagerV2) GetNodesInProcessCache() map[activity_tree.ImageProcessKey]bool {

	cgr := m.resolvers.CGroupResolver
	pr := m.resolvers.ProcessResolver
	tagsResolver := m.resolvers.TagsResolver

	type imageTagKey struct {
		imageName string
		imageTag  string
	}

	pids := make(map[imageTagKey][]uint32)

	result := make(map[activity_tree.ImageProcessKey]bool)

	cgr.Iterate(func(cgce *cgroupModel.CacheEntry) bool {
		cgce.Lock()
		defer cgce.Unlock()

		var cgceTags []string
		var err error
		var imageName, imageTag string
		if cgce.ContainerID != "" {
			cgceTags, err = tagsResolver.ResolveWithErr(cgce.ContainerID)
			if err != nil {
				return false
			}
			imageName = utils.GetTagValue("image_name", cgceTags)
			imageTag = utils.GetTagValue("image_tag", cgceTags)
		} else if cgce.CGroupID != "" {
			cgceTags, err = tagsResolver.ResolveWithErr(cgce.CGroupID)
			if err != nil {
				return false
			}
			imageName = utils.GetTagValue("service", cgceTags)
			imageTag = utils.GetTagValue("version", cgceTags)
		} else {
			return false
		}

		if imageTag == "" {
			imageTag = "latest"
		}

		imageTagKey := imageTagKey{
			imageName: imageName,
			imageTag:  imageTag,
		}
		for pid := range cgce.PIDs {
			pids[imageTagKey] = append(pids[imageTagKey], pid)
		}

		return false
	})

	// we do the resolution of filepaths here so that we can release the cgroup resolver lock before acquiring the process resolver lock
	for k, pids := range pids {

		key := activity_tree.ImageProcessKey{
			ImageName: k.imageName,
			ImageTag:  k.imageTag,
			Filepath:  "",
		}

		for _, pid := range pids {
			pce := pr.Resolve(pid, pid, 0, true, nil)
			if pce == nil {
				seclog.Warnf("couldn't resolve process cache entry for pid %d, this process may have exited", pid)
				continue
			}

			key.Filepath = pce.FileEvent.PathnameStr
			result[key] = true
		}
	}

	return result
}
