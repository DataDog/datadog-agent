// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package proto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestNewEvaluateRequest(t *testing.T) {
	tests := []struct {
		name        string
		programName string
		entity      workloadfilter.Filterable
		wantErr     bool
		validate    func(t *testing.T, req *pb.WorkloadFilterEvaluateRequest)
	}{
		{
			name:        "Container",
			programName: "test-prog",
			entity: &workloadfilter.Container{
				FilterContainer: &pb.FilterContainer{
					Id:   "c1",
					Name: "container1",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, req *pb.WorkloadFilterEvaluateRequest) {
				assert.Equal(t, "test-prog", req.ProgramName)
				container := req.GetContainer()
				require.NotNil(t, container)
				assert.Equal(t, "c1", container.Id)
				assert.Equal(t, "container1", container.Name)
			},
		},
		{
			name:        "Pod",
			programName: "test-prog",
			entity: &workloadfilter.Pod{
				FilterPod: &pb.FilterPod{
					Id:        "p1",
					Name:      "pod1",
					Namespace: "default",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, req *pb.WorkloadFilterEvaluateRequest) {
				assert.Equal(t, "test-prog", req.ProgramName)
				pod := req.GetPod()
				require.NotNil(t, pod)
				assert.Equal(t, "p1", pod.Id)
				assert.Equal(t, "pod1", pod.Name)
			},
		},
		{
			name:        "Process",
			programName: "test-prog",
			entity: &workloadfilter.Process{
				FilterProcess: &pb.FilterProcess{
					Name:    "proc1",
					Cmdline: "cmd",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, req *pb.WorkloadFilterEvaluateRequest) {
				assert.Equal(t, "test-prog", req.ProgramName)
				proc := req.GetProcess()
				require.NotNil(t, proc)
				assert.Equal(t, "proc1", proc.Name)
			},
		},
		{
			name:        "Service",
			programName: "test-prog",
			entity: &workloadfilter.KubeService{
				FilterKubeService: &pb.FilterKubeService{
					Name:      "svc1",
					Namespace: "ns1",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, req *pb.WorkloadFilterEvaluateRequest) {
				assert.Equal(t, "test-prog", req.ProgramName)
				svc := req.GetKubeService()
				require.NotNil(t, svc)
				assert.Equal(t, "svc1", svc.Name)
			},
		},
		{
			name:        "Endpoint",
			programName: "test-prog",
			entity: &workloadfilter.KubeEndpoint{
				FilterKubeEndpoint: &pb.FilterKubeEndpoint{
					Name:      "ep1",
					Namespace: "ns1",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, req *pb.WorkloadFilterEvaluateRequest) {
				assert.Equal(t, "test-prog", req.ProgramName)
				ep := req.GetKubeEndpoint()
				require.NotNil(t, ep)
				assert.Equal(t, "ep1", ep.Name)
			},
		},
		{
			name:        "UnsupportedType",
			programName: "test-prog",
			entity:      &mock.Filterable{EntityType: workloadfilter.ResourceType("mock")}, // Unknown type
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewEvaluateRequest(tt.programName, tt.entity)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, req)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, req)
				if tt.validate != nil {
					tt.validate(t, req)
				}
			}
		})
	}
}

func TestExtractFilterable(t *testing.T) {
	tests := []struct {
		name     string
		req      *pb.WorkloadFilterEvaluateRequest
		wantErr  bool
		validate func(t *testing.T, f workloadfilter.Filterable)
	}{
		{
			name: "Container",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_Container{
					Container: &pb.FilterContainer{
						Id:   "c1",
						Name: "container1",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, f workloadfilter.Filterable) {
				c, ok := f.(*workloadfilter.Container)
				require.True(t, ok)
				assert.Equal(t, "c1", c.Id)
			},
		},
		{
			name: "Pod",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_Pod{
					Pod: &pb.FilterPod{
						Id: "p1",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, f workloadfilter.Filterable) {
				p, ok := f.(*workloadfilter.Pod)
				require.True(t, ok)
				assert.Equal(t, "p1", p.Id)
			},
		},
		{
			name: "Process",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_Process{
					Process: &pb.FilterProcess{
						Name: "proc1",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, f workloadfilter.Filterable) {
				p, ok := f.(*workloadfilter.Process)
				require.True(t, ok)
				assert.Equal(t, "proc1", p.Name)
			},
		},
		{
			name: "Service",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_KubeService{
					KubeService: &pb.FilterKubeService{
						Name: "svc1",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, f workloadfilter.Filterable) {
				s, ok := f.(*workloadfilter.KubeService)
				require.True(t, ok)
				assert.Equal(t, "svc1", s.Name)
			},
		},
		{
			name: "Endpoint",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_KubeEndpoint{
					KubeEndpoint: &pb.FilterKubeEndpoint{
						Name: "ep1",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, f workloadfilter.Filterable) {
				e, ok := f.(*workloadfilter.KubeEndpoint)
				require.True(t, ok)
				assert.Equal(t, "ep1", e.Name)
			},
		},
		{
			name: "MissingContainerPayload",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_Container{
					Container: nil,
				},
			},
			wantErr: true,
		},
		{
			name: "MissingPodPayload",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: &pb.WorkloadFilterEvaluateRequest_Pod{
					Pod: nil,
				},
			},
			wantErr: true,
		},
		{
			name: "EmptyWorkload",
			req: &pb.WorkloadFilterEvaluateRequest{
				Workload: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ExtractFilterable(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, f)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, f)
				if tt.validate != nil {
					tt.validate(t, f)
				}
			}
		})
	}
}

func TestResultConversion(t *testing.T) {
	tests := []struct {
		name  string
		pbRes pb.WorkloadFilterResult
		wfRes workloadfilter.Result
	}{
		{
			name:  "Include",
			pbRes: pb.WorkloadFilterResult_INCLUDE,
			wfRes: workloadfilter.Included,
		},
		{
			name:  "Exclude",
			pbRes: pb.WorkloadFilterResult_EXCLUDE,
			wfRes: workloadfilter.Excluded,
		},
		{
			name:  "Unknown",
			pbRes: pb.WorkloadFilterResult_UNKNOWN,
			wfRes: workloadfilter.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ToWorkloadFilterResult
			gotWf := ToWorkloadFilterResult(tt.pbRes)
			assert.Equal(t, tt.wfRes, gotWf)

			// Test FromWorkloadFilterResult
			gotPb := FromWorkloadFilterResult(tt.wfRes)
			assert.Equal(t, tt.pbRes, gotPb)
		})
	}
}
