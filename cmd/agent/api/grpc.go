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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

func (s *server) ServiceHeartbeat(ctx context.Context, in *pb.FlareHeartbeatRequest) (*pb.FlareHeartbeatResponse, error) {
	trigger := &pb.FlareHeartbeatResponse_Trigger{
		FlareIdentifier: 1234,
		LogLevel:        pb.FlareLogLevel_INFO,
		DurationSeconds: 60,
	}
	return &pb.FlareHeartbeatResponse{Trigger: trigger}, nil
}

func (s *server) FlareLogEvent(ctx context.Context, in *pb.FlareLogRequest) (*pb.FlareLogResponse, error) {

	log.Info("Received log event request...")
	log.Infof("Params: %v", in)
	response := &pb.FlareLogResponse{
		// Continue: true,
	}
	return response, nil
}

// AuthFuncOverride will override the AuthFunc registered with the unary interceptor
func (s *server) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) GetTags(ctx context.Context, in *pb.TagRequest) (*pb.TagReply, error) {
	tags, _ := tagger.Tag(in.GetEntity(), collectors.HighCardinality)
	return &pb.TagReply{Tags: tags}, nil
}
