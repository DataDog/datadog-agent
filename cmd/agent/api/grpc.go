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
	"github.com/DataDog/datadog-agent/pkg/flare/remote"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	hostutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (s *server) FlareServiceHeartbeat(ctx context.Context, in *pb.FlareHeartbeatRequest) (*pb.FlareHeartbeatResponse, error) {
	remote.RegisterSource(in.TracerIdentifier, "APM", in.TracerService, in.TracerEnvironment)
	flare, err := remote.GetFlareForId(in.TracerIdentifier)
	if err != nil {
		return &pb.FlareHeartbeatResponse{}, err
	}

	if flare == nil {
		return &pb.FlareHeartbeatResponse{}, nil
	}

	trigger := &pb.FlareHeartbeatResponse_Trigger{
		FlareIdentifier: flare.Id,
		EndTime:         flare.Ts,
	}
	return &pb.FlareHeartbeatResponse{Trigger: trigger}, nil

}

func (s *server) FlareServiceQuery(ctx context.Context, in *pb.FlareQueryRequest) (*pb.FlareQueryResponse, error) {
	response := pb.FlareQueryResponse{}

	if in.Query == nil {
		return &response, nil
	}

	sources := remote.GetSourcesByServiceAndEnv(in.Query.TracerService, in.Query.TracerEnvironment)

	answer := []*pb.FlareHeartbeatRequest{}
	for id, s := range sources {
		answer = append(answer, &pb.FlareHeartbeatRequest{
			TracerIdentifier:  id,
			TracerService:     s.Service,
			TracerEnvironment: s.Env,
		})
	}

	response.Answer = answer
	return &response, nil

}

func (s *server) FlareLogEvent(ctx context.Context, in *pb.FlareLogRequest) (*pb.FlareLogResponse, error) {

	log.Info("Received log event request...")
	log.Infof("Params: %v", in)
	response := &pb.FlareLogResponse{
		// Continue: true,
	}
	return response, nil
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
