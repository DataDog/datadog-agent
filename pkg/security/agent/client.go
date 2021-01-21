// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"context"
	"errors"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"google.golang.org/grpc"
)

// RuntimeSecurityClient is used to send request to security module
type RuntimeSecurityClient struct {
	conn *grpc.ClientConn
}

// DumpProcessCache send a dump request
func (c *RuntimeSecurityClient) DumpProcessCache() (string, error) {
	apiClient := api.NewSecurityModuleClient(c.conn)

	response, err := apiClient.DumpProcessCache(context.Background(), &api.DumpProcessCacheParams{})
	if err != nil {
		return "", err
	}

	return response.Filename, nil
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

	path := "unix://" + socketPath
	conn, err := grpc.Dial(path, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityClient{
		conn: conn,
	}, nil
}
