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
		isExpired := time.Now().After(ad.Profile.Metadata.End)
		isStopped := ad.GetState() == dump.Stopped

		if isExpired || isStopped {
			expiredDumps = append(expiredDumps, ad)

			// Only remove from ignoreFromSnapshot if expired naturally
			// Keep manually stopped dumps in ignoreFromSnapshot to prevent re-creation
			if isExpired && !isStopped {
				delete(m.ignoreFromSnapshot, ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode)
			}
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
		m.FinalizeKernelEventCollection(ad, true)

		seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.Profile.IsEmpty() && ad.Profile.GetWorkloadSelector() != nil {
			if err := m.persist(ad.Profile, m.configuredStorageRequests); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
			} else if m.config.RuntimeSecurity.SecurityProfileEnabled {
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
	var cgroupInode uint64
	var timestamp uint64

	for iterator.Next(&cgroupInode, &timestamp) {
		if time.Now().After(m.resolvers.TimeResolver.ResolveMonotonicTimestamp(timestamp)) {
			if err := m.cgroupWaitList.Delete(cgroupInode); err != nil {
				seclog.Errorf("couldn't delete cgroup_wait_list entry for inode (%v): %v", cgroupInode, err)
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

	// check if we're at capacity
	if len(m.activeDumps) >= m.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
		return fmt.Errorf("activity dump capacity reached (%d/%d)", len(m.activeDumps), m.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount)
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

	// start by syncing active dumps and traced cgroup map
	m.syncTracedCgroups()

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

	var workloadID containerutils.WorkloadID
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

	if !ad.Profile.Metadata.CGroupContext.CGroupPathKey.IsNull() {
		// insert cgroup ID in traced_cgroups map (it might already exist, do not update in that case)
		if err := m.tracedCgroupsMap.Update(ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode, ad.Cookie, ebpf.UpdateNoExist); err != nil {
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

	if !ad.Profile.Metadata.CGroupContext.CGroupPathKey.IsNull() {
		err := m.tracedCgroupsMap.Delete(ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode)
		if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("couldn't delete activity dump filter cgroup %s: %v", ad.GetSelectorStr(), err)
		}
	}

	// cleanup traced PIDs for this dump
	m.cleanupTracedPids(ad)

	return nil
}

// cleanupTracedPids removes all PIDs associated with a dump from the traced_pids map
func (m *Manager) cleanupTracedPids(ad *dump.ActivityDump) {
	var pids []uint32

	// Try to get workload from cgroup resolver
	if ad.Profile.Metadata.ContainerID != "" {
		// Container workload
		if cacheEntry := m.resolvers.CGroupResolver.GetCacheEntryContainerID(ad.Profile.Metadata.ContainerID); cacheEntry != nil {
			pids = cacheEntry.GetPIDs()
		}
	} else if ad.Profile.Metadata.CGroupContext.CGroupID != "" {
		// Host workload
		if cachedEntry := m.resolvers.CGroupResolver.GetCacheEntryByCgroupID(ad.Profile.Metadata.CGroupContext.CGroupID); cachedEntry != nil {
			pids = cachedEntry.GetPIDs()
		}
	}

	// Remove all PIDs from traced_pids map
	for _, pid := range pids {
		if err := m.tracedPIDsMap.Delete(pid); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			seclog.Debugf("couldn't delete PID %d from traced_pids for dump [%s]: %v", pid, ad.GetSelectorStr(), err)
		}
	}
}

// FinalizeKernelEventCollection finalizes an active dump: envs and args are scrubbed, tags, service and container ID are set. If a cgroup
// spot can be released, the dump will be fully stopped.
func (m *Manager) FinalizeKernelEventCollection(ad *dump.ActivityDump, releaseTracedCgroupSpot bool) {
	m.m.Lock()
	defer m.m.Unlock()
	m.finalizeKernelEventCollection(ad, releaseTracedCgroupSpot)
}

// finalizeKernelEventCollection thread unsafe version of FinalizeKernelEventCollection
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
		newTag := "container_id:" + string(ad.Profile.Metadata.ContainerID)
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
		defaultConfig := m.getDefaultLoadConfig()

		// if not a container, check we should trace it
		if workloads[0].GCroupCacheEntry.IsContainerContextNull() && !m.config.RuntimeSecurity.ActivityDumpTraceSystemdCgroups {
			continue
		}

		if err := m.startDumpWithConfig(workloads[0].GCroupCacheEntry.GetContainerID(), workloads[0].GCroupCacheEntry.GetCGroupContext(), utils.NewCookie(), *defaultConfig); err != nil {
			seclog.Debugf("%v", err)
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

			Name:              "activity-dump-" + utils.RandString(10),
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

func (m *Manager) evictTracedCgroup(cgroup *model.CGroupContext) {
	// first, retrieve the cookie from the traced cgroup entry
	var cookie uint64
	if err := m.tracedCgroupsMap.Lookup(cgroup.CGroupPathKey.Inode, &cookie); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			seclog.Errorf("couldn't lookup activity dump cgroup %s cookie: %v", cgroup.CGroupID, err)
		}
		// if the entry doesn't exist, we still try to cleanup
	}

	// second, push the evicted cgroup to the discarded map
	if err := m.tracedCgroupsDiscardedMap.Put(cgroup.CGroupPathKey.Inode, uint8(1)); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			seclog.Errorf("couldn't discard activity dump cgroup %s: %v", cgroup.CGroupID, err)
		}
	}

	if cookie != 0 {
		// third, cleanup the activity_dumps_config map using the cookie
		if err := m.activityDumpsConfigMap.Delete(cookie); err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				seclog.Errorf("couldn't delete activity dump config for cgroup %s (cookie %d): %v", cgroup.CGroupID, cookie, err)
			}
		}

		// fourth, cleanup the activity_dump_rate_limiters map using the cookie
		if err := m.activityDumpRateLimitersMap.Delete(cookie); err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				seclog.Errorf("couldn't delete activity dump rate limiter for cgroup %s (cookie %d): %v", cgroup.CGroupID, cookie, err)
			}
		}
	}

	// finally, evict it from the traced one
	if err := m.tracedCgroupsMap.Delete(cgroup.CGroupPathKey.Inode); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			seclog.Errorf("couldn't delete activity dump filter cgroup %s: %v", cgroup.CGroupID, err)
		}
	}

}

