// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"fmt"
	"slices"
	"time"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// fetchSilentWorkloads returns the list of workloads for which we haven't received any profile
func (m *Manager) fetchSilentWorkloads() map[cgroupModel.WorkloadSelector][]*tags.Workload {

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	out := make(map[cgroupModel.WorkloadSelector][]*tags.Workload)

	for selector, profile := range m.profiles {
		if !profile.LoadedInKernel.Load() {
			profile.InstancesLock.Lock()
			instances := make([]*tags.Workload, len(profile.Instances))
			copy(instances, profile.Instances)
			profile.InstancesLock.Unlock()
			out[selector] = instances
		}
	}

	return out
}

// LookupEventInProfiles lookups event in profiles
func (m *Manager) LookupEventInProfiles(event *model.Event) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	// ignore events with an error
	if event.Error != nil {
		return
	}

	// create profile selector
	event.FieldHandlers.ResolveContainerTags(event, event.ContainerContext)
	if len(event.ContainerContext.Tags) == 0 {
		return
	}
	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", event.ContainerContext.Tags), "*")
	if err != nil {
		return
	}

	// lookup profile
	m.profilesLock.Lock()
	profile := m.profiles[selector]
	m.profilesLock.Unlock()
	if profile == nil {
		m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)
		return
	}
	if !profile.IsEventTypeValid(event.GetEventType()) || !profile.LoadedInKernel.Load() {
		m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)
		return
	}

	_ = event.FieldHandlers.ResolveContainerCreatedAt(event, event.ContainerContext)

	// check if the event should be injected in the profile automatically
	imageTag := utils.GetTagValue("image_tag", event.ContainerContext.Tags)
	if imageTag == "" {
		imageTag = "latest" // not sure about this one
	}

	ctx, found := profile.GetVersionContext(imageTag)
	if found {
		ctx.LastSeenNano = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now()))
	} else {
		evictedVersions := profile.PrepareNewVersion(imageTag, event.ContainerContext.Tags, m.config.RuntimeSecurity.SecurityProfileMaxImageTags, uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now())))
		for _, evictedVersion := range evictedVersions {
			m.countEvictedVersion(imageTag, evictedVersion)
		}
		ctx, found = profile.GetVersionContext(imageTag)
		if !found {
			return
		}
	}

	// if we have one version of the profile in unstable for this event type, just skip the whole process
	globalEventTypeProfilState := profile.GetGlobalEventTypeState(event.GetEventType())
	if globalEventTypeProfilState == model.UnstableEventType {
		m.incrementEventFilteringStat(event.GetEventType(), model.UnstableEventType, NA)
		// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
		// the anomaly flag if the user space profile considers it to not be an anomaly. Here, when a version is unstable,
		// we don't want to generate anomalies for this profile anymore.
		event.ResetAnomalyDetectionEvent()
		return
	}

	profileState := m.tryAutolearn(profile, ctx, event, imageTag)
	if profileState != model.NoProfile {
		ctx.EventTypeState[event.GetEventType()].State = profileState
	}
	switch profileState {
	case model.NoProfile, model.ProfileAtMaxSize, model.UnstableEventType:
		// an error occurred or we are in unstable state
		// do not link the profile to avoid sending anomalies

		// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
		// the anomaly flag if the user space profile considers it to not be an anomaly.
		// We can also get a syscall anomaly detection kernel space for runc, which is ignored in the activity tree
		// (i.e. tryAutolearn returns NoProfile) because "runc" can't be a root node.
		event.ResetAnomalyDetectionEvent()

		return
	case model.AutoLearning, model.WorkloadWarmup:
		// the event was either already in the profile, or has just been inserted
		fillProfileContextFromProfile(&event.SecurityProfileContext, profile, imageTag, profileState)
		event.AddToFlags(model.EventFlagsSecurityProfileInProfile)

		return
	case model.StableEventType:
		// check if the event is in its profile
		// and if this is not an exec event, check if we can benefit of the occasion to add missing processes
		insertMissingProcesses := false
		if event.GetEventType() != model.ExecEventType {
			if execState := m.getEventTypeState(profile, ctx, event, model.ExecEventType, imageTag); execState == model.AutoLearning || execState == model.WorkloadWarmup {
				insertMissingProcesses = true
			}
		}
		found, err := profile.Contains(event, insertMissingProcesses, imageTag, activity_tree.ProfileDrift, m.resolvers)
		if err != nil {
			// ignore, evaluation failed
			m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)

			// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
			// the anomaly flag if the user space profile considers it to not be an anomaly.
			event.ResetAnomalyDetectionEvent()
			return
		}
		fillProfileContextFromProfile(&event.SecurityProfileContext, profile, imageTag, profileState)
		if found {
			event.AddToFlags(model.EventFlagsSecurityProfileInProfile)
			m.incrementEventFilteringStat(event.GetEventType(), profileState, InProfile)

			// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
			// the anomaly flag if the user space profile considers it to not be an anomaly.
			event.ResetAnomalyDetectionEvent()
		} else {
			m.incrementEventFilteringStat(event.GetEventType(), profileState, NotInProfile)
			if m.canGenerateAnomaliesFor(event) {
				event.AddToFlags(model.EventFlagsAnomalyDetectionEvent)
			}
		}
	}
}

