// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"context"
	"errors"
	"net"
	"runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

// RuntimeSecurityClient is used to send request to security module
type RuntimeSecurityClient struct {
	apiClient api.SecurityModuleClient
	conn      *grpc.ClientConn
}

// SecurityModuleClientWrapper represents a security module client
type SecurityModuleClientWrapper interface {
	DumpDiscarders() (string, error)
	DumpProcessCache(withArgs bool) (string, error)
	GenerateActivityDump(request *api.ActivityDumpParams) (*api.ActivityDumpMessage, error)
	ListActivityDumps() (*api.ActivityDumpListMessage, error)
	StopActivityDump(name, containerid, comm string) (*api.ActivityDumpStopMessage, error)
	GenerateEncoding(request *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error)
	DumpNetworkNamespace(snapshotInterfaces bool) (*api.DumpNetworkNamespaceMessage, error)
	GetConfig() (*api.SecurityConfigMessage, error)
	GetStatus() (*api.Status, error)
	RunSelfTest() (*api.SecuritySelfTestResultMessage, error)
	ReloadPolicies() (*api.ReloadPoliciesResultMessage, error)
	GetRuleSetReport() (*api.GetRuleSetReportResultMessage, error)
	GetEvents() (api.SecurityModule_GetEventsClient, error)
	GetActivityDumpStream() (api.SecurityModule_GetActivityDumpStreamClient, error)
	ListSecurityProfiles(includeCache bool) (*api.SecurityProfileListMessage, error)
	SaveSecurityProfile(name string, tag string) (*api.SecurityProfileSaveMessage, error)
	Close()
}

// DumpDiscarders sends a request to dump discarders
func (c *RuntimeSecurityClient) DumpDiscarders() (string, error) {
	response, err := c.apiClient.DumpDiscarders(context.Background(), &api.DumpDiscardersParams{})
	if err != nil {
		return "", err
	}

	return response.DumpFilename, nil
}

// DumpProcessCache sends a process cache dump request
func (c *RuntimeSecurityClient) DumpProcessCache(withArgs bool) (string, error) {
	response, err := c.apiClient.DumpProcessCache(context.Background(), &api.DumpProcessCacheParams{WithArgs: withArgs})
	if err != nil {
		return "", err
	}

	return response.Filename, nil
}

// GenerateActivityDump send a dump activity request
func (c *RuntimeSecurityClient) GenerateActivityDump(request *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	return c.apiClient.DumpActivity(context.Background(), request)
}

// ListActivityDumps lists the active activity dumps
func (c *RuntimeSecurityClient) ListActivityDumps() (*api.ActivityDumpListMessage, error) {
	return c.apiClient.ListActivityDumps(context.Background(), &api.ActivityDumpListParams{})
}

// StopActivityDump stops an active dump if it exists
func (c *RuntimeSecurityClient) StopActivityDump(name, containerid, comm string) (*api.ActivityDumpStopMessage, error) {
	return c.apiClient.StopActivityDump(context.Background(), &api.ActivityDumpStopParams{
		Comm:        comm,
		Name:        name,
		ContainerID: containerid,
	})
}

// GenerateEncoding sends a transcoding request
func (c *RuntimeSecurityClient) GenerateEncoding(request *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	return c.apiClient.TranscodingRequest(context.Background(), request)
}

// DumpNetworkNamespace sends a network namespace cache dump request
func (c *RuntimeSecurityClient) DumpNetworkNamespace(snapshotInterfaces bool) (*api.DumpNetworkNamespaceMessage, error) {
	return c.apiClient.DumpNetworkNamespace(context.Background(), &api.DumpNetworkNamespaceParams{SnapshotInterfaces: snapshotInterfaces})
}

// GetConfig retrieves the config of the runtime security module
func (c *RuntimeSecurityClient) GetConfig() (*api.SecurityConfigMessage, error) {
	response, err := c.apiClient.GetConfig(context.Background(), &api.GetConfigParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetStatus returns the status of the module
func (c *RuntimeSecurityClient) GetStatus() (*api.Status, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)
	return apiClient.GetStatus(context.Background(), &api.GetStatusParams{})
}

// RunSelfTest instructs the system probe to run a self test
func (c *RuntimeSecurityClient) RunSelfTest() (*api.SecuritySelfTestResultMessage, error) {
	response, err := c.apiClient.RunSelfTest(context.Background(), &api.RunSelfTestParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// ReloadPolicies instructs the system probe to reload its policies
func (c *RuntimeSecurityClient) ReloadPolicies() (*api.ReloadPoliciesResultMessage, error) {
	response, err := c.apiClient.ReloadPolicies(context.Background(), &api.ReloadPoliciesParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetRuleSetReport gets the currently loaded policies from the system probe
func (c *RuntimeSecurityClient) GetRuleSetReport() (*api.GetRuleSetReportResultMessage, error) {
	response, err := c.apiClient.GetRuleSetReport(context.Background(), &api.GetRuleSetReportParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetEvents returns a stream of events
func (c *RuntimeSecurityClient) GetEvents() (api.SecurityModule_GetEventsClient, error) {
	stream, err := c.apiClient.GetEvents(context.Background(), &api.GetEventParams{})
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// GetActivityDumpStream returns a stream of activity dumps
func (c *RuntimeSecurityClient) GetActivityDumpStream() (api.SecurityModule_GetActivityDumpStreamClient, error) {
	stream, err := c.apiClient.GetActivityDumpStream(context.Background(), &api.ActivityDumpStreamParams{})
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// ListSecurityProfiles lists the profiles held in memory by the Security Profile manager
func (c *RuntimeSecurityClient) ListSecurityProfiles(includeCache bool) (*api.SecurityProfileListMessage, error) {
	return c.apiClient.ListSecurityProfiles(context.Background(), &api.SecurityProfileListParams{
		IncludeCache: includeCache,
	})
}

// SaveSecurityProfile saves the requested security profile to disk
func (c *RuntimeSecurityClient) SaveSecurityProfile(name string, tag string) (*api.SecurityProfileSaveMessage, error) {
	return c.apiClient.SaveSecurityProfile(context.Background(), &api.SecurityProfileSaveParams{
		Selector: &api.WorkloadSelectorMessage{
			Name: name,
			Tag:  tag,
		},
	})
}

// Close closes the connection
func (c *RuntimeSecurityClient) Close() {
	c.conn.Close()
}

// NewRuntimeSecurityClient instantiates a new RuntimeSecurityClient
func NewRuntimeSecurityClient() (*RuntimeSecurityClient, error) {
	socketPath := coreconfig.Datadog.GetString("runtime_security_config.socket")
	if socketPath == "" {
		return nil, errors.New("runtime_security_config.socket must be set")
	}

	conn, err := grpc.Dial(
		socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(api.VTProtoCodecName)),
		grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
			if runtime.GOOS == "windows" {
				return net.Dial("tcp", url)
			}
			return net.Dial("unix", url)
		}),
	)
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityClient{
		conn:      conn,
		apiClient: api.NewSecurityModuleClient(conn),
	}, nil
}
