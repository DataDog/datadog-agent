// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"errors"
	"fmt"
	"time"

	"github.com/cilium/ebpf"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ProcessEvent processes a new event and insert it in an activity dump if applicable
func (m *Manager) ProcessEvent(event *model.Event) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	if event.Error != nil {
		return
	}

	if !event.IsActivityDumpSample() {
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	for _, ad := range m.activeDumps {
		inserted, size, _ := ad.Insert(event, m.resolvers)
		if inserted && size >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
			if err := m.pauseKernelEventCollection(ad); err != nil {
				seclog.Warnf("couldn't pause max-sized activity dump: %v", err)
			}
		}
	}
}

// HasActiveActivityDump returns true if the given event has an active dump
func (m *Manager) HasActiveActivityDump(event *model.Event) bool {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return false
	}

	// ignore events with an error
	if event.Error != nil {
		return false
	}

	// is this event sampled for activity dumps ?
	if !event.IsActivityDumpSample() {
		return false
	}

	m.m.Lock()
	defer m.m.Unlock()

	for _, d := range m.activeDumps {
		if d.GetState() == dump.Running && d.MatchesSelector(event.ProcessCacheEntry) {
			return true
		}
	}

	return false
}

// getExpiredDumps returns the list of dumps that have timed out and remove them from the active dumps
func (m *Manager) getExpiredDumps() []*dump.ActivityDump {
	m.m.Lock()
	defer m.m.Unlock()

	var expiredDumps []*dump.ActivityDump
	var newDumps []*dump.ActivityDump
	for _, ad := range m.activeDumps {
		if time.Now().After(ad.Profile.Metadata.End) || ad.GetState() == dump.Stopped {
			expiredDumps = append(expiredDumps, ad)
			delete(m.ignoreFromSnapshot, ad.Profile.Metadata.CGroupContext.CGroupFile)
		} else {
			newDumps = append(newDumps, ad)
		}
	}

	m.activeDumps = newDumps
	return expiredDumps
}

// cleanup
func (m *Manager) cleanup() {
	// fetch expired dumps
	dumps := m.getExpiredDumps()

	for _, ad := range dumps {
		m.finalizeKernelEventCollection(ad, true)
		seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.Profile.IsEmpty() && ad.Profile.GetWorkloadSelector() != nil {
			if err := m.persist(ad.Profile, m.configuredStorageRequests); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
			} else if m.config.RuntimeSecurity.SecurityProfileEnabled && ad.Profile.Metadata.CGroupContext.CGroupFlags.IsContainer() {
				// TODO: remove the IsContainer check once we start handling profiles for non-containerized workloads
				select {
				case m.newProfiles <- ad.Profile:
				default:
					// drop the profile and log error if the channel is full
					seclog.Warnf("couldn't send new profile to the manager: channel is full")
				}
			}
		} else {
			m.emptyDropped.Inc()
		}
	}

	// cleanup cgroup_wait_list map
	iterator := m.cgroupWaitList.Iterate()
	cgroupFile := make([]byte, model.PathKeySize)
	var timestamp uint64

	for iterator.Next(&cgroupFile, &timestamp) {
		if time.Now().After(m.resolvers.TimeResolver.ResolveMonotonicTimestamp(timestamp)) {
			if err := m.cgroupWaitList.Delete(&cgroupFile); err != nil {
				seclog.Errorf("couldn't delete cgroup_wait_list entry for (%v): %v", cgroupFile, err)
			}
		}
	}
}