// tryAutolearn tries to autolearn the input event. It returns the profile state: stable, unstable, autolearning or workloadwarmup
func (m *Manager) tryAutolearn(profile *profile.Profile, ctx *profile.VersionContext, event *model.Event, imageTag string) model.EventFilteringProfileState {
	profileState := m.getEventTypeState(profile, ctx, event, event.GetEventType(), imageTag)
	var nodeType activity_tree.NodeGenerationType
	if profileState == model.AutoLearning {
		nodeType = activity_tree.ProfileDrift
	} else if profileState == model.WorkloadWarmup {
		nodeType = activity_tree.WorkloadWarmup
	} else { // Stable or Unstable state
		return profileState
	}

	// here we are either in AutoLearning or WorkloadWarmup
	// try to insert the event in the profile

	// defines if we want or not to insert missing processes
	insertMissingProcesses := false
	if event.GetEventType() == model.ExecEventType {
		insertMissingProcesses = true
	} else if execState := m.getEventTypeState(profile, ctx, event, model.ExecEventType, imageTag); execState == model.AutoLearning || execState == model.WorkloadWarmup {
		insertMissingProcesses = true
	}

	newEntry, err := profile.Insert(event, insertMissingProcesses, imageTag, nodeType, m.resolvers)
	if err != nil {
		m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)
		return model.NoProfile
	} else if newEntry {
		eventState, ok := ctx.EventTypeState[event.GetEventType()]
		if ok { // should always be the case
			eventState.LastAnomalyNano = event.TimestampRaw
		}

		// if a previous version of this profile was stable for this event type,
		// and a new entry was added, trigger an anomaly detection
		globalEventTypeState := profile.GetGlobalEventTypeState(event.GetEventType())
		if globalEventTypeState == model.StableEventType && m.canGenerateAnomaliesFor(event) {
			event.AddToFlags(model.EventFlagsAnomalyDetectionEvent)
		} else {
			// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
			// the anomaly flag if the user space profile considers it to not be an anomaly: there is a new entry and no
			// previous version is in stable state.
			event.ResetAnomalyDetectionEvent()
		}

		m.incrementEventFilteringStat(event.GetEventType(), profileState, NotInProfile)
	} else { // no newEntry
		m.incrementEventFilteringStat(event.GetEventType(), profileState, InProfile)
		// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
		// the anomaly flag if the user space profile considers it to not be an anomaly
		event.ResetAnomalyDetectionEvent()
	}
	return profileState
}

// fillProfileContextFromProfile fills the given ctx with profile infos
func fillProfileContextFromProfile(ctx *model.SecurityProfileContext, p *profile.Profile, imageTag string, state model.EventFilteringProfileState) {
	ctx.Name = p.Metadata.Name
	if ctx.Name == "" {
		ctx.Name = DefaultProfileName
	}

	ctx.EventTypes = p.GetEventTypes()
	ctx.EventTypeState = state
	profileContext, ok := p.GetVersionContext(imageTag)
	if ok { // should always be the case
		ctx.Tags = profileContext.Tags
	}
}

