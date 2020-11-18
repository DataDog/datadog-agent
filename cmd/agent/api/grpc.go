// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"

	pb "github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	hostutil "github.com/DataDog/datadog-agent/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnableExperimentalEndpoints enables experimental endpoints (set via -X)
var EnableExperimentalEndpoints bool

type server struct {
	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentServer
}

func (s *server) GetHostname(ctx context.Context, in *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := hostutil.GetHostname()
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *server) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) GetTags(ctx context.Context, in *pb.TagRequest) (*pb.TagReply, error) {
	if EnableExperimentalEndpoints {
		tags, _ := tagger.Tag(in.GetEntity(), collectors.HighCardinality)
		return &pb.TagReply{Tags: tags}, nil
	}

	return nil, status.Errorf(codes.PermissionDenied,
		"This is an experimental endpoint and has been disabled in this build")
}
