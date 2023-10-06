// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// DumpDiscarders handles discarder dump requests
func (a *APIServer) DumpDiscarders(ctx context.Context, params *api.DumpDiscardersParams) (*api.DumpDiscardersMessage, error) {
	filePath, err := a.probe.DumpDiscarders()
	if err != nil {
		return nil, err
	}
	seclog.Infof("Discarder dump file path: %s", filePath)

	return &api.DumpDiscardersMessage{DumpFilename: filePath}, nil
}

// DumpProcessCache handles process cache dump requests
func (a *APIServer) DumpProcessCache(ctx context.Context, params *api.DumpProcessCacheParams) (*api.SecurityDumpProcessCacheMessage, error) {
	resolvers := a.probe.GetResolvers()

	filename, err := resolvers.ProcessResolver.Dump(params.WithArgs)
	if err != nil {
		return nil, err
	}

	return &api.SecurityDumpProcessCacheMessage{
		Filename: filename,
	}, nil
}

// DumpActivity handle an activity dump request
func (a *APIServer) DumpActivity(ctx context.Context, params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if managers := a.probe.GetProfileManagers(); managers != nil {
		msg, err := managers.DumpActivity(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// ListActivityDumps returns the list of active dumps
func (a *APIServer) ListActivityDumps(ctx context.Context, params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if managers := a.probe.GetProfileManagers(); managers != nil {
		msg, err := managers.ListActivityDumps(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// StopActivityDump stops an active activity dump if it exists
func (a *APIServer) StopActivityDump(ctx context.Context, params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if managers := a.probe.GetProfileManagers(); managers != nil {
		msg, err := managers.StopActivityDump(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// TranscodingRequest encodes an activity dump following the requested parameters
func (a *APIServer) TranscodingRequest(ctx context.Context, params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if managers := a.probe.GetProfileManagers(); managers != nil {
		msg, err := managers.GenerateTranscoding(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// ListSecurityProfiles returns the list of security profiles
func (a *APIServer) ListSecurityProfiles(ctx context.Context, params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	if managers := a.probe.GetProfileManagers(); managers != nil {
		msg, err := managers.ListSecurityProfiles(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// SaveSecurityProfile saves the requested security profile to disk
func (a *APIServer) SaveSecurityProfile(ctx context.Context, params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	if managers := a.probe.GetProfileManagers(); managers != nil {
		msg, err := managers.SaveSecurityProfile(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// GetStatus returns the status of the module
func (a *APIServer) GetStatus(ctx context.Context, params *api.GetStatusParams) (*api.Status, error) {
	status, err := a.probe.GetConstantFetcherStatus()
	if err != nil {
		return nil, err
	}

	constants := make([]*api.ConstantValueAndSource, 0, len(status.Values))
	for _, v := range status.Values {
		constants = append(constants, &api.ConstantValueAndSource{
			ID:     v.ID,
			Value:  v.Value,
			Source: v.FetcherName,
		})
	}

	apiStatus := &api.Status{
		Environment: &api.EnvironmentStatus{
			Constants: &api.ConstantFetcherStatus{
				Fetchers: status.Fetchers,
				Values:   constants,
			},
		},
		SelfTests: a.selfTester.GetStatus(),
	}

	envErrors := a.probe.VerifyEnvironment()
	if envErrors != nil {
		apiStatus.Environment.Warnings = make([]string, len(envErrors.Errors))
		for i, err := range envErrors.Errors {
			apiStatus.Environment.Warnings[i] = err.Error()
		}
	}

	apiStatus.Environment.KernelLockdown = string(kernel.GetLockdownMode())

	if kernel, err := a.probe.GetKernelVersion(); err == nil {
		apiStatus.Environment.UseMmapableMaps = kernel.HaveMmapableMaps()
		apiStatus.Environment.UseRingBuffer = a.probe.UseRingBuffers()
	}

	return apiStatus, nil
}

// DumpNetworkNamespace handles network namespace cache dump requests
func (a *APIServer) DumpNetworkNamespace(ctx context.Context, params *api.DumpNetworkNamespaceParams) (*api.DumpNetworkNamespaceMessage, error) {
	return a.probe.GetResolvers().NamespaceResolver.DumpNetworkNamespaces(params), nil
}

// RunSelfTest runs self test and then reload the current policies
func (a *APIServer) RunSelfTest(ctx context.Context, params *api.RunSelfTestParams) (*api.SecuritySelfTestResultMessage, error) {
	if a.cwsConsumer == nil {
		return nil, errors.New("failed to found module in APIServer")
	}

	if a.selfTester == nil {
		return &api.SecuritySelfTestResultMessage{
			Ok:    false,
			Error: "self-tests are disabled",
		}, nil
	}

	if _, err := a.cwsConsumer.RunSelfTest(false); err != nil {
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
