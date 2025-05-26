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

	empty "github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

// APIClient is used to send request to security module
type APIClient struct {
	apiClient api.SecurityEventsClient
	conn      *grpc.ClientConn
}

// SecurityModuleClientWrapper represents a security module client
type SecurityModuleClientWrapper interface {
	SendEvent(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[api.SecurityEventMessage, empty.Empty], error)
	SendActivityDumpStream(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[api.ActivityDumpStreamMessage, empty.Empty], error)
	Close()
}

func (c *APIClient) Start() {
	stream, err := c.apiClient.SendEvent(context.Background())
	if err != nil {
		fmt.Printf(">>>>>>>>>> Error starting grpc client: %v\n", err)
	}

	for {
		err := stream.Send(&api.SecurityEventMessage{
			RuleID: "test",
		})
		if err != nil {
			fmt.Printf(">>>>>>>>>>> Error sending event: %v\n", err)
		} else {
			fmt.Printf(">>>>>>>>>>> Sent event\n")
		}
		time.Sleep(1 * time.Second)
	}
}

// NewAPIClient instantiates a new APIClient
func NewAPIClient() (*APIClient, error) {
	socketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket")

	socketPath = "/tmp/runtime-security.sock"

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

	fmt.Printf(">>>>>>>>>>> Connected to %s\n", socketPath)

	if err != nil {
		return nil, err
	}

	return &APIClient{
		conn:      conn,
		apiClient: api.NewSecurityEventsClient(conn),
	}, nil
}