// insertActivityDump inserts an activity dump in the list of activity dumps handled by the manager
func (m *Manager) insertActivityDump(newDump *dump.ActivityDump) error {
	// sanity checks
	if len(newDump.Profile.Metadata.ContainerID) > 0 {
		// check if the provided container ID is new
		for _, ad := range m.activeDumps {
			if ad.Profile.Metadata.ContainerID == newDump.Profile.Metadata.ContainerID {
				// an activity dump is already active for this container ID, ignore
				return fmt.Errorf("dump for container %s already running", ad.Profile.Metadata.ContainerID)
			}
		}
	}

	if len(newDump.Profile.Metadata.CGroupContext.CGroupID) > 0 {
		// check if the provided cgroup ID is new
		for _, ad := range m.activeDumps {
			if ad.Profile.Metadata.CGroupContext.CGroupID == newDump.Profile.Metadata.CGroupContext.CGroupID {
				// an activity dump is already active for this cgroup ID, ignore
				return fmt.Errorf("dump for cgroup %s already running", ad.Profile.Metadata.CGroupContext.CGroupID)
			}
		}
	}

	// loop through the process cache entry tree and push traced pids if necessary
	pces := m.newProcessCacheEntrySearcher(newDump)
	m.resolvers.ProcessResolver.Walk(func(entry *model.ProcessCacheEntry) {
		if !pces.ad.MatchesSelector(entry) {
			return
		}
		pces.ad.Profile.Metadata.CGroupContext = entry.CGroup
		pces.searchTracedProcessCacheEntry(entry)
	})

	// enable the new dump to start collecting events from kernel space
	if err := m.enableKernelEventCollection(newDump); err != nil {
		return fmt.Errorf("couldn't insert new dump: %w", err)
	}

	// Delay the activity dump snapshot to reduce the overhead on the main goroutine
	select {
	case m.snapshotQueue <- newDump:
	default:
	}

	// set the AD state now so that we can start inserting new events
	newDump.SetState(dump.Running)

	// append activity dump to the list of active dumps
	m.activeDumps = append(m.activeDumps, newDump)

	seclog.Infof("tracing started for [%s]", newDump.GetSelectorStr())
	return nil
}

// resolveTags thread unsafe version ot ResolveTags
func (m *Manager) resolveTags(ad *dump.ActivityDump) error {
	selector := ad.Profile.GetWorkloadSelector()
	if selector != nil {
		return nil
	}

	var workloadID interface{}
	if len(ad.Profile.Metadata.ContainerID) > 0 {
		workloadID = containerutils.ContainerID(ad.Profile.Metadata.ContainerID)
	} else if len(ad.Profile.Metadata.CGroupContext.CGroupID) > 0 {
		workloadID = ad.Profile.Metadata.CGroupContext.CGroupID
	}

	if workloadID != nil {
		tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
		if err != nil {
			return fmt.Errorf("failed to resolve %v: %w", workloadID, err)
		}
		ad.Profile.AddTags(tags)
	}

	ad.Profile.AddTags([]string{
		"cgroup_manager:" + ad.Profile.Metadata.CGroupContext.CGroupFlags.GetCGroupManager().String(),
	})

	return nil
}

// resolveTags resolves activity dump container tags when they are missing
func (m *Manager) resolveTagsAll() {
	m.m.Lock()
	defer m.m.Unlock()

	for _, ad := range m.activeDumps {
		m.resolveTagsPerAd(ad)
	}
}

// resolveTagsPerAd resolves the tags for a single activity dump
func (m *Manager) resolveTagsPerAd(ad *dump.ActivityDump) {
	err := m.resolveTags(ad)
	if err != nil {
		seclog.Warnf("couldn't resolve activity dump tags (will try again later): %v", err)
	}

	// check if we should discard this dump based on the manager dump limiter or the deny list
	selector := ad.Profile.GetWorkloadSelector()
	if selector == nil {
		// wait for the tags
		return
	}

	shouldFinalize := false
	for _, entry := range m.workloadDenyList {
		if entry.Match(*selector) {
			shouldFinalize = true
			m.workloadDenyListHits.Inc()
			break
		}
	}

	if !shouldFinalize && !ad.IsCountedByLimiter() {
		counter, ok := m.dumpLimiter.Get(*selector)
		if !ok {
			counter = atomic.NewUint64(0)
			m.dumpLimiter.Add(*selector, counter)
		}

		if counter.Load() >= uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
			shouldFinalize = true
			m.dropMaxDumpReached.Inc()
		} else {
			ad.SetCountedByLimiter(true)
			counter.Add(1)
		}
	}

	if shouldFinalize {
		m.finalizeKernelEventCollection(ad, true)
	}
}

