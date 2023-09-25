// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	protoutils "github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

const (
	workloadmetaStreamSendTimeout = 1 * time.Minute
	workloadmetaKeepAliveInterval = 9 * time.Minute
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
	filter, err := protoutils.WorkloadmetaFilterFromProtoFilter(in.GetFilter())
	if err != nil {
		return err
	}

	workloadmetaEventsChannel := s.store.Subscribe("stream-client", workloadmeta.NormalPriority, filter)
	defer s.store.Unsubscribe(workloadmetaEventsChannel)

	ticker := time.NewTicker(workloadmetaKeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case eventBundle := <-workloadmetaEventsChannel:
			close(eventBundle.Ch)

			var protobufEvents []*pb.WorkloadmetaEvent

			for _, event := range eventBundle.Events {
				protobufEvent, err := protoutils.ProtobufEventFromWorkloadmetaEvent(event)

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
					telemetry.RemoteServerErrors.Inc()
					return err
				}

				ticker.Reset(workloadmetaKeepAliveInterval)
			}
		case <-out.Context().Done():
			return nil

		// The remote workloadmeta client has a timeout that closes the
		// connection after some minutes of inactivity. In order to avoid
		// closing the connection and having to open it again, the server will
		// send an empty message from time to time as defined in the ticker. The
		// goal is only to keep the connection alive without losing the
		// protection against “half” closed connections brought by the timeout.
		case <-ticker.C:
			err = grpc.DoWithTimeout(func() error {
				return out.Send(&pb.WorkloadmetaStreamResponse{
					Events: []*pb.WorkloadmetaEvent{},
				})
			}, workloadmetaStreamSendTimeout)

			if err != nil {
				log.Warnf("error sending workloadmeta keep-alive: %s", err)
				telemetry.RemoteServerErrors.Inc()
				return err
			}
		}
	}
}
