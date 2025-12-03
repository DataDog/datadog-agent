// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements the gRPC server for workloadfilter evaluations.
package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/proto"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Server exposes workloadfilter evaluation helpers to remote agents.
type Server struct {
	store workloadfilter.Component
}

// NewServer returns a new workloadfilter gRPC server instance.
func NewServer(store workloadfilter.Component) *Server {
	return &Server{store: store}
}

// WorkloadFilterEvaluate evaluates the requested filter program.
func (s *Server) WorkloadFilterEvaluate(_ context.Context, req *pb.WorkloadFilterEvaluateRequest) (*pb.WorkloadFilterEvaluateResponse, error) {
	if s == nil || s.store == nil {
		return nil, status.Error(codes.Unimplemented, "workloadfilter server not available")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	entity, err := proto.ExtractFilterable(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workload payload: %v", err)
	}

	result, err := s.store.Evaluate(req.GetProgramName(), entity)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}

	return &pb.WorkloadFilterEvaluateResponse{Result: proto.FromWorkloadFilterResult(result)}, nil
}
