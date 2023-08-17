// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

// Mocks

//nolint:revive
type MockRuntimeSecurityClient struct{}

//nolint:revive
func NewMockRuntimeSecurityClient() SecurityModuleClientWrapper {
	client := &MockRuntimeSecurityClient{}
	return client
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) DumpDiscarders() (string, error) {
	return "", nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) DumpProcessCache(withArgs bool) (string, error) {
	return "", nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GenerateActivityDump(request *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) ListActivityDumps() (*api.ActivityDumpListMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) StopActivityDump(name, containerid, comm string) (*api.ActivityDumpStopMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GenerateEncoding(request *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) DumpNetworkNamespace(snapshotInterfaces bool) (*api.DumpNetworkNamespaceMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GetConfig() (*api.SecurityConfigMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GetStatus() (*api.Status, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) RunSelfTest() (*api.SecuritySelfTestResultMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) ReloadPolicies() (*api.ReloadPoliciesResultMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GetRuleSetReport() (*api.GetRuleSetReportResultMessage, error) {
	return &api.GetRuleSetReportResultMessage{
		RuleSetReportMessage: &api.RuleSetReportMessage{
			Policies: []*api.EventTypePolicy{
				{
					EventType: "exec",
					Mode:      1,
					Flags:     math.MaxUint8,
					Approvers: nil,
				},
				{
					EventType: "open",
					Mode:      2,
					Flags:     math.MaxUint8,
					Approvers: &api.Approvers{
						ApproverDetails: []*api.ApproverDetails{
							{
								Field: "open.file.path",
								Value: "/etc/gshadow",
								Type:  1,
							},
							{
								Field: "open.file.path",
								Value: "/etc/shadow",
								Type:  1,
							},
							{
								Field: "open.flags",
								Value: "64",
								Type:  1,
							},
						},
					},
				},
			},
		},
	}, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GetEvents() (api.SecurityModule_GetEventsClient, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) GetActivityDumpStream() (api.SecurityModule_GetActivityDumpStreamClient, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) ListSecurityProfiles(includeCache bool) (*api.SecurityProfileListMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) SaveSecurityProfile(name string, tag string) (*api.SecurityProfileSaveMessage, error) {
	return nil, nil
}

//nolint:revive
func (rsc *MockRuntimeSecurityClient) Close() {
}
