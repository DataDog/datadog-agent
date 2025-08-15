// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package server implements the transport-specific logic for the configstream component.
package server

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	configstream "github.com/DataDog/datadog-agent/comp/core/configstream/def"
)

// Server implements the transport-specific logic for the configstream component.
type Server struct {
	comp configstream.Component
}

// NewServer creates a new Server.
func NewServer(comp configstream.Component) *Server {
	return &Server{comp: comp}
}

// StreamConfigEvents handles the gRPC streaming logic.
func (s *Server) StreamConfigEvents(req *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	eventsCh, unsubscribe := s.comp.Subscribe(req)
	defer unsubscribe()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event, ok := <-eventsCh:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				log.Warnf("Failed to send config event to client: %v", err)
				return err
			}
		}
	}
}
