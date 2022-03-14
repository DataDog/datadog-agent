// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"errors"
	"net"

	"google.golang.org/grpc"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
)

// RuntimeSecurityClient is used to send request to security module
type RuntimeSecurityClient struct {
	apiClient api.SecurityModuleClient
	conn      *grpc.ClientConn
}

// DumpProcessCache send a dump request
func (c *RuntimeSecurityClient) DumpProcessCache(withArgs bool) (string, error) {
	response, err := c.apiClient.DumpProcessCache(context.Background(), &api.DumpProcessCacheParams{WithArgs: withArgs})
	if err != nil {
		return "", err
	}

	return response.Filename, nil
}

// GenerateActivityDump send a dump activity request
func (c *RuntimeSecurityClient) GenerateActivityDump(comm string, timeout int32, withGraph bool, differentiateArgs bool, outputDirectory string) (*api.SecurityActivityDumpMessage, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)
	return apiClient.DumpActivity(context.Background(), &api.DumpActivityParams{
		Comm:              comm,
		Timeout:           timeout,
		WithGraph:         withGraph,
		DifferentiateArgs: differentiateArgs,
		OutputDirectory:   outputDirectory,
	})
}

// ListActivityDumps lists the active activity dumps
func (c *RuntimeSecurityClient) ListActivityDumps() (*api.SecurityActivityDumpListMessage, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)
	return apiClient.ListActivityDumps(context.Background(), &api.ListActivityDumpsParams{})
}

// StopActivityDump stops an active dump if it exists
func (c *RuntimeSecurityClient) StopActivityDump(comm string) (*api.SecurityActivityDumpStoppedMessage, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)
	return apiClient.StopActivityDump(context.Background(), &api.StopActivityDumpParams{
		Comm: comm,
	})
}

// GenerateProfile generates a policy file from the provided activity dump
func (c *RuntimeSecurityClient) GenerateProfile(file string) (*api.SecurityProfileGeneratedMessage, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)
	return apiClient.GenerateProfile(context.Background(), &api.GenerateProfileParams{
		ActivityDumpFile: file,
	})
}

func (c *RuntimeSecurityClient) GenerateGraph(file string) (*api.SecurityGraphGeneratedMessage, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)
	return apiClient.GenerateGraph(context.Background(), &api.GenerateGraphParams{
		ActivityDumpFile: file,
	})
}

// GetConfig retrieves the config of the runtime security module
func (c *RuntimeSecurityClient) GetConfig() (*api.SecurityConfigMessage, error) {
	response, err := c.apiClient.GetConfig(context.Background(), &api.GetConfigParams{})
	if err != nil {
		return nil, err
	}
	return response, nil
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

// GetEvents returns a stream of events
func (c *RuntimeSecurityClient) GetEvents() (api.SecurityModule_GetEventsClient, error) {
	stream, err := c.apiClient.GetEvents(context.Background(), &api.GetEventParams{})
	if err != nil {
		return nil, err
	}
	return stream, nil
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

	conn, err := grpc.Dial(socketPath, grpc.WithInsecure(), grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return net.Dial("unix", url)
	}))
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityClient{
		conn:      conn,
		apiClient: api.NewSecurityModuleClient(conn),
	}, nil
}
