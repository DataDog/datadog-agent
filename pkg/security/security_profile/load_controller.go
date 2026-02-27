// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func (m *Manager) newActivityDumpLoadConfig(evt []model.EventType, timeout time.Duration, waitListTimeout time.Duration, rate uint16, start time.Time) *model.ActivityDumpLoadConfig {
	lc := &model.ActivityDumpLoadConfig{
		TracedEventTypes: evt,
		Timeout:          timeout,
		Rate:             uint16(rate),
	}
	if m.resolvers != nil {
		lc.StartTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start))
		lc.EndTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start.Add(timeout)))
		lc.WaitListTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start.Add(waitListTimeout)))
	}
	return lc
}

func (m *Manager) defaultActivityDumpLoadConfig(now time.Time) *model.ActivityDumpLoadConfig {
	return m.newActivityDumpLoadConfig(
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		m.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
	)
}

func (m *Manager) getDefaultLoadConfig() *model.ActivityDumpLoadConfig {
	if m.activityDumpLoadConfig != nil {
		return m.activityDumpLoadConfig
	}
	m.activityDumpLoadConfig = m.defaultActivityDumpLoadConfig(time.Now())
	return m.activityDumpLoadConfig
}

func (m *Manager) nextPartialDump(prev *dump.ActivityDump) *dump.ActivityDump {
	previousLoadConfig := prev.LoadConfig.Load()

	now := time.Now()
	newLoadConfig := m.newActivityDumpLoadConfig(
		previousLoadConfig.TracedEventTypes,
		previousLoadConfig.Timeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		previousLoadConfig.Rate,
		now,
	)
	newDump := dump.NewActivityDump(m.pathsReducer, prev.Profile.Metadata.DifferentiateArgs, 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, newLoadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Header = prev.Profile.Header
		ad.Profile.Metadata = prev.Profile.Metadata
		ad.Profile.Metadata.Name = "activity-dump-" + utils.RandString(10)
		ad.Profile.Metadata.Start = now
		ad.Profile.Metadata.End = now.Add(previousLoadConfig.Timeout)
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
		if err := m.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumpSizeInMemory, float64(dumpSize), []string{"dump_index:" + strconv.Itoa(i)}, 1); err != nil {
			seclog.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpActiveDumpSizeInMemory, err)
		}

		if dumpSize >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
			toDelete = append([]int{i}, toDelete...)
			dumps = append(dumps, ad)
			m.ignoreFromSnapshot[ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode] = true
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

		if err := m.insertActivityDump(newDump); err != nil {
			seclog.Errorf("couldn't resume tracing [%s]: %v", newDump.GetSelectorStr(), err)
		}

		// remove container ID from the map of ignored container IDs for the snapshot
		delete(m.ignoreFromSnapshot, ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode)
	}
}
