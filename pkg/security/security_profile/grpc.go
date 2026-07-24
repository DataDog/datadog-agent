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
	"sync"
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
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
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

	// host-wide mode: rather than tracing a single cgroup, restart the capture
	// window of every dump the manager is already tracing. Gated behind its own
	// config switch and never forwards to the remote backend (see StopActivityDump).
	if params.GetHost() {
		if !m.config.RuntimeSecurity.ActivityDumpHostDumpEnabled {
			return &api.ActivityDumpMessage{Error: ErrHostDumpDisabled.Error()}, ErrHostDumpDisabled
		}

		m.m.Lock()
		defer m.m.Unlock()

		// Open the window with a full-host baseline: reset the tree of any
		// already-active dumps (and clear the ignore-list so previously-stopped
		// cgroups are traceable again), then start a fresh dump for every other
		// currently-running cgroup so the window captures everything running now
		// -- not only cgroups that happen to exec after this point.
		reset := m.resetAllDumps()
		snapped := m.snapshotAllCgroups()
		seclog.Infof("host-wide capture window (re)started: %d active dumps reset, %d cgroups snapshotted", reset, snapped)
		return &api.ActivityDumpMessage{
			Metadata: &api.MetadataMessage{Name: fmt.Sprintf("host-wide capture window started (%d reset + %d snapshotted = %d cgroups)", reset, snapped, reset+snapped)},
		}, nil
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

	m.m.Lock()
	defer m.m.Unlock()

	now := time.Now()
	loadConfig := m.newActivityDumpLoadConfig(
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		timeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		m.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
	)

	newDump := dump.NewActivityDump(m.pathsReducer, params.GetDifferentiateArgs(), 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, loadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Metadata = mtdt.Metadata{
			AgentVersion:      version.AgentVersion,
			AgentCommit:       version.Commit,
			KernelVersion:     m.kernelVersion.Code.String(),
			LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
			Arch:              utils.RuntimeArch(),

			Name:              "activity-dump-" + utils.RandString(10),
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

// finalizeDumpLocked stops kernel-side collection for a dump and marks its cgroup
// to be ignored from snapshots so it is not immediately re-created. It does NOT
// persist the dump. The caller must hold m.m and remove the dump from
// m.activeDumps.
func (m *Manager) finalizeDumpLocked(ad *dump.ActivityDump) {
	m.finalizeKernelEventCollection(ad, true)
	// mark the cgroup to ignore from snapshot to prevent re-creation
	m.ignoreFromSnapshot[ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode] = true
	seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())
}

// persistStoppedDump persists an already-finalized dump using the provided storage
// requests. When sendToProfiles is true (and security profiles are enabled) the
// profile is also queued to the profile manager. This performs encoding and I/O
// (potentially a remote upload) and must NOT be called while holding m.m. It
// returns a message describing the dump.
func (m *Manager) persistStoppedDump(ad *dump.ActivityDump, storageRequests map[config.StorageFormat][]config.StorageRequest, sendToProfiles bool) *api.ActivityDumpMessage {
	// persist dump if not empty
	if !ad.Profile.IsEmpty() && ad.Profile.GetWorkloadSelector() != nil {
		if err := m.persist(ad.Profile, storageRequests); err != nil {
			seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
		} else if sendToProfiles && m.config.RuntimeSecurity.SecurityProfileEnabled {
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

	return ad.Profile.ToSecurityActivityDumpMessage(ad.GetTimeout(), storageRequests)
}

// StopActivityDump stops an active activity dump
func (m *Manager) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	// host-wide mode: stop every active dump and persist each to LOCAL storage only
	// (never forwarded to the remote backend, and not fed into the security profile
	// pipeline). Gated behind its own config switch. Finalization happens under the
	// lock; the encode+write of each dump is done after releasing it so a slow disk
	// cannot stall the CWS event pipeline.
	if params.GetAll() {
		if !m.config.RuntimeSecurity.ActivityDumpHostDumpEnabled {
			return &api.ActivityDumpStopMessage{Error: ErrHostDumpDisabled.Error()}, ErrHostDumpDisabled
		}

		m.m.Lock()
		stopped := m.activeDumps
		m.activeDumps = nil
		for _, ad := range stopped {
			m.finalizeDumpLocked(ad)
		}
		m.m.Unlock()

		dumps := make([]*api.ActivityDumpMessage, 0, len(stopped))
		for _, ad := range stopped {
			dumps = append(dumps, m.persistStoppedDump(ad, m.localStorageRequests, false))
		}
		return &api.ActivityDumpStopMessage{Dumps: dumps}, nil
	}

	if params.GetName() == "" && params.GetContainerID() == "" && params.GetCGroupID() == "" {
		err := errors.New("you must specify one selector between name, containerID and cgroupID")
		return &api.ActivityDumpStopMessage{Error: err.Error()}, err
	}

	// find and finalize the matching dump under the lock, then persist it after
	// releasing the lock so encoding/upload does not stall the event pipeline.
	m.m.Lock()
	var stopped *dump.ActivityDump
	for i, ad := range m.activeDumps {
		if (params.GetName() != "" && ad.Profile.Metadata.Name == params.GetName()) ||
			(params.GetContainerID() != "" && ad.Profile.Metadata.ContainerID == containerutils.ContainerID(params.GetContainerID())) ||
			(params.GetCGroupID() != "" && ad.Profile.Metadata.CGroupContext.CGroupID == containerutils.CGroupID(params.GetCGroupID())) {
			m.finalizeDumpLocked(ad)
			m.activeDumps = append(m.activeDumps[:i], m.activeDumps[i+1:]...)
			stopped = ad
			break
		}
	}
	m.m.Unlock()

	if stopped != nil {
		m.persistStoppedDump(stopped, m.configuredStorageRequests, true)
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
		m.defaultActivityDumpLoadConfig(time.Now()),
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
	out, err := listSecurityProfilesCommon(m.config, &m.profilesLock, m.profiles, m.resolvers.TimeResolver, params)
	if err != nil {
		return out, err
	}

	// V1-specific: include pending cache if requested
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
	return out, nil
}

// SaveSecurityProfile saves the requested security profile to disk
func (m *Manager) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	return saveSecurityProfileCommon(m.config, &m.profilesLock, m.profiles, params)
}

// ListSecurityProfiles lists the security profiles for the ManagerV2
func (m *ManagerV2) ListSecurityProfiles(params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	return listSecurityProfilesCommon(m.config, &m.profilesLock, m.profiles, m.resolvers.TimeResolver, params)
}

// SaveSecurityProfile saves the requested security profile to disk for the ManagerV2
func (m *ManagerV2) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	return saveSecurityProfileCommon(m.config, &m.profilesLock, m.profiles, params)
}

// Common functions for both Manager and ManagerV2
// listSecurityProfilesCommon is the shared implementation for listing security profiles
func listSecurityProfilesCommon(
	cfg *config.Config,
	profilesLock *sync.Mutex,
	profiles map[cgroupModel.WorkloadSelector]*profile.Profile,
	resolver *ktime.Resolver,
	_ *api.SecurityProfileListParams,
) (*api.SecurityProfileListMessage, error) {
	if !cfg.RuntimeSecurity.SecurityProfileEnabled {
		return &api.SecurityProfileListMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	var out api.SecurityProfileListMessage

	profilesLock.Lock()
	defer profilesLock.Unlock()

	for _, p := range profiles {
		msg := p.ToSecurityProfileMessage(resolver)
		out.Profiles = append(out.Profiles, msg)
	}

	return &out, nil
}

// saveSecurityProfileCommon is the shared implementation for saving a security profile
func saveSecurityProfileCommon(
	cfg *config.Config,
	profilesLock *sync.Mutex,
	profiles map[cgroupModel.WorkloadSelector]*profile.Profile,
	params *api.SecurityProfileSaveParams,
) (*api.SecurityProfileSaveMessage, error) {
	if !cfg.RuntimeSecurity.SecurityProfileEnabled {
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

	profilesLock.Lock()
	p := profiles[selector]
	profilesLock.Unlock()

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
	f, err := os.CreateTemp("/tmp", p.Metadata.Name+"-*.profile")
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
