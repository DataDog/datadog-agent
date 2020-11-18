// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/flare/remote"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type grpcServer struct {
	pb.UnimplementedFlareServer
}

func (s *grpcServer) ServiceHeartbeat(ctx context.Context, in *pb.FlareHeartbeatRequest) (*pb.FlareHeartbeatResponse, error) {
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

func (s *grpcServer) ServiceQuery(ctx context.Context, in *pb.FlareQueryRequest) (*pb.FlareQueryResponse, error) {
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

func (s *grpcServer) LogEvent(ctx context.Context, in *pb.FlareLogRequest) (*pb.FlareLogResponse, error) {

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
func (s *grpcServer) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}
