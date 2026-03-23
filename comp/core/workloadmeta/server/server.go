// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a gRPC server that streams the entities stored in
// Workloadmeta.
package server

import (
	"context"
	"fmt"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/proto"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/telemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	streamSendTimeout             = 1 * time.Minute
	workloadmetaKeepAliveInterval = 9 * time.Minute
	sendQueueSize                 = 50
	sendQueueTimeout              = 30 * time.Second
)

// NewServer returns a new server with a workloadmeta instance
func NewServer(store workloadmeta.Component) *Server {
	return &Server{
		wmeta:             store,
		streamSendTimeout: streamSendTimeout,
		sendQueueTimeout:  sendQueueTimeout,
	}
}

// Server is a grpc server that streams workloadmeta entities
type Server struct {
	wmeta             workloadmeta.Component
	streamSendTimeout time.Duration
	sendQueueTimeout  time.Duration
	sendQueueSize     int
}

// StreamEntities streams entities from the workloadmeta store applying the given filter
func (s *Server) StreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	filter, err := proto.WorkloadmetaFilterFromProtoFilter(in.GetFilter())
	if err != nil {
		return err
	}

	workloadmetaEventsChannel := s.wmeta.Subscribe("stream-client", workloadmeta.NormalPriority, filter)
	defer s.wmeta.Unsubscribe(workloadmetaEventsChannel)

	ctx, cancel := context.WithCancel(out.Context())
	defer cancel()

	queueSize := sendQueueSize
	if s.sendQueueSize > 0 {
		queueSize = s.sendQueueSize
	}
	sendQueue := make(chan []*pb.WorkloadmetaEvent, queueSize)

	// Receiver goroutine: drains the workloadmeta subscription channel quickly
	// (to avoid blocking other workloadmeta subscribers) and enqueues protobuf
	// events into the send queue.
	workloadmetaReceiverErrCh := make(chan error, 1)
	go func() {
		defer close(sendQueue)
		recvErr := s.receiveEvents(ctx, workloadmetaEventsChannel, sendQueue)
		if recvErr != nil {
			cancel() // stop sendEvents to unsubscribe promptly
		}
		workloadmetaReceiverErrCh <- recvErr
	}()

	sendErr := s.sendEvents(ctx, out, sendQueue)
	cancel() // sending finished; stop the workloadmeta receiver

	// Wait for the workloadmeta receiver to finish
	recvErr := <-workloadmetaReceiverErrCh

	if sendErr != nil {
		return sendErr
	}
	return recvErr
}

// receiveEvents drains events from the workloadmeta subscription, converts them
// to protobuf, and enqueues them into sendQueue.
func (s *Server) receiveEvents(ctx context.Context, eventsCh chan workloadmeta.EventBundle, sendQueue chan<- []*pb.WorkloadmetaEvent) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case eventBundle, ok := <-eventsCh:
			if !ok {
				return nil
			}
			eventBundle.Acknowledge()

			protobufEvents := make([]*pb.WorkloadmetaEvent, 0, len(eventBundle.Events))

			for _, event := range eventBundle.Events {
				protobufEvent, err := proto.ProtobufEventFromWorkloadmetaEvent(event)

				if err != nil {
					log.Errorf("error converting workloadmeta event to protobuf: %s", err)
					continue
				}

				if protobufEvent != nil {
					protobufEvents = append(protobufEvents, protobufEvent)
				}
			}

			if len(protobufEvents) == 0 {
				continue
			}

			select {
			case sendQueue <- protobufEvents:
			case <-time.After(s.sendQueueTimeout):
				return fmt.Errorf("send queue full for %s, disconnecting client", s.sendQueueTimeout)
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// sendEvents reads protobuf responses from sendQueue and sends them over gRPC.
// It also sends periodic keep-alive messages.
func (s *Server) sendEvents(ctx context.Context, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer, sendQueue <-chan []*pb.WorkloadmetaEvent) error {
	ticker := time.NewTicker(workloadmetaKeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case events, ok := <-sendQueue:
			if !ok {
				return nil
			}

			err := grpc.DoWithTimeout(func() error {
				return out.Send(&pb.WorkloadmetaStreamResponse{
					Events: events,
				})
			}, s.streamSendTimeout)

			if err != nil {
				log.Warnf("error sending workloadmeta event: %s", err)
				telemetry.RemoteServerErrors.Inc()
				return err
			}

			ticker.Reset(workloadmetaKeepAliveInterval)

		case <-ctx.Done():
			return nil

		// The remote workloadmeta client has a timeout that closes the
		// connection after some minutes of inactivity. In order to avoid
		// closing the connection and having to open it again, the server will
		// send an empty message from time to time as defined in the ticker. The
		// goal is only to keep the connection alive without losing the
		// protection against “half” closed connections brought by the timeout.
		case <-ticker.C:
			err := grpc.DoWithTimeout(func() error {
				return out.Send(&pb.WorkloadmetaStreamResponse{
					Events: []*pb.WorkloadmetaEvent{},
				})
			}, s.streamSendTimeout)

			if err != nil {
				log.Warnf("error sending workloadmeta keep-alive: %s", err)
				telemetry.RemoteServerErrors.Inc()
				return err
			}
		}
	}
}
