// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/golang/protobuf/ptypes/empty"
)

// RuntimeSecurityCmdClient is used to send request to security module
type RuntimeSecurityCmdClient struct {
	apiClient api.SecurityModuleCmdClient
	conn      *grpc.ClientConn
}

// RuntimeSecurityEventClient is used to send request to security module
type RuntimeSecurityEventClient struct {
	eventClient api.SecurityModuleEventClient
	conn        *grpc.ClientConn
}

// SecurityModuleCmdClientWrapper represents a security module client
type SecurityModuleCmdClientWrapper interface {
	DumpDiscarders() (string, error)
	DumpProcessCache(withArgs bool, format string) (string, error)
	GenerateActivityDump(request *api.ActivityDumpParams) (*api.ActivityDumpMessage, error)
	ListActivityDumps() (*api.ActivityDumpListMessage, error)
	StopActivityDump(name, container, cgroup string) (*api.ActivityDumpStopMessage, error)
	GenerateEncoding(request *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error)
	DumpNetworkNamespace(snapshotInterfaces bool) (*api.DumpNetworkNamespaceMessage, error)
	GetConfig() (*api.SecurityConfigMessage, error)
	GetStatus() (*api.Status, error)
	RunSelfTest() (*api.SecuritySelfTestResultMessage, error)
	ReloadPolicies() (*api.ReloadPoliciesResultMessage, error)
	GetRuleSetReport() (*api.GetRuleSetReportMessage, error)
	ListSecurityProfiles(includeCache bool) (*api.SecurityProfileListMessage, error)
	SaveSecurityProfile(name string, tag string) (*api.SecurityProfileSaveMessage, error)
	Close()
}

// DumpDiscarders sends a request to dump discarders
func (c *RuntimeSecurityCmdClient) DumpDiscarders() (string, error) {
	response, err := c.apiClient.DumpDiscarders(context.Background(), &api.DumpDiscardersParams{})
	if err != nil {
		return "", err
	}

	return response.DumpFilename, nil
}

// DumpProcessCache sends a process cache dump request
func (c *RuntimeSecurityCmdClient) DumpProcessCache(withArgs bool, format string) (string, error) {
	response, err := c.apiClient.DumpProcessCache(context.Background(), &api.DumpProcessCacheParams{WithArgs: withArgs, Format: format})
	if err != nil {
		return "", err
	}

	return response.Filename, nil
}

// ListActivityDumps lists the active activity dumps
func (c *RuntimeSecurityCmdClient) ListActivityDumps() (*api.ActivityDumpListMessage, error) {
	return c.apiClient.ListActivityDumps(context.Background(), &api.ActivityDumpListParams{})
}

// GenerateActivityDump send a dump activity request
func (c *RuntimeSecurityCmdClient) GenerateActivityDump(request *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	return c.apiClient.DumpActivity(context.Background(), request)
}

// StopActivityDump stops an active dump if it exists
func (c *RuntimeSecurityCmdClient) StopActivityDump(name, container, cgroup string) (*api.ActivityDumpStopMessage, error) {
	return c.apiClient.StopActivityDump(context.Background(), &api.ActivityDumpStopParams{
		Name:        name,
		ContainerID: container,
		CGroupID:    cgroup,
	})
}

// GenerateEncoding sends a transcoding request
func (c *RuntimeSecurityCmdClient) GenerateEncoding(request *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	return c.apiClient.TranscodingRequest(context.Background(), request)
}

// DumpNetworkNamespace sends a network namespace cache dump request
func (c *RuntimeSecurityCmdClient) DumpNetworkNamespace(snapshotInterfaces bool) (*api.DumpNetworkNamespaceMessage, error) {
	return c.apiClient.DumpNetworkNamespace(context.Background(), &api.DumpNetworkNamespaceParams{SnapshotInterfaces: snapshotInterfaces})
}

