// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Code generated manually to match executor.proto. DO NOT EDIT without updating executor.proto.

package executor

import gogo "github.com/gogo/protobuf/proto"

// StatusRequest requests executor readiness.
type StatusRequest struct {
}

func (m *StatusRequest) Reset()         { *m = StatusRequest{} }
func (m *StatusRequest) String() string { return gogo.CompactTextString(m) }
func (*StatusRequest) ProtoMessage()    {}

// StatusResponse reports executor readiness and currently active tasks.
type StatusResponse struct {
	ProtocolVersion uint32 `protobuf:"varint,1,opt,name=protocol_version,json=protocolVersion,proto3" json:"protocol_version,omitempty"`
	Ready           bool   `protobuf:"varint,2,opt,name=ready,proto3" json:"ready,omitempty"`
	ActiveTasks     int32  `protobuf:"varint,3,opt,name=active_tasks,json=activeTasks,proto3" json:"active_tasks,omitempty"`
	Version         string `protobuf:"bytes,4,opt,name=version,proto3" json:"version,omitempty"`
}

func (m *StatusResponse) Reset()         { *m = StatusResponse{} }
func (m *StatusResponse) String() string { return gogo.CompactTextString(m) }
func (*StatusResponse) ProtoMessage()    {}

// SubmitTaskRequest sends a raw OPMS task JSON payload to the executor.
type SubmitTaskRequest struct {
	TaskJson []byte `protobuf:"bytes,1,opt,name=task_json,json=taskJson,proto3" json:"task_json,omitempty"`
}

func (m *SubmitTaskRequest) Reset()         { *m = SubmitTaskRequest{} }
func (m *SubmitTaskRequest) String() string { return gogo.CompactTextString(m) }
func (*SubmitTaskRequest) ProtoMessage()    {}

// SubmitTaskResponse reports whether the executor accepted task ownership.
type SubmitTaskResponse struct {
	Accepted bool   `protobuf:"varint,1,opt,name=accepted,proto3" json:"accepted,omitempty"`
	Reason   string `protobuf:"bytes,2,opt,name=reason,proto3" json:"reason,omitempty"`
}

func (m *SubmitTaskResponse) Reset()         { *m = SubmitTaskResponse{} }
func (m *SubmitTaskResponse) String() string { return gogo.CompactTextString(m) }
func (*SubmitTaskResponse) ProtoMessage()    {}

func init() {
	gogo.RegisterType((*StatusRequest)(nil), "datadog.privateactionrunner.executor.StatusRequest")
	gogo.RegisterType((*StatusResponse)(nil), "datadog.privateactionrunner.executor.StatusResponse")
	gogo.RegisterType((*SubmitTaskRequest)(nil), "datadog.privateactionrunner.executor.SubmitTaskRequest")
	gogo.RegisterType((*SubmitTaskResponse)(nil), "datadog.privateactionrunner.executor.SubmitTaskResponse")
}