func (m *Manager) enableKernelEventCollection(ad *dump.ActivityDump) error {
	// insert load config now (it might already exist when starting a new partial dump, update it in that case)
	if err := m.activityDumpsConfigMap.Update(ad.Cookie, ad.LoadConfig.Load(), ebpf.UpdateAny); err != nil {
		if !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("couldn't push activity dump load config: %w", err)
		}
	}

	if !ad.Profile.Metadata.CGroupContext.CGroupFile.IsNull() {
		// insert container ID in traced_cgroups map (it might already exist, do not update in that case)
		if err := m.tracedCgroupsMap.Update(ad.Profile.Metadata.CGroupContext.CGroupFile, ad.Cookie, ebpf.UpdateNoExist); err != nil {
			if !errors.Is(err, ebpf.ErrKeyExist) {
				// delete activity dump load config
				_ = m.activityDumpsConfigMap.Delete(ad.Cookie)
				return fmt.Errorf("couldn't push activity dump cgroup ID %s: %w", ad.GetSelectorStr(), err)
			}
		}
	}

	return nil
}

// pause (thread unsafe) assuming the current dump is running, "pause" sets the kernel space filters of the dump so that
// events are ignored in kernel space, and not sent to user space.
func (m *Manager) pauseKernelEventCollection(ad *dump.ActivityDump) error {
	if ad.GetState() <= dump.Paused {
		// nothing to do
		return nil
	}
	ad.SetState(dump.Paused)

	newLoadConfig := *ad.LoadConfig.Load()
	newLoadConfig.Paused = 1
	ad.LoadConfig.Store(&newLoadConfig)
	if err := m.activityDumpsConfigMap.Put(ad.Cookie, &newLoadConfig); err != nil {
		return fmt.Errorf("failed to pause activity dump [%s]: %w", ad.Profile.Metadata.ContainerID, err)
	}

	return nil
}

// disable (thread unsafe) assuming the current dump is running, "disable" removes kernel space filters so that events are no longer sent
// from kernel space
func (m *Manager) disableKernelEventCollection(ad *dump.ActivityDump) error {
	if ad.GetState() <= dump.Disabled {
		// nothing to do
		return nil
	}
	ad.SetState(dump.Disabled)

	// remove activity dump
	if err := m.activityDumpsConfigMap.Delete(ad.Cookie); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("couldn't delete activity dump load config for dump [%s]: %w", ad.GetSelectorStr(), err)
	}

	if !ad.Profile.Metadata.CGroupContext.CGroupFile.IsNull() {
		err := m.tracedCgroupsMap.Delete(ad.Profile.Metadata.CGroupContext.CGroupFile)
		if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("couldn't delete activity dump filter cgroup %s: %v", ad.GetSelectorStr(), err)
		}
	}

	return nil
}

