// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/proto/utils"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	workloadmetaStreamSendTimeout = 1 * time.Minute
)

func NewServer(store workloadmeta.Store) *Server {
	return &Server{
		store: store,
	}
}

type Server struct {
	store workloadmeta.Store
}

// StreamEntities streams entities from the workloadmeta store applying the given filter
func (s *Server) StreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	filter, err := utils.WorkloadmetaFilterFromProtoFilter(in.GetFilter())
	if err != nil {
		return err
	}

	workloadmetaEventsChannel := s.store.Subscribe("stream-client", workloadmeta.NormalPriority, filter)
	defer s.store.Unsubscribe(workloadmetaEventsChannel)

	// Note: In the tagger stream function, when there are no events for a few
	// minutes, we send an empty list to keep the connection alive. We might
	// need to do the same here depending on the implementation of the remote
	// workloadmeta.

	for {
		select {
		case eventBundle := <-workloadmetaEventsChannel:
			close(eventBundle.Ch)

			var protobufEvents []*pb.WorkloadmetaEvent

			for _, event := range eventBundle.Events {
				protobufEvent, err := utils.ProtobufEventFromWorkloadmetaEvent(event)

				if err != nil {
					log.Errorf("error converting workloadmeta event to protobuf: %s", err)
					continue
				}

				if protobufEvent != nil {
					protobufEvents = append(protobufEvents, protobufEvent)
				}
			}

			if len(protobufEvents) > 0 {
				err := grpc.DoWithTimeout(func() error {
					return out.Send(&pb.WorkloadmetaStreamResponse{
						Events: protobufEvents,
					})
				}, workloadmetaStreamSendTimeout)

				if err != nil {
					log.Warnf("error sending workloadmeta event: %s", err)
					return err
				}
			}
		case <-out.Context().Done():
			return nil
		}
	}
}