// FillProfileContextFromContainerID populates a SecurityProfileContext for the given container ID
func (m *Manager) FillProfileContextFromContainerID(id string, ctx *model.SecurityProfileContext, imageTag string) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, profile := range m.profiles {
		profile.InstancesLock.Lock()
		for _, instance := range profile.Instances {
			instance.Lock()
			if instance.ContainerID == containerutils.ContainerID(id) {
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

// loadProfile (thread unsafe) loads a Security Profile in kernel space
func (m *Manager) loadProfileMap(profile *profile.Profile) error {
	profile.LoadedInKernel.Store(true)
	profile.LoadedNano.Store(uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now())))

	// push kernel space filters
	if err := m.securityProfileSyscallsMap.Put(profile.GetProfileCookie(), profile.GenerateSyscallsFilters()); err != nil {
		return fmt.Errorf("couldn't push syscalls filter (check map size limit ?): %w", err)
	}

	// TODO: load generated programs
	seclog.Debugf("security profile %s loaded in kernel space", profile.Metadata.Name)
	return nil
}

// unloadProfile (thread unsafe) unloads a Security Profile from kernel space
func (m *Manager) unloadProfileMap(profile *profile.Profile) {
	profile.LoadedInKernel.Store(false)

	// remove kernel space filters
	if err := m.securityProfileSyscallsMap.Delete(profile.GetProfileCookie()); err != nil {
		seclog.Errorf("couldn't remove syscalls filter: %v", err)
	}

	// TODO: delete all kernel space programs
	seclog.Debugf("security profile %s unloaded from kernel space", profile.Metadata.Name)
}

// linkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *Manager) linkProfileMap(profile *profile.Profile, workload *tags.Workload) {
	if err := m.securityProfileMap.Put([]byte(workload.ContainerID), profile.GetProfileCookie()); err != nil {
		seclog.Errorf("couldn't link workload %s (selector: %s) with profile %s (check map size limit ?): %v", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name, err)
		return
	}
	seclog.Infof("workload %s (selector: %s) successfully linked to profile %s", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name)
}

// linkProfile applies a profile to the provided workload
func (m *Manager) linkProfile(profile *profile.Profile, workload *tags.Workload) {
	profile.InstancesLock.Lock()
	defer profile.InstancesLock.Unlock()

	// check if this instance of this workload is already tracked
	for _, w := range profile.Instances {
		if w.ContainerID == workload.ContainerID {
			// nothing to do, leave
			return
		}
	}

	// update the list of tracked instances
	profile.Instances = append(profile.Instances, workload)

	// can we apply the profile or is it not ready yet ?
	if profile.LoadedInKernel.Load() {
		m.linkProfileMap(profile, workload)
	}
}

// unlinkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *Manager) unlinkProfileMap(profile *profile.Profile, workload *tags.Workload) {
	if !profile.LoadedInKernel.Load() {
		return
	}

	if err := m.securityProfileMap.Delete([]byte(workload.ContainerID)); err != nil {
		seclog.Errorf("couldn't unlink workload %s (selector: %s) with profile %s: %v", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name, err)
	}
	seclog.Infof("workload %s (selector: %s) successfully unlinked from profile %s", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name)
}

// unlinkProfile removes the link between a workload and a profile
func (m *Manager) unlinkProfile(profile *profile.Profile, workload *tags.Workload) {
	profile.InstancesLock.Lock()
	defer profile.InstancesLock.Unlock()

	// remove the workload from the list of instances of the Security Profile
	for key, val := range profile.Instances {
		if workload.ContainerID == val.ContainerID {
			profile.Instances = append(profile.Instances[0:key], profile.Instances[key+1:]...)
			break
		}
	}

	// remove link between the profile and the workload
	m.unlinkProfileMap(profile, workload)
}

func (m *Manager) onWorkloadSelectorResolvedEvent(workload *tags.Workload) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	workload.Lock()
	defer workload.Unlock()

	if workload.Deleted.Load() {
		// this workload was deleted before we had time to apply its profile, ignore
		return
	}

	// TODO: remove the IsContainer check once we start handling profiles for non-containerized workloads
	if !workload.CGroupFlags.IsContainer() {
		return
	}

	defaultConfigs, err := m.getDefaultLoadConfigs()
	if err != nil {
		seclog.Errorf("couldn't get default load configs: %v", err)
		return
	}

	// check whether we are configured to apply a profile for this type of workload/cgroup
	// as this function is called by the tags resolver, which also resolves tags for systemd cgroups
	_, found := defaultConfigs[workload.CGroupFlags.GetCGroupManager()]
	if !found {
		seclog.Debugf("no default load config found for manager %s, not applying profile for workload %s", workload.CGroupFlags.GetCGroupManager().String(), workload.Selector.String())
		return
	}

	selector := workload.Selector
	selector.Tag = "*"

	// check if the workload of this selector already exists
	p, ok := m.profiles[selector]
	if !ok {
		// check the cache
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		p, ok = m.pendingCache.Get(selector)
		if ok {
			m.cacheHit.Inc()

			// remove profile from cache
			_ = m.pendingCache.Remove(selector)

			// since the profile was in cache, it was removed from kernel space, load it now
			// (locking isn't necessary here, but added as a safeguard)
			// TODO: check locking scheme
			err := m.loadProfileMap(p)
			if err != nil {
				seclog.Errorf("couldn't load security profile %s in kernel space: %v", p.GetSelectorStr(), err)
				return
			}

			// insert the profile in the list of active profiles
			m.profiles[selector] = p
		} else {
			m.cacheMiss.Inc()

			p = profile.New(
				profile.WithWorkloadSelector(selector),
				profile.WithPathsReducer(m.pathsReducer),
				profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
				profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
				profile.WithEventTypes(m.secProfEventTypes),
			)

			// insert the profile in the list of active profiles
			m.profiles[selector] = p

			// try to load the profile from local storage
			ok, err := m.localStorage.Load(&selector, p)
			if err != nil {
				seclog.Warnf("couldn't load profile from local storage: %v", err)
				return
			} else if ok {
				err = m.loadProfileMap(p)
				if err != nil {
					seclog.Errorf("couldn't load security profile %s in kernel space: %v", p.GetSelectorStr(), err)
					return
				}
			}
		}
	}

	m.linkProfile(p, workload)
}