// finalize (thread unsafe) finalizes an active dump: envs and args are scrubbed, tags, service and container ID are set. If a cgroup
// spot can be released, the dump will be fully stopped.
func (m *Manager) finalizeKernelEventCollection(ad *dump.ActivityDump, releaseTracedCgroupSpot bool) {
	if ad.GetState() == dump.Stopped {
		return
	}

	now := time.Now()
	ad.Profile.Metadata.End = now
	m.lastStoppedDumpTime = now

	if releaseTracedCgroupSpot {
		if err := m.disableKernelEventCollection(ad); err != nil {
			seclog.Errorf("couldn't disable activity dump: %v", err)
		}

		ad.SetState(dump.Stopped)
	}

	// add additional tags
	ad.Profile.AddTags(m.contextTags)

	// look for the service tag and set the service of the dump
	ad.Profile.Header.Service = ad.Profile.GetTagValue("service")

	// add the container ID in a tag
	if len(ad.Profile.Metadata.ContainerID) > 0 {
		// make sure we are not adding the same tag twice
		newTag := fmt.Sprintf("container_id:%s", ad.Profile.Metadata.ContainerID)
		if !ad.Profile.HasTag(newTag) {
			ad.Profile.AddTags([]string{newTag})
		} else {
			seclog.Errorf("container_id tag already present in tags (is finalize called multiple times?): %s", newTag)
		}
	}

	// add VersionContext
	if selector := ad.Profile.GetWorkloadSelector(); selector != nil && selector.IsReady() {
		nowNano := uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(now))
		tags := ad.Profile.GetTags()
		vCtx := &profile.VersionContext{
			FirstSeenNano:  nowNano,
			LastSeenNano:   nowNano,
			EventTypeState: make(map[model.EventType]*profile.EventTypeState),
			Syscalls:       ad.Profile.ComputeSyscallsList(),
			Tags:           make([]string, len(tags)),
		}
		copy(vCtx.Tags, tags)

		ad.Profile.AddVersionContext(selector.Tag, vCtx)
	}

	// scrub processes and retain args envs now
	ad.Profile.ScrubProcessArgsEnvs(m.resolvers.ProcessResolver)
}

// stopDumpsWithSelector stops the active dumps for the given selector and prevent a workload with the provided selector from ever being dumped again
func (m *Manager) stopDumpsWithSelector(selector cgroupModel.WorkloadSelector) {
	counter, ok := m.dumpLimiter.Get(selector)
	if !ok {
		counter = atomic.NewUint64(uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload))
		m.dumpLimiter.Add(selector, counter)
	} else {
		if counter.Load() < uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
			seclog.Infof("activity dumps will no longer be generated for %s", selector.String())
			counter.Store(uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload))
		}
	}

	m.m.Lock()
	defer m.m.Unlock()

	for _, ad := range m.activeDumps {
		if adSelector := ad.Profile.GetWorkloadSelector(); adSelector != nil && adSelector.Match(selector) {
			m.finalizeKernelEventCollection(ad, true)
			m.dropMaxDumpReached.Inc()
		}
	}
}

// handleSilentWorkloads checks if we should start tracing one of the workloads from a profile without an activity tree of the Security Profile manager
func (m *Manager) handleSilentWorkloads() {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	// check if it's a good time to look for a silent workload, to do so, check if the last stopped dump was stopped more
	// than the configured amount of time ago
	if time.Since(m.lastStoppedDumpTime) < m.config.RuntimeSecurity.ActivityDumpSilentWorkloadsDelay {
		return
	}

	// if we're already at capacity leave now - this prevents an unnecessary lock on the security profile manager
	if len(m.activeDumps) >= m.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
		return
	}

	// fetch silent workloads
workloadLoop:
	for selector, workloads := range m.fetchSilentWorkloads() {
		if len(workloads) == 0 {
			// this profile is on its way out, ignore
			continue
		}

		if len(m.activeDumps) >= m.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
			// we're at capacity, ignore for now
			break
		}

		// check if we already have an activity dump for this selector
		for _, ad := range m.activeDumps {
			// the dump selector is resolved if it has been counted by the limiter
			if !ad.IsCountedByLimiter() {
				continue
			}

			adSelector := ad.Profile.GetWorkloadSelector()
			if adSelector != nil && adSelector.Match(selector) {
				// we already have an activity dump for this selector, ignore
				continue workloadLoop
			}
		}

		// if we're still here, we can start tracing this workload
		defaultConfigs, err := m.getDefaultLoadConfigs()
		if err != nil {
			seclog.Errorf("couldn't get default load configs: %v", err)
			continue
		}

		defaultConfig, found := defaultConfigs[workloads[0].CGroupContext.CGroupFlags.GetCGroupManager()]
		if !found {
			seclog.Errorf("Failed to find default activity dump config for cgroup %s managed by %s", string(workloads[0].CGroupContext.CGroupID), workloads[0].CGroupContext.CGroupFlags.GetCGroupManager().String())
			continue
		}

		if err := m.startDumpWithConfig(workloads[0].ContainerID, workloads[0].CGroupContext, utils.NewCookie(), *defaultConfig); err != nil {
			if !errors.Is(err, unix.E2BIG) {
				seclog.Debugf("%v", err)
				break
			}
			seclog.Errorf("%v", err)
		}
	}
}

