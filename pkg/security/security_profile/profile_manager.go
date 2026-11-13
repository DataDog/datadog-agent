// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProfileManager is the interface for profile management operations.
type ProfileManager interface {
	// Start starts the profile manager
	Start(ctx context.Context)

	// ProcessEvent processes an event for security profiles
	ProcessEvent(event *model.Event)

	// SendStats sends metrics about the profile manager
	SendStats() error

	// ListSecurityProfiles returns the list of security profiles
	ListSecurityProfiles(params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error)

	// SaveSecurityProfile saves the requested security profile to disk
	SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error)

	// GenerateTranscoding generates a transcoding request for the given activity dump
	GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error)

	// ListActivityDumps returns the list of active activity dumps
	ListActivityDumps(params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error)

	// StopActivityDump stops an active activity dump if it exists
	StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error)

	// HandleSampleRefresh updates the LastSeen timestamp for the activity tree node
	// associated with the given kernel dedup cookie
	HandleSampleRefresh(cookie uint64)
}