func (m *Manager) onWorkloadDeletedEvent(workload *tags.Workload) {
	// lookup the profile
	selector := cgroupModel.WorkloadSelector{
		Image: workload.Selector.Image,
		Tag:   "*",
	}
	m.profilesLock.Lock()
	p := m.profiles[selector]
	m.profilesLock.Unlock()
	if p == nil {
		// nothing to do, leave
		return
	}

	// removes the link between the profile and this workload
	m.unlinkProfile(p, workload)

	// check if the profile should be deleted
	m.shouldDeleteProfile(p)
}

func (m *Manager) shouldDeleteProfile(p *profile.Profile) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()

	p.InstancesLock.Lock()
	defer p.InstancesLock.Unlock()
	// check if the profile should be deleted
	if len(p.Instances) != 0 {
		// this profile is still in use, leave now
		return
	}

	// remove the profile from the list of profiles
	delete(m.profiles, *p.GetWorkloadSelector())

	if p.LoadedInKernel.Load() {
		// remove profile from kernel space
		m.unloadProfileMap(p)
		if err := m.persistProfile(p); err != nil {
			seclog.Errorf("couldn't persist profile: %v", err)
		}
	}

	// cleanup profile before insertion in cache
	p.Reset()

	// TODO: is it possible to have a profile with no selector here?
	if p.GetWorkloadSelector().IsReady() {
		// do not insert in cache
		return
	}

	// add profile in cache
	m.pendingCache.Add(*p.GetWorkloadSelector(), p)
}

// onNewProfile handles the arrival of a new profile after it was created as an activity dump
func (m *Manager) onNewProfile(newProfile *profile.Profile) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// here the new profile is coming from an activity dump, so its activity tree corresponds to the selector image_name + image_tag.
	// we need to update the selector to a security profile selector (image_name + "*" to match all image tags).
	selector := *newProfile.GetWorkloadSelector()
	profileManagerSelector := selector
	if selector.Tag != "*" {
		profileManagerSelector.Tag = "*"
	}

	// Update the Security Profile content
	p, ok := m.profiles[profileManagerSelector]
	if !ok {
		// this was likely a short-lived workload, cache the profile in case this workload comes back
		// insert in cache and leave
		p = profile.New(
			profile.WithWorkloadSelector(selector),
			profile.WithPathsReducer(m.pathsReducer),
			profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
			profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
			profile.WithEventTypes(m.secProfEventTypes),
		)

		p.LoadFromNewProfile(newProfile)

		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		m.pendingCache.Add(profileManagerSelector, p)
		return
	}

	// if profile was waited, push it
	if !p.LoadedInKernel.Load() {
		// merge the content of the new profile
		p.LoadFromNewProfile(newProfile)

		// load the profile in kernel space
		if err := m.loadProfileMap(p); err != nil {
			seclog.Errorf("couldn't load security profile %s in kernel space: %v", p.GetWorkloadSelector(), err)
			return
		}
		// link all workloads
		for _, workload := range p.Instances {
			m.linkProfileMap(p, workload)
		}
	}

	// if we already have a loaded profile for this workload, just ignore the new one
}

