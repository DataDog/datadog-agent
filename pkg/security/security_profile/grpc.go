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
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
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

// ListActivityDumps returns the list of active activity dumps
func (m *Manager) ListActivityDumps(_ *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpListMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	m.m.Lock()
	defer m.m.Unlock()

	var activeDumpMsgs []*api.ActivityDumpMessage
	for _, d := range m.activeDumps {
		activeDumpMsgs = append(activeDumpMsgs, d.Profile.ToSecurityActivityDumpMessage(d.GetTimeout(), m.configuredStorageRequests))
	}
	return &api.ActivityDumpListMessage{
		Dumps: activeDumpMsgs,
	}, nil
}

// DumpActivity handles an activity dump request
func (m *Manager) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	if params.GetContainerID() == "" && params.GetCGroupID() == "" {
		err := errors.New("you must specify one selector between containerID and cgroupID")
		return &api.ActivityDumpMessage{Error: err.Error()}, err
	}

	var timeout time.Duration
	if params.GetTimeout() == "" {
		timeout = m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout
	} else {
		var err error
		timeout, err = time.ParseDuration(params.GetTimeout())
		if err != nil {
			err := fmt.Errorf("failed to handle activity dump request: invalid timeout duration: %w", err)
			return &api.ActivityDumpMessage{Error: err.Error()}, err
		}
	}

	cgroupFlags := containerutils.CGroupFlags(0)
	if params.GetCGroupID() != "" {
		_, flags := containerutils.FindContainerID(containerutils.CGroupID(params.GetCGroupID()))
		cgroupFlags = containerutils.CGroupFlags(flags)
	}

	m.m.Lock()
	defer m.m.Unlock()

	now := time.Now()
	loadConfig := m.newActivityDumpLoadConfig(
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		timeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		m.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
		cgroupFlags,
	)

	newDump := dump.NewActivityDump(m.pathsReducer, params.GetDifferentiateArgs(), 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, loadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Metadata = mtdt.Metadata{
			AgentVersion:      version.AgentVersion,
			AgentCommit:       version.Commit,
			KernelVersion:     m.kernelVersion.Code.String(),
			LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
			Arch:              utils.RuntimeArch(),

			Name:              fmt.Sprintf("activity-dump-%s", utils.RandString(10)),
			ProtobufVersion:   profile.ProtobufVersion,
			DifferentiateArgs: params.GetDifferentiateArgs(),
			ContainerID:       containerutils.ContainerID(params.GetContainerID()),
			CGroupContext: model.CGroupContext{
				CGroupID: containerutils.CGroupID(params.GetCGroupID()),
			},
			Start: now,
			End:   now.Add(timeout),
		}
		ad.Profile.Header.Host = m.hostname
		ad.Profile.Header.Source = ActivityDumpSource
	})

	if err := m.insertActivityDump(newDump); err != nil {
		err := fmt.Errorf("couldn't start tracing [%s]: %w", params.GetContainerID(), err)
		return &api.ActivityDumpMessage{Error: err.Error()}, err
	}

	return newDump.Profile.ToSecurityActivityDumpMessage(timeout, m.configuredStorageRequests), nil
}

// StopActivityDump stops an active activity dump
func (m *Manager) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	m.m.Lock()
	defer m.m.Unlock()

	if params.GetName() == "" && params.GetContainerID() == "" && params.GetCGroupID() == "" {
		err := errors.New("you must specify one selector between name, containerID and cgroupID")
		return &api.ActivityDumpStopMessage{Error: err.Error()}, err
	}

	toDelete := -1
	for i, ad := range m.activeDumps {
		if (params.GetName() != "" && ad.Profile.Metadata.Name == params.GetName()) ||
			(params.GetContainerID() != "" && ad.Profile.Metadata.ContainerID == containerutils.ContainerID(params.GetContainerID())) ||
			(params.GetCGroupID() != "" && ad.Profile.Metadata.CGroupContext.CGroupID == containerutils.CGroupID(params.GetCGroupID())) {
			m.finalizeKernelEventCollection(ad, true)
			seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())
			toDelete = i

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
			break
		}
	}

	if toDelete >= 0 {
		m.activeDumps = append(m.activeDumps[:toDelete], m.activeDumps[toDelete+1:]...)
		return &api.ActivityDumpStopMessage{}, nil
	}

	var err error
	if params.GetName() != "" {
		err = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following name: %s", params.GetName())
	} else if params.GetContainerID() != "" {
		err = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following containerID: %s", params.GetContainerID())
	} else /* if params.GetCGroupID() != "" */ {
		err = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following cgroup ID: %s", params.GetCGroupID())
	}

	return &api.ActivityDumpStopMessage{Error: err.Error()}, err
}

