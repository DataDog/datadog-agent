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
	"os"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"google.golang.org/protobuf/proto"

	model "github.com/DataDog/agent-payload/v5/sbom"
)

// DumpDiscarders handles discarder dump requests
func (a *APIServer) DumpDiscarders(_ context.Context, _ *api.DumpDiscardersParams) (*api.DumpDiscardersMessage, error) {
	filePath, err := a.probe.DumpDiscarders()
	if err != nil {
		return nil, err
	}
	seclog.Infof("Discarder dump file path: %s", filePath)

	return &api.DumpDiscardersMessage{DumpFilename: filePath}, nil
}

// DumpProcessCache handles process cache dump requests
func (a *APIServer) DumpProcessCache(_ context.Context, params *api.DumpProcessCacheParams) (*api.SecurityDumpProcessCacheMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	var (
		filename string
		err      error
	)

	switch params.Format {
	case "json":
		jsonContent, err := p.Resolvers.ProcessResolver.ToJSON(true)
		if err != nil {
			return nil, err
		}

		dump, err := os.CreateTemp("/tmp", "process-cache-dump-*.json")
		if err != nil {
			return nil, err
		}

		defer dump.Close()

		filename = dump.Name()
		if err := os.Chmod(dump.Name(), 0400); err != nil {
			return nil, err
		}

		if _, err := dump.Write(jsonContent); err != nil {
			return nil, err
		}

	case "dot", "":
		filename, err = p.Resolvers.ProcessResolver.ToDot(params.WithArgs)
		if err != nil {
			return nil, err
		}
	}

	return &api.SecurityDumpProcessCacheMessage{
		Filename: filename,
	}, nil
}

// DumpActivity handles an activity dump request
func (a *APIServer) DumpActivity(_ context.Context, params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	if manager := p.GetProfileManager(); manager != nil {
		msg, err := manager.DumpActivity(params)
		if err != nil {
			seclog.Errorf("%s", err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// ListActivityDumps returns the list of active dumps
func (a *APIServer) ListActivityDumps(_ context.Context, params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	if manager := p.GetProfileManager(); manager != nil {
		msg, err := manager.ListActivityDumps(params)
		if err != nil {
			seclog.Errorf("%s", err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// StopActivityDump stops an active activity dump if it exists
func (a *APIServer) StopActivityDump(_ context.Context, params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	if manager := p.GetProfileManager(); manager != nil {
		msg, err := manager.StopActivityDump(params)
		if err != nil {
			seclog.Errorf("%s", err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// TranscodingRequest encodes an activity dump following the requested parameters
func (a *APIServer) TranscodingRequest(_ context.Context, params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	if manager := p.GetProfileManager(); manager != nil {
		msg, err := manager.GenerateTranscoding(params)
		if err != nil {
			seclog.Errorf("%s", err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// ListSecurityProfiles returns the list of security profiles
func (a *APIServer) ListSecurityProfiles(_ context.Context, params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	if manager := p.GetProfileManager(); manager != nil {
		msg, err := manager.ListSecurityProfiles(params)
		if err != nil {
			seclog.Errorf("%s", err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// SaveSecurityProfile saves the requested security profile to disk
func (a *APIServer) SaveSecurityProfile(_ context.Context, params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	if manager := p.GetProfileManager(); manager != nil {
		msg, err := manager.SaveSecurityProfile(params)
		if err != nil {
			seclog.Errorf("%s", err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// GetStatus returns the status of the module
func (a *APIServer) GetStatus(_ context.Context, _ *api.GetStatusParams) (*api.Status, error) {
	var apiStatus api.Status
	if a.selfTester != nil {
		apiStatus.SelfTests = a.selfTester.GetStatus()
	}
	apiStatus.PoliciesStatus = a.policiesStatus

	seclVariables := a.GetSECLVariables()
	for _, seclVariable := range seclVariables {
		apiStatus.SECLVariables = append(apiStatus.SECLVariables, seclVariable)
	}

	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if ok {
		status, err := p.GetConstantFetcherStatus()
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

		apiStatus.Environment = &api.EnvironmentStatus{
			Constants: &api.ConstantFetcherStatus{
				Fetchers: status.Fetchers,
				Values:   constants,
			},
			KernelLockdown:  string(kernel.GetLockdownMode()),
			UseMmapableMaps: p.GetKernelVersion().HaveMmapableMaps(),
			UseRingBuffer:   p.GetUseRingBuffers(),
			UseFentry:       p.GetUseFentry(),
		}

		envErrors := p.VerifyEnvironment()
		if envErrors != nil {
			apiStatus.Environment.Warnings = make([]string, len(envErrors.Errors))
			for i, err := range envErrors.Errors {
				apiStatus.Environment.Warnings[i] = err.Error()
			}
		}
	}

	return &apiStatus, nil
}

// DumpNetworkNamespace handles network namespace cache dump requests
func (a *APIServer) DumpNetworkNamespace(_ context.Context, params *api.DumpNetworkNamespaceParams) (*api.DumpNetworkNamespaceMessage, error) {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, fmt.Errorf("not supported")
	}

	return p.Resolvers.NamespaceResolver.DumpNetworkNamespaces(params), nil
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

func (a *APIServer) collectOSReleaseData() {
	p, ok := a.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return
	}

	kv := p.GetKernelVersion()

	a.kernelVersion = kv.Code.String()
	a.distribution = fmt.Sprintf("%s - %s", kv.OsRelease["ID"], kv.OsRelease["VERSION_ID"])
}

func (a *APIServer) collectSBOMS() {
	if sbomResolver := a.probe.PlatformProbe.(*probe.EBPFProbe).Resolvers.SBOMResolver; sbomResolver != nil {
		if err := sbomResolver.RegisterListener(sbom.SBOMComputed, func(sbom *model.SBOMEntity) {
			select {
			case a.sboms <- sbom:
			default:
				seclog.Warnf("dropping SBOM event")
			}
		}); err != nil {
			seclog.Errorf("failed to register SBOM listener: %s", err)
		}
	}
}

// GetSBOMStream handles SBOM stream requests
func (a *APIServer) GetSBOMStream(_ *sbompb.SBOMStreamParams, stream sbompb.SBOMCollector_GetSBOMStreamServer) error {
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-a.stopChan:
			return nil
		case sbom := <-a.sboms:
			bytes, err := proto.Marshal(sbom)
			if err != nil {
				seclog.Errorf("failed to convert SBOM: %s", err)
			}

			msg := &sbompb.SBOMMessage{
				Data: bytes,
				Kind: string(workloadmeta.KindContainer),
				ID:   sbom.Id,
			}

			if err := stream.Send(msg); err != nil {
				return fmt.Errorf("failed to send SBOM: %s", err)
			}
		}
	}
}
