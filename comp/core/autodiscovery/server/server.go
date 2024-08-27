// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewServer returns a new server with a workloadmeta instance
func NewServer(ac autodiscovery.Component) *Server {
	return &Server{
		ac: ac,
	}
}

// Server is a grpc server that streams workloadmeta entities
type Server struct {
	ac autodiscovery.Component
}

// StreamEntities streams entities from the workloadmeta store applying the given filter
func (s *Server) StreamConfig(out pb.AgentSecure_AutodiscoveryStreamConfigServer) error {
	scheduleChannel, unscheduleChannel := s.ac.GetServerChannels()

	for {
		select {
		case config, ok := <-scheduleChannel:
			if !ok {
				return nil
			}

			protobufConfig := protobufConfigFromAutodiscoveryConfig(config)
			protobufConfig.EventType = pb.ConfigEventType_SCHEDULE

			err := grpc.DoWithTimeout(func() error {
				return out.Send(&pb.AutodiscoveryStreamResponse{
					Configs: []*pb.Config{protobufConfig},
				})
			}, 1*time.Minute)

			if err != nil {
				log.Warnf("error sending schedule autodiscovbery event: %s", err)
				return err
			}
		case config, ok := <-unscheduleChannel:
			if !ok {
				return nil
			}

			protobufConfig := protobufConfigFromAutodiscoveryConfig(config)
			protobufConfig.EventType = pb.ConfigEventType_UNSCHEDULE

			err := grpc.DoWithTimeout(func() error {
				return out.Send(&pb.AutodiscoveryStreamResponse{
					Configs: []*pb.Config{protobufConfig},
				})
			}, 1*time.Minute)

			if err != nil {
				log.Warnf("error sending unschedule autodiscovbery event: %s", err)
				return err
			}

		case <-out.Context().Done():
			return nil
		}
	}
}
