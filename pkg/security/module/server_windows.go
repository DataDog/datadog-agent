// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// sshSessionPatcher stub for Windows
type sshSessionPatcher interface {
	IsResolved() error
	PatchEvent(_ interface{})
	MaxRetry() int
}

// createSSHSessionPatcher creates a no-op patcher for Windows
func createSSHSessionPatcher(_ *model.Event, _ *probe.Probe) sshSessionPatcher {
	return nil
}

// DumpDiscarders handles discarder dump requests
func (a *APIServer) DumpDiscarders(_ context.Context, _ *api.DumpDiscardersParams) (*api.DumpDiscardersMessage, error) {
	return nil, errors.New("not supported")
}

// DumpProcessCache handles process cache dump requests
func (a *APIServer) DumpProcessCache(_ context.Context, _ *api.DumpProcessCacheParams) (*api.SecurityDumpProcessCacheMessage, error) {
	return nil, errors.New("not supported")
}

// ListActivityDumps returns the list of active dumps
func (a *APIServer) ListActivityDumps(_ context.Context, _ *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	return nil, errors.New("not supported")
}

// StopActivityDump stops an active activity dump if it exists
func (a *APIServer) StopActivityDump(_ context.Context, _ *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	return nil, errors.New("not supported")
}

// TranscodingRequest encodes an activity dump following the requested parameters
func (a *APIServer) TranscodingRequest(_ context.Context, _ *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	return nil, errors.New("not supported")
}

// ListSecurityProfiles returns the list of security profiles
func (a *APIServer) ListSecurityProfiles(_ context.Context, _ *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	return nil, errors.New("not supported")
}

// SaveSecurityProfile saves the requested security profile to disk
func (a *APIServer) SaveSecurityProfile(_ context.Context, _ *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	return nil, errors.New("not supported")
}

func (a *APIServer) fillStatusPlatform(_ *api.Status) error {
	return nil
}

// DumpNetworkNamespace handles network namespace cache dump requests
func (a *APIServer) DumpNetworkNamespace(_ context.Context, _ *api.DumpNetworkNamespaceParams) (*api.DumpNetworkNamespaceMessage, error) {
	return nil, errors.New("not supported")
}

// RunSelfTest runs self test and then reload the current policies
func (a *APIServer) RunSelfTest(_ context.Context, _ *api.RunSelfTestParams) (*api.SecuritySelfTestResultMessage, error) {
	if a.cwsConsumer == nil {
		return nil, errors.New("failed to found module in APIServer")
	}

	if a.selfTester == nil {
		return &api.SecuritySelfTestResultMessage{
			Ok:    false,
			Error: "self-tests are disabled",
		}, nil
	}

	if _, err := a.cwsConsumer.RunSelfTest(true); err != nil {
		return &api.SecuritySelfTestResultMessage{
			Ok:    false,
			Error: err.Error(),
		}, nil
	}

	return &api.SecuritySelfTestResultMessage{
		Ok:    true,
		Error: "",
	}, nil
}

func (a *APIServer) collectOSReleaseData() {}
