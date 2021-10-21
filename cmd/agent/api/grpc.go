// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	dsdReplay "github.com/DataDog/datadog-agent/pkg/dogstatsd/replay"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pbutils "github.com/DataDog/datadog-agent/pkg/proto/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/replay"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	hostutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	taggerStreamSendTimeout = 1 * time.Minute
)

type server struct {
	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentSecureServer
	configService *remoteconfig.Service
}

func (s *server) GetHostname(ctx context.Context, in *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := hostutil.GetHostname(ctx)
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *server) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}

// DogstatsdCaptureTrigger triggers a dogstatsd traffic capture for the
// duration specified in the request. If a capture is already in progress,
// an error response is sent back.
func (s *serverSecure) DogstatsdCaptureTrigger(ctx context.Context, req *pb.CaptureTriggerRequest) (*pb.CaptureTriggerResponse, error) {
	d, err := time.ParseDuration(req.GetDuration())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	err = common.DSD.Capture(req.GetPath(), d, req.GetCompressed())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	// wait for the capture to start
	for !common.DSD.TCapture.IsOngoing() {
		time.Sleep(500 * time.Millisecond)
	}

	p, err := common.DSD.TCapture.Path()
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	return &pb.CaptureTriggerResponse{Path: p}, nil
}

// DogstatsdSetTaggerState allows setting a captured tagger state in the
// Tagger facilities. This endpoint is used when traffic replays are in
// progress. An empty state or nil request will result in the Tagger
// capture state being reset to nil.
func (s *serverSecure) DogstatsdSetTaggerState(ctx context.Context, req *pb.TaggerState) (*pb.TaggerStateResponse, error) {
	// Reset and return if no state pushed
	if req == nil || req.State == nil {
		log.Debugf("API: empty request or state")
		tagger.ResetCaptureTagger()
		dsdReplay.SetPidMap(nil)
		return &pb.TaggerStateResponse{Loaded: false}, nil
	}

	// FiXME: we should perhaps lock the capture processing while doing this...
	t := replay.NewTagger()
	if t == nil {
		return &pb.TaggerStateResponse{Loaded: false}, fmt.Errorf("unable to instantiate state")
	}
	t.LoadState(req.State)

	log.Debugf("API: setting capture state tagger")
	tagger.SetCaptureTagger(t)
	dsdReplay.SetPidMap(req.PidMap)

	log.Debugf("API: loaded state successfully")

	return &pb.TaggerStateResponse{Loaded: true}, nil
}

// StreamTags subscribes to added, removed, or changed entities in the Tagger
// and streams them to clients as pb.StreamTagsResponse events. Filtering is as
// of yet not implemented.
func (s *serverSecure) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
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

	for {
		select {
		case events := <-eventCh:
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
		}
	}
}

// FetchEntity fetches an entity from the Tagger with the desired cardinality tags.
func (s *serverSecure) TaggerFetchEntity(ctx context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
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

func (s *serverSecure) GetConfigs(ctx context.Context, in *pb.GetConfigsRequest) (*pb.GetConfigsResponse, error) {
	if s.configService == nil {
		log.Debug("Remote configuration service not initialized")
		return nil, errors.New("remote configuration service not initialized")
	}

	configs, err := s.configService.GetConfigs(in.Product.String())
	if err != nil {
		return nil, err
	}

	return &pb.GetConfigsResponse{
		ConfigResponses: configs,
	}, nil
}

func (s *serverSecure) GetConfigUpdates(in *pb.SubscribeConfigRequest, out pb.AgentSecure_GetConfigUpdatesServer) error {
	if s.configService == nil {
		log.Debug("Remote configuration service not initialized")
		return errors.New("remote config service not initialized")
	}

	ctx := out.Context()
	configs := make(chan *pb.ConfigResponse, 1)

	log.Debugf("New remote configuration subscriber request for product %s", in.Product)
	subscriber := remoteconfig.NewSubscriber(in.Product, time.Second, func(config *pb.ConfigResponse) error {
		log.Debug("Pushing configuration for gRPC client")
		select {
		case configs <- config:
			log.Debug("Pushed configuration to gRPC client")
			return nil
		default:
			return errors.New("failed to notify gRPC subscriber")
		}
	})

	log.Debugf("New remote configuration subscriber for product %s", in.Product)
	s.configService.RegisterSubscriber(subscriber)
	defer s.configService.UnregisterSubscriber(subscriber)

	for {
		log.Debug("Streaming config to gRPC client")
		select {
		case config := <-configs:
			log.Debugf("Sending configuration for product %s", in.Product)
			if err := out.Send(config); err != nil {
				log.Errorf("error sending config event: %s", err)
				return nil
			}
		case <-ctx.Done():
			log.Infof("Unsubscribing gRPC client for product %s", in.Product)
			return nil
		}
	}
}

func init() {
	grpclog.SetLoggerV2(grpc.NewLogger())
}