func (m *Manager) startDumpWithConfig(containerID containerutils.ContainerID, cgroupContext model.CGroupContext, cookie uint64, loadConfig model.ActivityDumpLoadConfig) error {
	// create a new activity dump
	newDump := dump.NewActivityDump(m.pathsReducer, m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs, 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, &loadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Metadata.ContainerID = containerID
		ad.Profile.Metadata = mtdt.Metadata{
			AgentVersion:      version.AgentVersion,
			AgentCommit:       version.Commit,
			KernelVersion:     m.kernelVersion.Code.String(),
			LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
			Arch:              utils.RuntimeArch(),

			Name:              fmt.Sprintf("activity-dump-%s", utils.RandString(10)),
			ProtobufVersion:   profile.ProtobufVersion,
			DifferentiateArgs: m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
			ContainerID:       containerID,
			CGroupContext:     cgroupContext,
			Start:             m.resolvers.TimeResolver.ResolveMonotonicTimestamp(loadConfig.StartTimestampRaw),
			End:               m.resolvers.TimeResolver.ResolveMonotonicTimestamp(loadConfig.EndTimestampRaw),
		}
		ad.Profile.Header.Host = m.hostname
		ad.Profile.Header.Source = ActivityDumpSource
	})
	newDump.Cookie = cookie

	if err := m.insertActivityDump(newDump); err != nil {
		return fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
	}
	return nil
}

// HandleCGroupTracingEvent handles a cgroup tracing event
func (m *Manager) HandleCGroupTracingEvent(event *model.CgroupTracingEvent) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	if len(event.CGroupContext.CGroupID) == 0 {
		seclog.Warnf("received a cgroup tracing event with an empty cgroup ID")
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	if err := m.startDumpWithConfig(event.ContainerContext.ContainerID, event.CGroupContext, event.ConfigCookie, event.Config); err != nil {
		seclog.Warnf("%v", err)
	}
}

// event lost recovery

// SnapshotTracedCgroups recovers lost CGroup tracing events by going through the kernel space map of cgroups
func (m *Manager) SnapshotTracedCgroups() {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	var err error
	var event model.CgroupTracingEvent
	var cgroupFile model.PathKey
	iterator := m.tracedCgroupsMap.Iterate()
	seclog.Infof("snapshotting traced_cgroups map")

	for iterator.Next(&cgroupFile, &event.ConfigCookie) {
		m.m.Lock()
		if m.ignoreFromSnapshot[cgroupFile] {
			m.m.Unlock()
			continue
		}
		m.m.Unlock()

		if err = m.activityDumpsConfigMap.Lookup(&event.ConfigCookie, &event.Config); err != nil {
			// this config doesn't exist anymore, remove expired entries
			seclog.Warnf("config not found for (%v): %v", cgroupFile, err)
			_ = m.tracedCgroupsMap.Delete(cgroupFile)
			continue
		}

		cgroupContext, _, err := m.resolvers.ResolveCGroupContext(cgroupFile, event.Config.CGroupFlags)
		if err != nil {
			seclog.Warnf("couldn't resolve cgroup context for (%v): %v", cgroupFile, err)
			continue
		}
		event.CGroupContext = *cgroupContext

		m.HandleCGroupTracingEvent(&event)
	}

	if err = iterator.Err(); err != nil {
		seclog.Warnf("couldn't iterate over the map traced_cgroups: %v", err)
	}
}
