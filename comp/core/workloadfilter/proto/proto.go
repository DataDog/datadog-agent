// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package proto contains protobuf helper functions for the workloadfilter component.
package proto

import (
	"errors"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// NewEvaluateRequest builds a WorkloadFilterEvaluateRequest from a name and an entity.
func NewEvaluateRequest(name string, entity workloadfilter.Filterable) (*pb.WorkloadFilterEvaluateRequest, error) {
	req := &pb.WorkloadFilterEvaluateRequest{
		ProgramName: name,
	}

	switch e := entity.(type) {
	case *workloadfilter.Container:
		req.Workload = &pb.WorkloadFilterEvaluateRequest_Container{Container: e.FilterContainer}
	case *workloadfilter.Pod:
		req.Workload = &pb.WorkloadFilterEvaluateRequest_Pod{Pod: e.FilterPod}
	case *workloadfilter.Process:
		req.Workload = &pb.WorkloadFilterEvaluateRequest_Process{Process: e.FilterProcess}
	case *workloadfilter.KubeService:
		req.Workload = &pb.WorkloadFilterEvaluateRequest_KubeService{KubeService: e.FilterKubeService}
	case *workloadfilter.KubeEndpoint:
		req.Workload = &pb.WorkloadFilterEvaluateRequest_KubeEndpoint{KubeEndpoint: e.FilterKubeEndpoint}
	case nil:
		return nil, errors.New("nil entity is not supported workload type")
	default:
		return nil, errors.New("unsupported workload type: " + string(e.Type()))
	}

	return req, nil
}

// ExtractFilterable converts a WorkloadFilterEvaluateRequest to a Filterable object.
func ExtractFilterable(req *pb.WorkloadFilterEvaluateRequest) (workloadfilter.Filterable, error) {
	switch payload := req.GetWorkload().(type) {
	case *pb.WorkloadFilterEvaluateRequest_Container:
		if payload.Container == nil {
			return nil, errors.New("missing container payload")
		}
		return &workloadfilter.Container{FilterContainer: payload.Container}, nil
	case *pb.WorkloadFilterEvaluateRequest_Pod:
		if payload.Pod == nil {
			return nil, errors.New("missing pod payload")
		}
		return &workloadfilter.Pod{FilterPod: payload.Pod}, nil
	case *pb.WorkloadFilterEvaluateRequest_Process:
		if payload.Process == nil {
			return nil, errors.New("missing process payload")
		}
		return &workloadfilter.Process{FilterProcess: payload.Process}, nil
	case *pb.WorkloadFilterEvaluateRequest_KubeService:
		if payload.KubeService == nil {
			return nil, errors.New("missing kube_service payload")
		}
		return &workloadfilter.KubeService{FilterKubeService: payload.KubeService}, nil
	case *pb.WorkloadFilterEvaluateRequest_KubeEndpoint:
		if payload.KubeEndpoint == nil {
			return nil, errors.New("missing kube_endpoint payload")
		}
		return &workloadfilter.KubeEndpoint{FilterKubeEndpoint: payload.KubeEndpoint}, nil
	default:
		return nil, errors.New("unsupported workload payload")
	}
}

// ToWorkloadFilterResult converts a WorkloadFilterResult to a workloadfilter.Result.
func ToWorkloadFilterResult(result pb.WorkloadFilterResult) workloadfilter.Result {
	switch result {
	case pb.WorkloadFilterResult_INCLUDE:
		return workloadfilter.Included
	case pb.WorkloadFilterResult_EXCLUDE:
		return workloadfilter.Excluded
	default:
		return workloadfilter.Unknown
	}
}

// FromWorkloadFilterResult converts a workloadfilter.Result to a WorkloadFilterResult.
func FromWorkloadFilterResult(res workloadfilter.Result) pb.WorkloadFilterResult {
	switch res {
	case workloadfilter.Included:
		return pb.WorkloadFilterResult_INCLUDE
	case workloadfilter.Excluded:
		return pb.WorkloadFilterResult_EXCLUDE
	default:
		return pb.WorkloadFilterResult_UNKNOWN
	}
}
