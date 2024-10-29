// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a gRPC server that handles querying remote agents
// for their status and flare details.
package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	remoteagent "github.com/DataDog/datadog-agent/comp/remoteagent/def"
	proto "github.com/DataDog/datadog-agent/comp/remoteagent/proto"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	remoteAgentKeepAliveInterval = 5 * time.Minute
	remoteAgentStreamSendTimeout = 10 * time.Second
)

// NewServer returns a new server with a remoteagent instance
func NewServer(comp remoteagent.Component) *Server {
	return &Server{
		comp: comp,
	}
}

// Server is a gRPC server that handles status/flare requests and responses between
// this agent and remote agents
type Server struct {
	comp remoteagent.Component
}

// UpdateRemoteAgent handles registration of remote agents and querying them for status/flare data
func (s *Server) UpdateRemoteAgent(srv pb.AgentSecure_UpdateRemoteAgentServer) error {
	log.Info("Serving new remote agent connection.")

	// We expect the remote agent to register itself with an initial streaming request,
	// and we can't do anything else until that happens.
	registrationData, err := waitForRegister(srv, srv.Context())
	if err != nil {
		return err
	}

	agentId := registrationData.AgentId
	statusRequestsIn, flareRequestsIn, err := s.comp.RegisterRemoteAgent(agentId)
	if err != nil {
		return fmt.Errorf("error registering remote agent: %s", err)
	}

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
		case request, ok := <-flareRequestsIn:
			log.Tracef("Received request to collect flare.")

			if !ok {
				return fmt.Errorf("remote agent server closed unexpectedly")
			}

			// Send a flare request to the remote agent, and then wait for a response.
			err := sendMessage(srv, request.Context(), buildFlareRequestResponse(agentId))
			if err != nil {
				log.Warnf("Error sending flare request to remote agent '%s': %v", agentId, err)
				return err
			}

			log.Tracef("Sent flare request to remote agent. Waiting for flare data...")

			ticker.Reset(remoteAgentKeepAliveInterval)

			flareData, err := waitForFlareData(srv, request.Context())
			if err != nil {
				log.Warnf("Error receiving flare data from remote agent '%s': %v", agentId, err)
				return err
			}

			request.Fulfill(flareData)

			log.Trace("Forwarded response back to caller.")
		case request, ok := <-statusRequestsIn:
			log.Tracef("Received request to collect status.")

			if !ok {
				return fmt.Errorf("remote agent server closed unexpectedly")
			}

			// Send a status request to the remote agent, and then wait for a response.
			err := sendMessage(srv, request.Context(), buildStatusRequestResponse(agentId))
			if err != nil {
				log.Warnf("Error sending status request to remote agent '%s': %v", agentId, err)
				return err
			}

			log.Tracef("Sent status request to remote agent. Waiting for status data...")

			ticker.Reset(remoteAgentKeepAliveInterval)

			statusData, err := waitForStatusData(srv, request.Context())
			if err != nil {
				log.Warnf("Error receiving status data from remote agent '%s': %v", agentId, err)
				return err
			}

			request.Fulfill(statusData)

			log.Trace("Forwarded response back to caller.")
		case <-srv.Context().Done():
			log.Debugf("Remote agent '%s' disconnected.", agentId)
			return nil
		case <-ticker.C:
			log.Debugf("Received keep-alive ticket. Sending ping...")

			// Keep-alive timer fired, so send a keep-alive response to keep the connection alive.
			context, cancel := context.WithTimeout(context.Background(), remoteAgentStreamSendTimeout)
			defer cancel()

			err := sendMessage(srv, context, buildKeepAliveResponse())
			if err != nil {
				log.Warnf("Error sending keep-alive request to remote agent '%s': %v", agentId, err)
				return err
			}
		}
	}
}

func waitForRegister(srv pb.AgentSecure_UpdateRemoteAgentServer, context context.Context) (*pb.RegistrationData, error) {
	request, err := receiveWithTimeout(srv, context)
	if err != nil {
		return nil, err
	}

	switch x := request.Payload.(type) {
	case *pb.UpdateRemoteAgentStreamRequest_Register:
		log.Tracef("Received register response: %v", x.Register)
		return x.Register, nil
	default:
		return nil, fmt.Errorf("expected register request, got: %v", x)
	}
}

func waitForFlareData(srv pb.AgentSecure_UpdateRemoteAgentServer, context context.Context) (*remoteagent.FlareData, error) {
	request, err := receiveWithTimeout(srv, context)
	if err != nil {
		return nil, err
	}

	switch payload := request.Payload.(type) {
	case *pb.UpdateRemoteAgentStreamRequest_Flare:
		log.Tracef("Received flare response: %v", payload.Flare)
		return proto.ProtobufToFlareData(payload.Flare), nil
	default:
		return nil, fmt.Errorf("expected flare data, got: %v", payload)
	}
}

func waitForStatusData(srv pb.AgentSecure_UpdateRemoteAgentServer, context context.Context) (*remoteagent.StatusData, error) {
	request, err := receiveWithTimeout(srv, context)
	if err != nil {
		return nil, err
	}

	switch payload := request.Payload.(type) {
	case *pb.UpdateRemoteAgentStreamRequest_Status:
		log.Tracef("Received status response: %v", payload.Status)
		return proto.ProtobufToStatusData(payload.Status), nil
	default:
		return nil, fmt.Errorf("expected status data, got: %v", payload)
	}
}

func sendMessage(srv pb.AgentSecure_UpdateRemoteAgentServer, context context.Context, msg *pb.UpdateRemoteAgentStreamResponse) error {
	errChan := make(chan error, 1)

	go func() {
		errChan <- srv.Send(msg)
		close(errChan)
	}()

	select {
	case err := <-errChan:
		return err
	case <-context.Done():
		return status.Errorf(codes.DeadlineExceeded, "timed out waiting for operation")
	}
}

func receiveWithTimeout(srv pb.AgentSecure_UpdateRemoteAgentServer, context context.Context) (*pb.UpdateRemoteAgentStreamRequest, error) {
	msgChan := make(chan *pb.UpdateRemoteAgentStreamRequest, 1)
	errChan := make(chan error, 1)

	go func() {
		msg, err := srv.Recv()
		if err != nil {
			errChan <- err
			close(errChan)
			return
		}

		msgChan <- msg
		close(msgChan)
	}()

	select {
	case msg := <-msgChan:
		return msg, nil
	case err := <-errChan:
		return nil, err
	case <-context.Done():
		return nil, status.Errorf(codes.DeadlineExceeded, "timed out waiting for operation")
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
