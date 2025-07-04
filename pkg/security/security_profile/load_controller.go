// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func (m *Manager) newActivityDumpLoadConfig(evt []model.EventType, timeout time.Duration, waitListTimeout time.Duration, rate uint16, start time.Time, flags containerutils.CGroupFlags) *model.ActivityDumpLoadConfig {
	lc := &model.ActivityDumpLoadConfig{
		TracedEventTypes: evt,
		Timeout:          timeout,
		Rate:             uint16(rate),
		CGroupFlags:      flags,
	}
	if m.resolvers != nil {
		lc.StartTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start))
		lc.EndTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start.Add(timeout)))
		lc.WaitListTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start.Add(waitListTimeout)))
	}
	return lc
}

func (m *Manager) defaultActivityDumpLoadConfig(now time.Time, flags containerutils.CGroupFlags) *model.ActivityDumpLoadConfig {
	return m.newActivityDumpLoadConfig(
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		m.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
		flags,
	)
}

func (m *Manager) getDefaultLoadConfigs() (map[containerutils.CGroupManager]*model.ActivityDumpLoadConfig, error) {
	if m.activityDumpLoadConfig != nil {
		return m.activityDumpLoadConfig, nil
	}

	defaults := m.defaultActivityDumpLoadConfig(time.Now(), containerutils.CGroupFlags(0)) // cgroup flags will be set per cgroup manager

	allDefaultConfigs := map[string]containerutils.CGroupManager{
		containerutils.CGroupManagerDocker.String():  containerutils.CGroupManagerDocker,
		containerutils.CGroupManagerPodman.String():  containerutils.CGroupManagerPodman,
		containerutils.CGroupManagerCRI.String():     containerutils.CGroupManagerCRI,
		containerutils.CGroupManagerCRIO.String():    containerutils.CGroupManagerCRIO,
		containerutils.CGroupManagerSystemd.String(): containerutils.CGroupManagerSystemd,
	}
	defaultConfigs := make(map[containerutils.CGroupManager]*model.ActivityDumpLoadConfig)
	for _, cgroupManager := range m.config.RuntimeSecurity.ActivityDumpCgroupsManagers {
		cgroupManager, found := allDefaultConfigs[cgroupManager]
		if !found {
			return nil, fmt.Errorf("unsupported cgroup manager '%s'", cgroupManager)
		}
		cgroupManagerLoadConfig := *defaults
		cgroupManagerLoadConfig.CGroupFlags = containerutils.CGroupFlags(cgroupManager)
		defaultConfigs[cgroupManager] = &cgroupManagerLoadConfig
	}

	m.activityDumpLoadConfig = defaultConfigs
	return defaultConfigs, nil
}

func (m *Manager) sendLoadControllerTriggeredMetric(tags []string) error {
	if err := m.statsdClient.Count(metrics.MetricActivityDumpLoadControllerTriggered, 1, tags, 1.0); err != nil {
		return fmt.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpLoadControllerTriggered, err)
	}
	return nil
}

func (m *Manager) nextPartialDump(prev *dump.ActivityDump) *dump.ActivityDump {
	previousLoadConfig := prev.LoadConfig.Load()
	timeToThreshold := time.Since(prev.Profile.Metadata.Start)

	newRate := previousLoadConfig.Rate
	if timeToThreshold < m.minDumpTimeout {
		newRate = previousLoadConfig.Rate * 3 / 4 // reduce by 25%
		if err := m.sendLoadControllerTriggeredMetric([]string{"reduction:rate"}); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	newTimeout := previousLoadConfig.Timeout
	if timeToThreshold < m.minDumpTimeout/2 && previousLoadConfig.Timeout > m.minDumpTimeout {
		newTimeout = previousLoadConfig.Timeout * 3 / 4 // reduce by 25%
		if newTimeout < m.minDumpTimeout {
			newTimeout = m.minDumpTimeout
		}
		if err := m.sendLoadControllerTriggeredMetric([]string{"reduction:dump_timeout"}); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	newEvents := make([]model.EventType, len(previousLoadConfig.TracedEventTypes))
	copy(newEvents, previousLoadConfig.TracedEventTypes)
	if timeToThreshold < m.minDumpTimeout/4 {
		var evtToRemove model.EventType
		newEvents = newEvents[:0]
	reductionOrder:
		for _, evt := range TracedEventTypesReductionOrder {
			for _, tracedEvt := range previousLoadConfig.TracedEventTypes {
				if evt == tracedEvt {
					evtToRemove = evt
					break reductionOrder
				}
			}
		}
		for _, evt := range previousLoadConfig.TracedEventTypes {
			if evt != evtToRemove {
				newEvents = append(newEvents, evt)
			}
		}

		if evtToRemove != model.UnknownEventType {
			if err := m.sendLoadControllerTriggeredMetric([]string{"reduction:traced_event_types", "event_type:" + evtToRemove.String()}); err != nil {
				seclog.Errorf("%v", err)
			}
		}
	}

	now := time.Now()
	newLoadConfig := m.newActivityDumpLoadConfig(newEvents, newTimeout, m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout, newRate, now, previousLoadConfig.CGroupFlags)
	newDump := dump.NewActivityDump(m.pathsReducer, prev.Profile.Metadata.DifferentiateArgs, 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, newLoadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Header = prev.Profile.Header
		ad.Profile.Metadata = prev.Profile.Metadata
		ad.Profile.Metadata.Name = fmt.Sprintf("activity-dump-%s", utils.RandString(10))
		ad.Profile.Metadata.Start = now
		ad.Profile.Metadata.End = now.Add(newTimeout)
		ad.Profile.AddTags(prev.Profile.GetTags())
	})

	newDump.Cookie = prev.Cookie

	return newDump
}

// getOverweightDumps returns the list of dumps that crossed the config.ActivityDumpMaxDumpSize threshold
func (m *Manager) getOverweightDumps() []*dump.ActivityDump {
	var dumps []*dump.ActivityDump
	var toDelete []int
	for i, ad := range m.activeDumps {
		dumpSize := ad.Profile.ComputeInMemorySize()

		// send dump size in memory metric
		if err := m.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumpSizeInMemory, float64(dumpSize), []string{fmt.Sprintf("dump_index:%d", i)}, 1); err != nil {
			seclog.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpActiveDumpSizeInMemory, err)
		}

		if dumpSize >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
			toDelete = append([]int{i}, toDelete...)
			dumps = append(dumps, ad)
			m.ignoreFromSnapshot[ad.Profile.Metadata.CGroupContext.CGroupFile] = true
		}
	}
	for _, i := range toDelete {
		m.activeDumps = append(m.activeDumps[:i], m.activeDumps[i+1:]...)
	}
	return dumps
}

func (m *Manager) triggerLoadController() {
	m.m.Lock()
	defer m.m.Unlock()

	// handle overweight dumps
	for _, ad := range m.getOverweightDumps() {
		// restart a new dump for the same workload
		newDump := m.nextPartialDump(ad)

		// stop the dump but do not release the cgroup
		m.finalizeKernelEventCollection(ad, false)
		seclog.Infof("tracing paused for [%s]", ad.GetSelectorStr())

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

		if err := m.insertActivityDump(newDump); err != nil {
			seclog.Errorf("couldn't resume tracing [%s]: %v", newDump.GetSelectorStr(), err)
		}

		// remove container ID from the map of ignored container IDs for the snapshot
		delete(m.ignoreFromSnapshot, ad.Profile.Metadata.CGroupContext.CGroupFile)
	}
}
