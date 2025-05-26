// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"errors"
	"fmt"
	"io"
	"runtime"

	"google.golang.org/grpc"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/golang/protobuf/ptypes/empty"
)

// SecurityAgentAPIServer is used to send request to security module
type SecurityAgentAPIServer struct {
	grpcServer *module.GRPCServer
	apiServer  api.SecurityAgentAPIServer
}

// SecurityModuleClientWrapper represents a security module client
type SecurityEventAPIServerWrapper interface {
}

type SecurityEventAPIServer struct {
	api.UnimplementedSecurityAgentAPIServer
}

func (s *SecurityEventAPIServer) SendEvent(stream grpc.ClientStreamingServer[api.SecurityEventMessage, empty.Empty]) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break // read done.
		}

		if err != nil {
			return err
		}
		fmt.Printf(">>>>>>>>>>> Received event: %v\n", msg)
	}

	return nil
}

func (s *SecurityEventAPIServer) SendActivityDumpStream(grpc.ClientStreamingServer[api.ActivityDumpStreamMessage, empty.Empty]) error {

	fmt.Println("SendActivityDumpStream")
	return nil
}

func (s *SecurityAgentAPIServer) Start() {
	err := s.grpcServer.Start()
	if err != nil {
		seclog.Errorf("error starting security agent grpc server: %v", err)
	}
}

func (s *SecurityAgentAPIServer) Stop() {
	s.grpcServer.Stop()
}

// NewSecurityAgentAPIServer instantiates a new SecurityAgentAPIServer
func NewSecurityAgentAPIServer() (*SecurityAgentAPIServer, error) {
	cfgSocketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket")
	socketPath := cfgSocketPath

	if socketPath == "" {
		return nil, errors.New("runtime_security_config.socket_events must be set")
	}

	family := config.GetFamilyAddress(socketPath)
	if family == "unix" {
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("unix sockets are not supported on Windows")
		}

		socketPath = fmt.Sprintf("unix://%s", socketPath)
	}

	grpcServer := module.NewGRPCServer(family, cfgSocketPath)
	apiServer := &SecurityEventAPIServer{}

	api.RegisterSecurityAgentAPIServer(grpcServer.ServiceRegistrar(), apiServer)

	return &SecurityAgentAPIServer{
		grpcServer: grpcServer,
		apiServer:  apiServer,
	}, nil
}
