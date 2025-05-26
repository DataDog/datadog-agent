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
	"github.com/golang/protobuf/ptypes/empty"
)

// RuntimeSecurityServer is used to send request to security module
type RuntimeSecurityServer struct {
	grpcServer *module.GRPCServer
	apiServer  api.SecurityEventsServer
}

// SecurityModuleClientWrapper represents a security module client
type SecurityEventAPIServerWrapper interface {
}

type SecurityEventAPIServer struct {
	api.UnimplementedSecurityEventsServer
}

func (s *SecurityEventAPIServer) SendEvent(stream grpc.ClientStreamingServer[api.SecurityEventMessage, empty.Empty]) error {
	fmt.Println("SendEvent")

	for {
		msg, err := stream.Recv()
		fmt.Printf(">>>>>>>>>>> Received event: %v %v\n", msg, err)
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

func (s *RuntimeSecurityServer) Start() {
	err := s.grpcServer.Start()
	if err != nil {
		fmt.Printf(">>>>>>>>>> Error starting grpc server: %v\n", err)
	}
}

func (s *RuntimeSecurityServer) Stop() {
	s.grpcServer.Stop()
}

// NewRuntimeSecurityServer instantiates a new RuntimeSecurityServer
func NewRuntimeSecurityServer() (*RuntimeSecurityServer, error) {
	cfgSocketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket_events")

	cfgSocketPath = "/tmp/runtime-security.sock"
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

	api.RegisterSecurityEventsServer(grpcServer.ServiceRegistrar(), apiServer)

	fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n")

	return &RuntimeSecurityServer{
		grpcServer: grpcServer,
		apiServer:  apiServer,
	}, nil
}
