// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"io"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// RemoteEventServer implements the SecurityAgentAPI gRPC service in system-probe,
// allowing remote system-probes (e.g., running in micro VMs) to connect via vsock
// and forward their security events to the host system-probe for backend delivery.
type RemoteEventServer struct {
	api.UnimplementedSecurityAgentAPIServer

	msgSender          EventMsgSender
	activityDumpSender ActivityDumpMsgSender
	expireEvent        func(*api.SecurityEventMessage)
	expireDump         func(*api.ActivityDumpStreamMessage)
}

// NewRemoteEventServer returns a RemoteEventServer wired to the given APIServer's senders.
func NewRemoteEventServer(apiServer *APIServer) *RemoteEventServer {
	return &RemoteEventServer{
		msgSender:          apiServer.msgSender,
		activityDumpSender: apiServer.activityDumpSender,
		expireEvent:        apiServer.expireEvent,
		expireDump:         apiServer.expireDump,
	}
}

// SendEvent receives security events from a remote system-probe and forwards them through the local pipeline.
func (s *RemoteEventServer) SendEvent(stream grpc.ClientStreamingServer[api.SecurityEventMessage, empty.Empty]) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		seclog.Tracef("received remote event from rule `%s`", msg.RuleID)
		s.msgSender.Send(msg, s.expireEvent)
	}
	return nil
}

// SendActivityDumpStream receives activity dumps from a remote system-probe and forwards them through the local pipeline.
func (s *RemoteEventServer) SendActivityDumpStream(stream grpc.ClientStreamingServer[api.ActivityDumpStreamMessage, empty.Empty]) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		seclog.Tracef("received remote activity dump [%s]", msg.GetSelector())
		s.activityDumpSender.Send(msg, s.expireDump)
	}
	return nil
}
