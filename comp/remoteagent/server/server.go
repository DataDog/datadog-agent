// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a gRPC server that handles querying remote agents
// for their status and flare details.
package server

import (
	"fmt"
	"time"

	remoteagent "github.com/DataDog/datadog-agent/comp/remoteagent/def"
	proto "github.com/DataDog/datadog-agent/comp/remoteagent/proto"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	remoteAgentKeepAliveInterval = 9 * time.Minute
	remoteAgentStreamSendTimeout = 1 * time.Minute
)

// NewServer returns a new server with a remoteagent instance
func NewServer(comp remoteagent.Component) *Server {
	return &Server{
		comp: comp,
	}
}

// Server is a grpc server that handles status/flare requests and responses between
// this agent and remote agents
type Server struct {
	comp remoteagent.Component
}

// UpdateRemoteAgent handles registration of remote agents and querying them for status/flare data
func (s *Server) UpdateRemoteAgent(srv pb.AgentSecure_UpdateRemoteAgentServer) error {
	log.Info("Serving new remote agent connection.")

	// We expect the remote agent to register itself with an initial streaming request,
	// and we can't do anything else until that happens.
	registrationData, err := waitForRegister(srv)
	if err != nil {
		return err
	}

	agentId := registrationData.AgentId
	statusRequestsIn, flareRequestsIn, err := s.comp.RegisterRemoteAgent(agentId)
	if err != nil {
		return fmt.Errorf("error registering remote agent: %s", err)
	}

	log.Infof("Remote agent registered with ID: %s", agentId)

	err = runRemoteAgentLoop(agentId, srv, statusRequestsIn, flareRequestsIn)
	s.comp.DeregisterRemoteAgent(agentId)

	return err
}

func runRemoteAgentLoop(agentId string, srv pb.AgentSecure_UpdateRemoteAgentServer, statusRequestsIn remoteagent.StatusRequests, flareRequestsIn remoteagent.FlareRequests) error {
	// Start a keep-alive timer to ensure the remote agent stays connected.
	//
	// We reset this timer any time we send a request to the remote agent since that request will perform the same
	// operation as a keep-alive response.
	ticker := time.NewTicker(remoteAgentKeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case flareRequest, ok := <-flareRequestsIn:
			if !ok {
				return fmt.Errorf("remote agent server closed unexpectedly")
			}

			// Send a flare request to the remote agent, and then wait for a response.
			err := sendResponseWithTimeout(srv, buildFlareRequestResponse(agentId))
			if err != nil {
				return err
			}

			ticker.Reset(remoteAgentKeepAliveInterval)

			flareData, err := waitForFlareResponse(srv)
			if err != nil {
				return err
			}

			flareRequest <- flareData
		case statusRequest, ok := <-statusRequestsIn:
			if !ok {
				return fmt.Errorf("remote agent server closed unexpectedly")
			}

			// Send a status request to the remote agent, and then wait for a response.
			err := sendResponseWithTimeout(srv, buildStatusRequestResponse(agentId))
			if err != nil {
				return err
			}

			ticker.Reset(remoteAgentKeepAliveInterval)

			statusData, err := waitForStatusResponse(srv)
			if err != nil {
				return err
			}

			statusRequest <- statusData
		case <-srv.Context().Done():
			log.Infof("Remote agent %s disconnected.", agentId)
			return nil
		case <-ticker.C:
			// Keep-alive timer fired, so send a keep-alive response to keep the connection alive.
			err := sendResponseWithTimeout(srv, buildKeepAliveResponse())
			if err != nil {
				return err
			}
		}
	}
}

func sendResponseWithTimeout(srv pb.AgentSecure_UpdateRemoteAgentServer, response *pb.UpdateRemoteAgentStreamResponse) error {
	err := grpc.DoWithTimeout(func() error {
		return srv.Send(response)
	}, remoteAgentStreamSendTimeout)

	if err != nil {
		log.Warnf("error sending remoteagent event: %s", err)
		return err
	}

	return nil
}

func waitForRegister(srv pb.AgentSecure_UpdateRemoteAgentServer) (*pb.RegistrationData, error) {
	request, err := srv.Recv()
	if err != nil {
		return nil, err
	}

	switch x := request.Payload.(type) {
	case *pb.UpdateRemoteAgentStreamRequest_Register:
		return x.Register, nil
	default:
		return nil, fmt.Errorf("expected register request: %v", x)
	}
}

func waitForFlareResponse(srv pb.AgentSecure_UpdateRemoteAgentServer) (*remoteagent.FlareData, error) {
	request, err := srv.Recv()
	if err != nil {
		return nil, err
	}

	switch x := request.Payload.(type) {
	case *pb.UpdateRemoteAgentStreamRequest_Flare:
		return proto.ProtobufToFlareData(x.Flare), nil
	default:
		return nil, fmt.Errorf("expected flare request: %v", x)
	}
}

func waitForStatusResponse(srv pb.AgentSecure_UpdateRemoteAgentServer) (*remoteagent.StatusData, error) {
	request, err := srv.Recv()
	if err != nil {
		return nil, err
	}

	switch x := request.Payload.(type) {
	case *pb.UpdateRemoteAgentStreamRequest_Status:
		return proto.ProtobufToStatusData(x.Status), nil
	default:
		return nil, fmt.Errorf("expected status request: %v", x)
	}
}

func buildKeepAliveResponse() *pb.UpdateRemoteAgentStreamResponse {
	return &pb.UpdateRemoteAgentStreamResponse{
		Payload: &pb.UpdateRemoteAgentStreamResponse_KeepAlive{
			KeepAlive: "PING",
		},
	}
}

func buildFlareRequestResponse(agentId string) *pb.UpdateRemoteAgentStreamResponse {
	return &pb.UpdateRemoteAgentStreamResponse{
		Payload: &pb.UpdateRemoteAgentStreamResponse_Flare{
			Flare: &pb.FlareRequest{
				AgentId: agentId,
			},
		},
	}
}

func buildStatusRequestResponse(agentId string) *pb.UpdateRemoteAgentStreamResponse {
	return &pb.UpdateRemoteAgentStreamResponse{
		Payload: &pb.UpdateRemoteAgentStreamResponse_Status{
			Status: &pb.StatusRequest{
				AgentId: agentId,
			},
		},
	}
}