func (m *Manager) canGenerateAnomaliesFor(e *model.Event) bool {
	return m.config.RuntimeSecurity.AnomalyDetectionEnabled && slices.Contains(m.config.RuntimeSecurity.AnomalyDetectionEventTypes, e.GetEventType())
}

func (m *Manager) getEventTypeState(p *profile.Profile, pctx *profile.VersionContext, event *model.Event, eventType model.EventType, imageTag string) model.EventFilteringProfileState {
	eventState, ok := pctx.EventTypeState[event.GetEventType()]
	if !ok {
		eventState = &profile.EventTypeState{
			LastAnomalyNano: pctx.FirstSeenNano,
			State:           model.AutoLearning,
		}
		pctx.EventTypeState[eventType] = eventState
	} else if eventState.State == model.UnstableEventType {
		// If for the given event type we already are on UnstableEventType, just return
		// (once reached, this state is immutable)
		if eventType == event.GetEventType() { // increment stat only once for each event
			m.incrementEventFilteringStat(eventType, model.UnstableEventType, NA)
		}
		return model.UnstableEventType
	}

	var nodeType activity_tree.NodeGenerationType
	var profileState model.EventFilteringProfileState
	// check if we are at the beginning of a workload lifetime
	if event.ResolveEventTime().Sub(time.Unix(0, int64(event.ContainerContext.CreatedAt))) < m.config.RuntimeSecurity.AnomalyDetectionWorkloadWarmupPeriod {
		nodeType = activity_tree.WorkloadWarmup
		profileState = model.WorkloadWarmup
	} else {
		// If for the given event type we already are on StableEventType (and outside of the warmup period), just return
		if eventState.State == model.StableEventType {
			return model.StableEventType
		}

		if eventType == event.GetEventType() { // update the stable/unstable states only for the event event type
			// did we reached the stable state time limit ?
			if time.Duration(event.TimestampRaw-eventState.LastAnomalyNano) >= m.config.RuntimeSecurity.GetAnomalyDetectionMinimumStablePeriod(eventType) {
				eventState.State = model.StableEventType
				// call the activity dump manager to stop dumping workloads from the current profile selector
				if m.config.RuntimeSecurity.ActivityDumpEnabled {
					uniqueImageTagSeclector := *p.GetWorkloadSelector()
					uniqueImageTagSeclector.Tag = imageTag
					m.stopDumpsWithSelector(uniqueImageTagSeclector)
				}
				return model.StableEventType
			}

			// did we reached the unstable time limit ?
			if time.Duration(event.TimestampRaw-p.LoadedNano.Load()) >= m.config.RuntimeSecurity.AnomalyDetectionUnstableProfileTimeThreshold {
				eventState.State = model.UnstableEventType
				return model.UnstableEventType
			}
		}

		nodeType = activity_tree.ProfileDrift
		profileState = model.AutoLearning
	}

	// check if the unstable size limit was reached, but only for the event event type
	if eventType == event.GetEventType() && p.ComputeInMemorySize() >= m.config.RuntimeSecurity.AnomalyDetectionUnstableProfileSizeThreshold {
		// for each event type we want to reach either the StableEventType or UnstableEventType states, even
		// if we already reach the AnomalyDetectionUnstableProfileSizeThreshold. That's why we have to keep
		// rearming the lastAnomalyNano timer based on if it's something new or not.
		found, err := p.Contains(event, false /*insertMissingProcesses*/, imageTag, nodeType, m.resolvers)
		if err != nil {
			m.incrementEventFilteringStat(eventType, model.NoProfile, NA)
			return model.NoProfile
		} else if !found {
			eventState.LastAnomalyNano = event.TimestampRaw
		} else if profileState == model.WorkloadWarmup {
			// if it's NOT something's new AND we are on container warmup period, just pretend
			// we are in learning/warmup phase (as we know, this event is already present on the profile)
			return model.WorkloadWarmup
		}
		return model.ProfileAtMaxSize
	}
	return profileState
}

func (m *Manager) incrementEventFilteringStat(eventType model.EventType, state model.EventFilteringProfileState, result EventFilteringResult) {
	m.eventFiltering[eventFilteringEntry{eventType, state, result}].Inc()
}

// CountEvictedVersion count the evicted version for associated metric
func (m *Manager) countEvictedVersion(imageName, imageTag string) {
	m.evictedVersionsLock.Lock()
	defer m.evictedVersionsLock.Unlock()
	m.evictedVersions = append(m.evictedVersions, cgroupModel.WorkloadSelector{
		Image: imageName,
		Tag:   imageTag,
	})
}
