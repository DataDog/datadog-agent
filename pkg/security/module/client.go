// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package module

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// SecurityAgentAPIClient is used to send request to security module
type SecurityAgentAPIClient struct {
	SecurityAgentAPIClient api.SecurityAgentAPIClient
	conn                   *grpc.ClientConn
}

// SendEvents sends events to the security agent
func (c *SecurityAgentAPIClient) SendEvents(ctx context.Context, msgs chan *api.SecurityEventMessage) {
	for {
		seclog.Debugf("sending events to security agent grpc client")

		stream, err := c.SecurityAgentAPIClient.SendEvent(context.Background())
		if err != nil {
			seclog.Warnf("error starting security agent grpc client: %v", err)

			// Wait for 1 second before trying to send events again
			time.Sleep(time.Second)
			continue
		}

	LOOP:
		for {
			select {
			case msg := <-msgs:
				err := stream.Send(msg)
				if err != nil {
					seclog.Errorf("error sending event: %v", err)
					break LOOP
				}
			case <-ctx.Done():
				return
			}
		}

		_, _ = stream.CloseAndRecv()

		// Wait for 1 second before trying to send events again
		time.Sleep(time.Second)
	}
}

// SendActivityDumps sends activity dumps to the security agent
func (c *SecurityAgentAPIClient) SendActivityDumps(ctx context.Context, msgs chan *api.ActivityDumpStreamMessage) {
	for {
		seclog.Debugf("sending events to security agent grpc client")

		stream, err := c.SecurityAgentAPIClient.SendActivityDumpStream(context.Background())
		if err != nil {
			seclog.Warnf("error starting security agent grpc client: %v", err)

			// Wait for 1 second before trying to send events again
			time.Sleep(time.Second)
			continue
		}

	LOOP:
		for {
			select {
			case msg := <-msgs:
				err := stream.Send(msg)
				if err != nil {
					seclog.Errorf("error sending event: %v", err)
					break LOOP
				}
			case <-ctx.Done():
				return
			}
		}

		_, _ = stream.CloseAndRecv()

		// Wait for 1 second before trying to send events again
		time.Sleep(time.Second)
	}
}

// NewSecurityAgentAPIClient instantiates a new SecurityAgentAPIClient
func NewSecurityAgentAPIClient(cfg *config.RuntimeSecurityConfig) (*SecurityAgentAPIClient, error) {
	socketPath := cfg.SocketPath
	if socketPath == "" {
		return nil, errors.New("runtime_security_config.socket must be set")
	}

	family := config.GetFamilyAddress(socketPath)
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

	return &SecurityAgentAPIClient{
		conn:                   conn,
		SecurityAgentAPIClient: api.NewSecurityAgentAPIClient(conn),
	}, nil
}
