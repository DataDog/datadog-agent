// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package server implements the transport-specific logic for the configstream component.
package server

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configstream "github.com/DataDog/datadog-agent/comp/core/configstream/def"
)

// Server implements the transport-specific logic for the configstream component.
type Server struct {
	cfg  config.Component
	comp configstream.Component
}

// NewServer creates a new Server.
func NewServer(cfg config.Component, comp configstream.Component) *Server {
	return &Server{cfg: cfg, comp: comp}
}

// StreamConfigEvents handles the gRPC streaming logic.
func (s *Server) StreamConfigEvents(req *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	eventsCh, unsubscribe := s.comp.Subscribe(req)
	defer unsubscribe()

	interval := s.cfg.GetDuration("config_stream.sleep_interval")

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

				if s, ok := status.FromError(err); ok {
					switch s.Code() {
					// Terminal errors, the client must re-establish the stream
					case codes.Canceled, codes.Unavailable, codes.ResourceExhausted:
						log.Infof("Closing config stream for client due to terminal error: %v", s.Code())
						return err
					}
				}
				// For other errors, we drop the event and continue
				time.Sleep(interval)
			}
		}
	}
}