// GenerateTranscoding executes the requested transcoding operation
func (m *Manager) GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.TranscodingRequestMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	m.m.Lock()
	defer m.m.Unlock()

	ad := dump.NewActivityDump(
		m.pathsReducer,
		m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
		0,
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		m.updateTracedPid,
		m.defaultActivityDumpLoadConfig(time.Now(), containerutils.CGroupFlags(0)),
	)

	// open and parse input file
	if err := ad.Profile.Decode(params.GetActivityDumpFile()); err != nil {
		err := fmt.Errorf("couldn't parse input file %s: %w", params.GetActivityDumpFile(), err)
		return &api.TranscodingRequestMessage{Error: err.Error()}, err
	}

	// add transcoding requests
	storageRequests, err := config.ParseStorageRequests(params.GetStorage())
	if err != nil {
		err := fmt.Errorf("couldn't parse transcoding request for [%s]: %w", ad.GetSelectorStr(), err)
		return &api.TranscodingRequestMessage{Error: err.Error()}, err
	}

	if err := m.persist(ad.Profile, perFormatStorageRequests(storageRequests)); err != nil {
		err := fmt.Errorf("couldn't persist dump [%s]: %w", ad.GetSelectorStr(), err)
		return &api.TranscodingRequestMessage{Error: err.Error()}, err
	}

	message := &api.TranscodingRequestMessage{}
	for _, request := range storageRequests {
		message.Storage = append(message.Storage, request.ToStorageRequestMessage(ad.Profile.Metadata.Name))
	}

	return message, nil
}

// ListSecurityProfiles returns the list of security profiles
func (m *Manager) ListSecurityProfiles(params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return &api.SecurityProfileListMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	var out api.SecurityProfileListMessage

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, p := range m.profiles {
		msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver)
		out.Profiles = append(out.Profiles, msg)
	}

	if params.GetIncludeCache() {
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		for _, k := range m.pendingCache.Keys() {
			p, ok := m.pendingCache.Peek(k)
			if !ok {
				continue
			}
			msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver)
			out.Profiles = append(out.Profiles, msg)
		}
	}
	return &out, nil
}

// SaveSecurityProfile saves the requested security profile to disk
func (m *Manager) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return &api.SecurityProfileSaveMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	selector, err := cgroupModel.NewWorkloadSelector(params.GetSelector().GetName(), "*")
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: err.Error(),
		}, nil
	}

	m.profilesLock.Lock()
	p := m.profiles[selector]
	m.profilesLock.Unlock()

	if p == nil {
		return &api.SecurityProfileSaveMessage{
			Error: "security profile not found",
		}, nil
	}

	// encode profile
	raw, err := p.EncodeSecurityProfileProtobuf()
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: fmt.Sprintf("couldn't encode security profile in %s format: %v", config.Protobuf, err),
		}, nil
	}

	// write profile to encoded profile to disk
	f, err := os.CreateTemp("/tmp", fmt.Sprintf("%s-*.profile", p.Metadata.Name))
	if err != nil {
		return nil, fmt.Errorf("couldn't create temporary file: %w", err)
	}
	defer f.Close()

	if _, err = f.Write(raw.Bytes()); err != nil {
		return nil, fmt.Errorf("couldn't write to temporary file: %w", err)
	}

	return &api.SecurityProfileSaveMessage{
		File: f.Name(),
	}, nil
}
