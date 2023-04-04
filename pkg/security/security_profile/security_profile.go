// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package security_profile

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// ProfileHandler of the activity dumps and security profiles
type ProfileHandler struct {
	Config       *config.RuntimeSecurityConfig
	StatsdClient statsd.ClientInterface

	activityDumpManager    *dump.ActivityDumpManager
	securityProfileManager *profile.SecurityProfileManager
}

// NewProfileHandler returns a new instance of a ProfileHandler
func NewProfileHandler(cfg *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface) *ProfileHandler {
	return &ProfileHandler{
		Config:       cfg,
		StatsdClient: statsdClient,
	}
}

// Init initializes the event handler
func (e *ProfileHandler) Init(manager *manager.Manager, resolvers *resolvers.Resolvers, kernelVersion *kernel.Version, scrubber *procutil.DataScrubber, eventCtor func() *model.Event) error {
	var err error

	if e.Config.IsActivityDumpEnabled() {
		e.activityDumpManager, err = dump.NewActivityDumpManager(e.Config, e.StatsdClient, eventCtor, resolvers.ProcessResolver, resolvers.TimeResolver, resolvers.TagsResolver, kernelVersion, scrubber, manager)
		if err != nil {
			return fmt.Errorf("couldn't create the activity dump manager: %w", err)
		}
	}

	if e.Config.SecurityProfileEnabled {
		e.securityProfileManager, err = profile.NewSecurityProfileManager(e.Config, e.StatsdClient, resolvers.CGroupResolver, manager)
		if err != nil {
			return fmt.Errorf("couldn't create the security profile manager: %w", err)
		}
	}

	return nil
}

// GetActivityDumpManager returns the activity dump manager
func (e *ProfileHandler) GetActivityDumpManager() *dump.ActivityDumpManager {
	return e.activityDumpManager
}

// Start triggers the goroutine of all the underlying handlers
func (e *ProfileHandler) Start(ctx context.Context, wg *sync.WaitGroup) error {
	if e.activityDumpManager != nil {
		wg.Add(1)
		go e.activityDumpManager.Start(ctx, wg)
	}

	if e.securityProfileManager != nil {
		go e.securityProfileManager.Start(ctx)
	}

	return nil
}

// SendStats sends statistics
func (e *ProfileHandler) SendStats() error {
	if e.activityDumpManager != nil {
		if err := e.activityDumpManager.SendStats(); err != nil {
			return fmt.Errorf("failed to send activity dump manager stats: %w", err)
		}
	}

	if e.securityProfileManager != nil {
		if err := e.securityProfileManager.SendStats(); err != nil {
			return fmt.Errorf("failed to send security profile manager stats: %w", err)
		}
	}

	return nil
}

// ProcessEvent processes an event
func (e *ProfileHandler) ProcessEvent(event *model.Event) {
	if e.activityDumpManager != nil && event.PathResolutionError == nil {
		e.activityDumpManager.ProcessEvent(event)
	}
}

// ErrActivityDumpManagerDisabled is returned when the activity dump manager is disabled
var ErrActivityDumpManagerDisabled = errors.New("ActivityDumpManager is disabled")

// DumpActivity handles an activity dump request
func (e *ProfileHandler) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if !e.Config.IsActivityDumpEnabled() {
		return &api.ActivityDumpMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return e.activityDumpManager.DumpActivity(params)
}

// ListActivityDumps returns the list of active dumps
func (e *ProfileHandler) ListActivityDumps(params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if !e.Config.IsActivityDumpEnabled() {
		return &api.ActivityDumpListMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return e.activityDumpManager.ListActivityDumps(params)
}

// StopActivityDump stops an active activity dump
func (e *ProfileHandler) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if !e.Config.IsActivityDumpEnabled() {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return e.activityDumpManager.StopActivityDump(params)
}

// GenerateTranscoding encodes an activity dump following the input parameters
func (e *ProfileHandler) GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if !e.Config.IsActivityDumpEnabled() {
		return &api.TranscodingRequestMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return e.activityDumpManager.TranscodingRequest(params)
}

func (e *ProfileHandler) GetActivityDumpTracedEventTypes() []model.EventType {
	return e.Config.ActivityDumpTracedEventTypes
}

// IsNeededForActivityDump returns if the event will be handled by AD
func (e *ProfileHandler) IsNeededForActivityDump(eventType eval.EventType) bool {
	if e.Config.IsActivityDumpEnabled() {
		for _, e := range e.GetActivityDumpTracedEventTypes() {
			if e.String() == eventType {
				return true
			}
		}
	}
	return false
}

// IsSyscallEventTypeEnabled returns if the syscall capture is enabled
func (e *ProfileHandler) IsSyscallEventTypeEnabled() bool {
	if e.Config.IsActivityDumpEnabled() {
		for _, e := range e.GetActivityDumpTracedEventTypes() {
			if e == model.SyscallsEventType {
				return true
			}
		}
	}
	return false
}

// FillProfileContextFromContainerID returns the profile of a container ID
func (e *ProfileHandler) FillProfileContextFromContainerID(id string, ctx *model.SecurityProfileContext) *profile.SecurityProfile {
	return e.securityProfileManager.FillProfileContextFromContainerID(id, ctx)
}
