// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pbutils "github.com/DataDog/datadog-agent/pkg/proto/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	taggerStreamSendTimeout = 1 * time.Minute
	streamKeepAliveInterval = 9 * time.Minute
)

type Server struct {
	tagger tagger.Tagger
}

func NewServer(t tagger.Tagger) *Server {
	return &Server{
		tagger: t,
	}
}

// TaggerStreamEntities subscribes to added, removed, or changed entities in the Tagger
// and streams them to clients as pb.StreamTagsResponse events. Filtering is as
// of yet not implemented.
func (s *Server) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
	cardinality, err := pbutils.Pb2TaggerCardinality(in.Cardinality)
	if err != nil {
		return err
	}

	// NOTE: StreamTagsRequest can specify filters, but they cannot be
	// implemented since the tagger has no concept of container metadata.
	// these filters will be introduced when we implement a container
	// metadata service that can receive them as is from the tagger.

	t := tagger.GetDefaultTagger()
	eventCh := t.Subscribe(cardinality)
	defer t.Unsubscribe(eventCh)

	ticker := time.NewTicker(streamKeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case events := <-eventCh:
			ticker.Reset(streamKeepAliveInterval)

			responseEvents := make([]*pb.StreamTagsEvent, 0, len(events))
			for _, event := range events {
				e, err := pbutils.Tagger2PbEntityEvent(event)
				if err != nil {
					log.Warnf("can't convert tagger entity to protobuf: %s", err)
					continue
				}

				responseEvents = append(responseEvents, e)
			}

			err = grpc.DoWithTimeout(func() error {
				return out.Send(&pb.StreamTagsResponse{
					Events: responseEvents,
				})
			}, taggerStreamSendTimeout)

			if err != nil {
				log.Warnf("error sending tagger event: %s", err)
				telemetry.ServerStreamErrors.Inc()
				return err
			}

		case <-out.Context().Done():
			return nil

		// The remote tagger client has a timeout that closes the
		// connection after 10 minutes of inactivity (implemented in
		// pkg/tagger/remote/tagger.go) In order to avoid closing the
		// connection and having to open it again, the server will send
		// an empty message after 9 minutes of inactivity. The goal is
		// only to keep the connection alive without losing the
		// protection against “half” closed connections brought by the
		// timeout.
		case <-ticker.C:
			err = grpc.DoWithTimeout(func() error {
				return out.Send(&pb.StreamTagsResponse{
					Events: []*pb.StreamTagsEvent{},
				})
			}, taggerStreamSendTimeout)

			if err != nil {
				log.Warnf("error sending tagger keep-alive: %s", err)
				telemetry.ServerStreamErrors.Inc()
				return err
			}
		}
	}
}

// TaggerFetchEntity fetches an entity from the Tagger with the desired cardinality tags.
func (s *Server) TaggerFetchEntity(ctx context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	if in.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, `missing "id" parameter`)
	}

	entityID := fmt.Sprintf("%s://%s", in.Id.Prefix, in.Id.Uid)
	cardinality, err := pbutils.Pb2TaggerCardinality(in.Cardinality)
	if err != nil {
		return nil, err
	}

	tags, err := tagger.Tag(entityID, cardinality)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.FetchEntityResponse{
		Id:          in.Id,
		Cardinality: in.Cardinality,
		Tags:        tags,
	}, nil
}