// GetConfig retrieves the config of the runtime security module
func (c *RuntimeSecurityCmdClient) GetConfig() (*api.SecurityConfigMessage, error) {
	response, err := c.apiClient.GetConfig(context.Background(), &api.GetConfigParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetStatus returns the status of the module
func (c *RuntimeSecurityCmdClient) GetStatus() (*api.Status, error) {
	apiClient := api.NewSecurityModuleCmdClient(c.conn)
	return apiClient.GetStatus(context.Background(), &api.GetStatusParams{})
}

// RunSelfTest instructs the system probe to run a self test
func (c *RuntimeSecurityCmdClient) RunSelfTest() (*api.SecuritySelfTestResultMessage, error) {
	response, err := c.apiClient.RunSelfTest(context.Background(), &api.RunSelfTestParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// ReloadPolicies instructs the system probe to reload its policies
func (c *RuntimeSecurityCmdClient) ReloadPolicies() (*api.ReloadPoliciesResultMessage, error) {
	response, err := c.apiClient.ReloadPolicies(context.Background(), &api.ReloadPoliciesParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetRuleSetReport gets the currently ruleset loaded status
func (c *RuntimeSecurityCmdClient) GetRuleSetReport() (*api.GetRuleSetReportMessage, error) {
	response, err := c.apiClient.GetRuleSetReport(context.Background(), &api.GetRuleSetReportParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// ListSecurityProfiles lists the profiles held in memory by the Security Profile manager
func (c *RuntimeSecurityCmdClient) ListSecurityProfiles(includeCache bool) (*api.SecurityProfileListMessage, error) {
	return c.apiClient.ListSecurityProfiles(context.Background(), &api.SecurityProfileListParams{
		IncludeCache: includeCache,
	})
}

// SaveSecurityProfile saves the requested security profile to disk
func (c *RuntimeSecurityCmdClient) SaveSecurityProfile(name string, tag string) (*api.SecurityProfileSaveMessage, error) {
	return c.apiClient.SaveSecurityProfile(context.Background(), &api.SecurityProfileSaveParams{
		Selector: &api.WorkloadSelectorMessage{
			Name: name,
			Tag:  tag,
		},
	})
}

// Close closes the connection
func (c *RuntimeSecurityCmdClient) Close() {
	c.conn.Close()
}

// GetEventStream returns a stream of events. Communication security-agent -> system-probe
func (c *RuntimeSecurityEventClient) GetEventStream() (api.SecurityModuleEvent_GetEventStreamClient, error) {
	stream, err := c.eventClient.GetEventStream(context.Background(), &empty.Empty{})
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// GetActivityDumpStream returns a stream of activity dumps. Communication security-agent -> system-probe
func (c *RuntimeSecurityEventClient) GetActivityDumpStream() (api.SecurityModuleEvent_GetActivityDumpStreamClient, error) {
	stream, err := c.eventClient.GetActivityDumpStream(context.Background(), &empty.Empty{})
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// NewRuntimeSecurityCmdClient instantiates a new RuntimeSecurityCmdClient
func NewRuntimeSecurityCmdClient() (*RuntimeSecurityCmdClient, error) {
	socketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket")
	cmdSocketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.cmd_socket")

	cmdSocketPath, err := common.GetCmdSocketPath(socketPath, cmdSocketPath)
	if err != nil {
		return nil, err
	}

	family := common.GetFamilyAddress(cmdSocketPath)
	if family == "unix" {
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("unix sockets are not supported on Windows")
		}

		cmdSocketPath = fmt.Sprintf("unix://%s", cmdSocketPath)
	}

	conn, err := grpc.NewClient(
		cmdSocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(api.VTProtoCodecName)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay: time.Second,
				MaxDelay:  time.Second,
			},
		}))
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityCmdClient{
		conn:      conn,
		apiClient: api.NewSecurityModuleCmdClient(conn),
	}, nil
}

// Close closes the connection
func (c *RuntimeSecurityEventClient) Close() {
	c.conn.Close()
}

// NewRuntimeSecurityEventClient instantiates a new RuntimeSecurityEventClient
func NewRuntimeSecurityEventClient() (*RuntimeSecurityEventClient, error) {
	socketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket")

	family := common.GetFamilyAddress(socketPath)
	if family == "unix" {
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("unix sockets are not supported on Windows")
		}

		socketPath = fmt.Sprintf("unix://%s", socketPath)
	}

	conn, err := grpc.NewClient(
		socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(api.VTProtoCodecName)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay: time.Second,
				MaxDelay:  time.Second,
			},
		}))
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityEventClient{
		conn:        conn,
		eventClient: api.NewSecurityModuleEventClient(conn),
	}, nil
}
