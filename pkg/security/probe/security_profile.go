// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// SecurityProfileManagers holds the security profile managers
type SecurityProfileManagers struct {
	config                 *config.Config
	activityDumpManager    *dump.ActivityDumpManager
	securityProfileManager *profile.SecurityProfileManager
}

// NewSecurityProfileManagers returns a new manager object
func NewSecurityProfileManagers[T any](p *Probe[T]) (*SecurityProfileManagers, error) {
	managers := SecurityProfileManagers{
		config: p.Config,
	}

	if p.IsActivityDumpEnabled() {
		activityDumpManager, err := dump.NewActivityDumpManager(p.Config, p.StatsdClient, func() *model.Event { return NewEvent(p.fieldHandlers) }, p.resolvers, p.kernelVersion, p.Manager)
		if err != nil {
			return nil, fmt.Errorf("couldn't create the activity dump manager: %w", err)
		}
		managers.activityDumpManager = activityDumpManager
	}

	if p.IsSecurityProfileEnabled() {
		securityProfileManager, err := profile.NewSecurityProfileManager(p.Config, p.StatsdClient, p.resolvers, p.Manager)
		if err != nil {
			return nil, fmt.Errorf("couldn't create the security profile manager: %w", err)
		}
		managers.securityProfileManager = securityProfileManager
	}

	if p.IsActivityDumpEnabled() && p.IsSecurityProfileEnabled() {
		managers.activityDumpManager.SetSecurityProfileManager(managers.securityProfileManager)
		managers.securityProfileManager.SetActivityDumpManager(managers.activityDumpManager)
	}

	return &managers, nil
}

// Start triggers the goroutine of all the underlying controllers and monitors of the Monitor
func (spm *SecurityProfileManagers) Start(ctx context.Context, wg *sync.WaitGroup) {
	if spm.activityDumpManager != nil {
		wg.Add(1)
		go spm.activityDumpManager.Start(ctx, wg)
	}
	if spm.securityProfileManager != nil {
		go spm.securityProfileManager.Start(ctx)
	}
}

// SendStats sends statistics about the probe to Datadog
func (spm *SecurityProfileManagers) SendStats() error {
	if spm.activityDumpManager != nil {
		if err := spm.activityDumpManager.SendStats(); err != nil {
			return fmt.Errorf("failed to send activity dump manager stats: %w", err)
		}
	}

	if spm.securityProfileManager != nil {
		if err := spm.securityProfileManager.SendStats(); err != nil {
			return fmt.Errorf("failed to send security profile manager stats: %w", err)
		}
	}

	return nil
}

// ErrActivityDumpManagerDisabled is returned when the activity dump manager is disabled
var ErrActivityDumpManagerDisabled = errors.New("ActivityDumpManager is disabled")

// ErrSecurityProfileManagerDisabled is returned when the security profile manager is disabled
var ErrSecurityProfileManagerDisabled = errors.New("SecurityProfileManager is disabled")

// AddActivityDumpHandler add a handler
func (spm *SecurityProfileManagers) AddActivityDumpHandler(handler dump.ActivityDumpHandler) {
	if spm.activityDumpManager != nil {
		spm.activityDumpManager.AddActivityDumpHandler(handler)
	}
}

// DumpActivity handles an activity dump request
func (spm *SecurityProfileManagers) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if spm.activityDumpManager == nil {
		return &api.ActivityDumpMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return spm.activityDumpManager.DumpActivity(params)
}

// ListActivityDumps returns the list of active dumps
func (spm *SecurityProfileManagers) ListActivityDumps(params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if spm.activityDumpManager == nil {
		return &api.ActivityDumpListMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return spm.activityDumpManager.ListActivityDumps(params)
}

// StopActivityDump stops an active activity dump
func (spm *SecurityProfileManagers) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if spm.activityDumpManager == nil {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return spm.activityDumpManager.StopActivityDump(params)
}

// GenerateTranscoding encodes an activity dump following the input parameters
func (spm *SecurityProfileManagers) GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if spm.activityDumpManager == nil {
		return &api.TranscodingRequestMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return spm.activityDumpManager.TranscodingRequest(params)
}

// GetActivityDumpTracedEventTypes returns traced event types
func (spm *SecurityProfileManagers) GetActivityDumpTracedEventTypes() []model.EventType {
	return spm.config.RuntimeSecurity.ActivityDumpTracedEventTypes
}

// SnapshotTracedCgroups snapshots traced cgroups
func (spm *SecurityProfileManagers) SnapshotTracedCgroups() {
	spm.activityDumpManager.SnapshotTracedCgroups()
}

// ListSecurityProfiles list the profiles
func (spm *SecurityProfileManagers) ListSecurityProfiles(params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	if spm.securityProfileManager == nil {
		return nil, ErrSecurityProfileManagerDisabled
	}
	return spm.securityProfileManager.ListSecurityProfiles(params)
}

// SaveSecurityProfile save a security profile
func (spm *SecurityProfileManagers) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	if spm.securityProfileManager == nil {
		return nil, ErrSecurityProfileManagerDisabled
	}
	return spm.securityProfileManager.SaveSecurityProfile(params)
}

// GetActivityDumpManager returns the activity dump manager
func (spm *SecurityProfileManagers) GetActivityDumpManager() *dump.ActivityDumpManager {
	return spm.activityDumpManager
}

// GetSecurityProfileManager returns the security profile manager
func (spm *SecurityProfileManagers) GetSecurityProfileManager() *profile.SecurityProfileManager {
	return spm.securityProfileManager
}