// HandleCGroupTracingEvent handles a cgroup tracing event
func (m *Manager) HandleCGroupTracingEvent(event *model.CgroupTracingEvent) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	if len(event.CGroupContext.CGroupID) == 0 {
		seclog.Warnf("received a cgroup tracing event with an empty cgroup ID")
		m.evictTracedCgroup(&event.CGroupContext)
		return
	}

	if event.ContainerContext.ContainerID == "" && !m.config.RuntimeSecurity.ActivityDumpTraceSystemdCgroups {
		m.evictTracedCgroup(&event.CGroupContext)
		return
	}

	// Check if this cgroup is in the discarded map (kernel blacklist)
	var discarded uint8
	err := m.tracedCgroupsDiscardedMap.Lookup(event.CGroupContext.CGroupPathKey.Inode, &discarded)
	if err == nil {
		// Cgroup is in the discarded map, should not trace it
		m.evictTracedCgroup(&event.CGroupContext)
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	// Check if this cgroup should be ignored (e.g., manually stopped dump)
	if m.ignoreFromSnapshot[event.CGroupContext.CGroupPathKey.Inode] {
		return
	}

	if err := m.startDumpWithConfig(event.ContainerContext.ContainerID, event.CGroupContext, event.ConfigCookie, event.Config); err != nil {
		seclog.Debugf("%v", err)
	}
}

// event lost recovery

// SyncTracedCgroups recovers lost CGroup tracing events by going through the kernel space map of cgroups
func (m *Manager) SyncTracedCgroups() {
	m.m.Lock()
	defer m.m.Unlock()
	m.syncTracedCgroups()
}

// manager should be locked
func (m *Manager) syncTracedCgroups() {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	var err error
	var event model.CgroupTracingEvent
	var cgroupInode uint64
	iterator := m.tracedCgroupsMap.Iterate()
	seclog.Infof("snapshotting traced_cgroups map")

	// Collect cgroups to delete during iteration, execute deletion AFTER
	// to avoid modifying the map while iterating (causes "iteration aborted")
	var cgroupsToDelete []uint64
	for iterator.Next(&cgroupInode, &event.ConfigCookie) {
		if m.ignoreFromSnapshot[cgroupInode] {
			// mark for deletion - will delete AFTER iteration
			cgroupsToDelete = append(cgroupsToDelete, cgroupInode)
			continue
		}

		if err = m.activityDumpsConfigMap.Lookup(&event.ConfigCookie, &event.Config); err != nil {
			// this config doesn't exist anymore, mark for deletion
			seclog.Debugf("config not found for inode (%v): %v", cgroupInode, err)
			cgroupsToDelete = append(cgroupsToDelete, cgroupInode)
			continue
		}

		hasActiveDump := false
		for _, ad := range m.activeDumps {
			if ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode == cgroupInode {
				hasActiveDump = true
				break
			}
		}

		if !hasActiveDump {
			// No active dump for this cgroup - mark for deletion
			cgroupsToDelete = append(cgroupsToDelete, cgroupInode)
		}
	}

	if err = iterator.Err(); err != nil {
		seclog.Warnf("couldn't iterate over the map traced_cgroups: %v", err)
	}

	// Delete all marked cgroups AFTER iteration (safe to modify map now)
	for _, cgroupInode := range cgroupsToDelete {
		_ = m.tracedCgroupsMap.Delete(cgroupInode)
	}
}
