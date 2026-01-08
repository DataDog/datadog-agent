// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a gRPC server that streams Tagger entities.
package server

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	taggerStreamSendTimeout = 1 * time.Minute
	streamKeepAliveInterval = 9 * time.Minute
)

// Server is a grpc server that streams tagger entities
type Server struct {
	taggerComponent tagger.Component
	telemetry       *telemetryStore
	maxEventSize    int
	throttler       Throttler
}

// NewServer returns a new Server
func NewServer(t tagger.Component, telemetry telemetry.Component, maxEventSize int, maxParallelSync int) *Server {
	return &Server{
		taggerComponent: t,
		telemetry:       newTelemetryStore(telemetry),
		maxEventSize:    maxEventSize,
		throttler:       NewSyncThrottler(uint32(maxParallelSync)),
	}
}

// TaggerStreamEntities subscribes to added, removed, or changed entities in the Tagger
// and streams them to clients as pb.StreamTagsResponse events. Filtering is as
// of yet not implemented.
func (s *Server) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
	cardinality, err := proto.Pb2TaggerCardinality(in.GetCardinality())
	if err != nil {
		return err
	}

	ticker := time.NewTicker(streamKeepAliveInterval)
	defer ticker.Stop()

	timeoutRefreshError := make(chan error)

	go func() {
		// The remote tagger client has a timeout that closes the
		// connection after 10 minutes of inactivity (implemented in
		// comp/core/tagger/remote/tagger.go) In order to avoid closing the
		// connection and having to open it again, the server will send
		// an empty message after 9 minutes of inactivity. The goal is
		// only to keep the connection alive without losing the
		// protection against “half” closed connections brought by the
		// timeout.
		for {
			select {
			case <-out.Context().Done():
				return

			case <-ticker.C:
				err = grpc.DoWithTimeout(func() error {
					return out.Send(&pb.StreamTagsResponse{
						Events: []*pb.StreamTagsEvent{},
					})
				}, taggerStreamSendTimeout)

				if err != nil {
					log.Warnf("error sending tagger keep-alive: %s", err)
					s.telemetry.ServerStreamErrors.Inc()
					timeoutRefreshError <- err
					return
				}
			}
		}
	}()

	filterBuilder := types.NewFilterBuilder()
	for _, prefix := range in.GetPrefixes() {
		filterBuilder = filterBuilder.Include(types.EntityIDPrefix(prefix))
	}

	filter := filterBuilder.Build(cardinality)

	streamingID := in.GetStreamingID()
	if streamingID == "" {
		streamingID = uuid.New().String()
	}
	subscriptionID := "streaming-client-" + streamingID

	// initBurst is a flag indicating if the initial sync is still in progress or not
	// true means the sync hasn't yet been finalised
	// false means the streaming client has already caught up with the server
	initBurst := true
	log.Debugf("requesting token from server throttler for streaming id: %q", streamingID)
	tk := s.throttler.RequestToken()
	defer s.throttler.Release(tk)

	subscription, err := s.taggerComponent.Subscribe(subscriptionID, filter)
	log.Debugf("tagger server has just initiated subscription for %q at time %v", subscriptionID, time.Now().Unix())
	if err != nil {
		log.Errorf("Failed to subscribe to tagger for subscription %q", subscriptionID)
		return err
	}

	defer subscription.Unsubscribe()

	sendFunc := func(chunk []*pb.StreamTagsEvent) error {
		return grpc.DoWithTimeout(func() error {
			return out.Send(&pb.StreamTagsResponse{
				Events: chunk,
			})
		}, taggerStreamSendTimeout)
	}

	for {
		select {
		case events, ok := <-subscription.EventsChan():
			if !ok {
				log.Warnf("subscriber channel closed, client will reconnect")
				return errors.New("subscriber channel closed")
			}

			ticker.Reset(streamKeepAliveInterval)

			responseEvents := make([]*pb.StreamTagsEvent, 0, len(events))
			for _, event := range events {
				e, err := proto.Tagger2PbEntityEvent(event)
				if err != nil {
					log.Warnf("can't convert tagger entity to protobuf: %s", err)
					continue
				}

				responseEvents = append(responseEvents, e)
			}

			if err := processChunksInPlace(responseEvents, s.maxEventSize, computeTagsEventInBytes, sendFunc); err != nil {
				log.Warnf("error sending tagger event: %s", err)
				s.telemetry.ServerStreamErrors.Inc()
				return err
			}

			if initBurst {
				initBurst = false
				s.throttler.Release(tk)
				log.Infof("tagger server has just finished initialization for subscription %q at time %v", subscriptionID, time.Now().Unix())
			}

		case <-out.Context().Done():
			return nil

		case err = <-timeoutRefreshError:
			return err
		}
	}
}

// TaggerFetchEntity fetches an entity from the Tagger with the desired cardinality tags.
//
//nolint:revive // TODO(CINT) Fix revive linter
func (s *Server) TaggerFetchEntity(_ context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	if in.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, `missing "id" parameter`)
	}

	entityID := types.NewEntityID(types.EntityIDPrefix(in.Id.Prefix), in.Id.Uid)
	cardinality, err := proto.Pb2TaggerCardinality(in.GetCardinality())
	if err != nil {
		return nil, err
	}

	tags, err := s.taggerComponent.Tag(entityID, cardinality)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.FetchEntityResponse{
		Id:          in.Id,
		Cardinality: in.GetCardinality(),
		Tags:        tags,
	}, nil
}

// TaggerGenerateContainerIDFromOriginInfo requests the Tagger to generate a container ID from the given OriginInfo.
func (s *Server) TaggerGenerateContainerIDFromOriginInfo(_ context.Context, in *pb.GenerateContainerIDFromOriginInfoRequest) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	generatedContainerID, err := s.taggerComponent.GenerateContainerIDFromOriginInfo(origindetection.OriginInfo{
		LocalData: origindetection.LocalData{
			ProcessID:   *in.LocalData.ProcessID,
			ContainerID: *in.LocalData.ContainerID,
			Inode:       *in.LocalData.Inode,
			PodUID:      *in.LocalData.PodUID,
		},
		ExternalData: origindetection.ExternalData{
			Init:          *in.ExternalData.Init,
			ContainerName: *in.ExternalData.ContainerName,
			PodUID:        *in.ExternalData.PodUID,
		},
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.GenerateContainerIDFromOriginInfoResponse{
		ContainerID: generatedContainerID,
	}, nil
}
