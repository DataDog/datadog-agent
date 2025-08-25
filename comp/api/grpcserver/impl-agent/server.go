// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	autodiscoverystream "github.com/DataDog/datadog-agent/comp/core/autodiscovery/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	rarproto "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/proto"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	taggerProto "github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	dsdReplay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type agentServer struct {
	hostname hostnameinterface.Component

	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentSecureServer
	taggerServer        *taggerserver.Server
	taggerComp          tagger.Component
	workloadmetaServer  *workloadmetaServer.Server
	configService       option.Option[rcservice.Component]
	configServiceMRF    option.Option[rcservicemrf.Component]
	dogstatsdServer     dogstatsdServer.Component
	capture             dsdReplay.Component
	pidMap              pidmap.Component
	remoteAgentRegistry remoteagentregistry.Component
	autodiscovery       autodiscovery.Component
	configComp          config.Component
}

func (s *agentServer) GetHostname(ctx context.Context, _ *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := s.hostname.Get(ctx)
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *agentServer) AuthFuncOverride(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) TaggerStreamEntities(req *pb.StreamTagsRequest, srv pb.AgentSecure_TaggerStreamEntitiesServer) error {
	return s.taggerServer.TaggerStreamEntities(req, srv)
}

// TaggerGenerateContainerIDFromOriginInfo generates a container ID from the Origin Info.
// This function takes an Origin Info but only uses the ExternalData part of it, this is done for backward compatibility.
func (s *serverSecure) TaggerGenerateContainerIDFromOriginInfo(ctx context.Context, req *pb.GenerateContainerIDFromOriginInfoRequest) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	return s.taggerServer.TaggerGenerateContainerIDFromOriginInfo(ctx, req)
}

func (s *serverSecure) TaggerFetchEntity(ctx context.Context, req *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	return s.taggerServer.TaggerFetchEntity(ctx, req)
}

// DogstatsdCaptureTrigger triggers a dogstatsd traffic capture for the
// duration specified in the request. If a capture is already in progress,
// an error response is sent back.
func (s *serverSecure) DogstatsdCaptureTrigger(_ context.Context, req *pb.CaptureTriggerRequest) (*pb.CaptureTriggerResponse, error) {
	d, err := time.ParseDuration(req.GetDuration())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	p, err := s.capture.StartCapture(req.GetPath(), d, req.GetCompressed())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	return &pb.CaptureTriggerResponse{Path: p}, nil
}

// DogstatsdSetTaggerState allows setting a captured tagger state in the
// Tagger facilities. This endpoint is used when traffic replays are in
// progress. An empty state or nil request will result in the Tagger
// capture state being reset to nil.
func (s *serverSecure) DogstatsdSetTaggerState(_ context.Context, req *pb.TaggerState) (*pb.TaggerStateResponse, error) {
	// Reset and return if no state pushed
	if req == nil || req.State == nil {
		log.Debugf("API: empty request or state")
		s.pidMap.SetPidMap(nil)
		return &pb.TaggerStateResponse{Loaded: false}, nil
	}

	// FiXME: we should perhaps lock the capture processing while doing this...
	mockReq := taggerimpl.MockRequires{
		Config:    s.configComp,
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	fakeTagger := taggerimpl.NewMock(mockReq).Comp
	if fakeTagger == nil {
		return &pb.TaggerStateResponse{Loaded: false}, fmt.Errorf("unable to instantiate state")
	}
	state := make([]taggerTypes.Entity, 0, len(req.State))

	// better stores these as the native type
	for id, entity := range req.State {
		entityID, err := taggerProto.Pb2TaggerEntityID(entity.Id)
		if err != nil {
			log.Errorf("Error getting identity ID for %v: %v", id, err)
			continue
		}

		state = append(state, taggerTypes.Entity{
			ID:                          *entityID,
			HighCardinalityTags:         entity.HighCardinalityTags,
			OrchestratorCardinalityTags: entity.OrchestratorCardinalityTags,
			LowCardinalityTags:          entity.LowCardinalityTags,
			StandardTags:                entity.StandardTags,
		})
	}
	fakeTagger.LoadState(state)

	s.pidMap.SetPidMap(req.PidMap)

	log.Debugf("API: loaded state successfully")

	return &pb.TaggerStateResponse{Loaded: true}, nil
}

var rcNotInitializedErr = status.Error(codes.Unimplemented, "remote configuration service not initialized")
var mrfRcNotInitializedErr = status.Error(codes.Unimplemented, "MRF remote configuration service not initialized")

func (s *serverSecure) ClientGetConfigs(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigState(_ context.Context, _ *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ConfigGetState()
}

func (s *serverSecure) ClientGetConfigsHA(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	rcServiceMRF, isSet := s.configServiceMRF.Get()
	if !isSet || rcServiceMRF == nil {
		log.Debug(mrfRcNotInitializedErr.Error())
		return nil, mrfRcNotInitializedErr
	}
	return rcServiceMRF.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigStateHA(_ context.Context, _ *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	rcServiceMRF, isSet := s.configServiceMRF.Get()
	if !isSet || rcServiceMRF == nil {
		log.Debug(mrfRcNotInitializedErr.Error())
		return nil, mrfRcNotInitializedErr
	}
	return rcServiceMRF.ConfigGetState()
}

func (s *serverSecure) ResetConfigState(_ context.Context, _ *emptypb.Empty) (*pb.ResetStateConfigResponse, error) {
	rcService, isSet := s.configService.Get()

	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ConfigResetState()
}

// WorkloadmetaStreamEntities streams entities from the workloadmeta store applying the given filter
func (s *serverSecure) WorkloadmetaStreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}

func (s *serverSecure) RegisterRemoteAgent(_ context.Context, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	if s.remoteAgentRegistry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	registration := rarproto.ProtobufToRemoteAgentRegistration(in)
	recommendedRefreshIntervalSecs, err := s.remoteAgentRegistry.RegisterRemoteAgent(registration)
	if err != nil {
		return nil, err
	}

	return &pb.RegisterRemoteAgentResponse{
		RecommendedRefreshIntervalSecs: recommendedRefreshIntervalSecs,
	}, nil
}

func (s *serverSecure) AutodiscoveryStreamConfig(_ *emptypb.Empty, out pb.AgentSecure_AutodiscoveryStreamConfigServer) error {
	return autodiscoverystream.Config(s.autodiscovery, out)
}

func (s *serverSecure) GetHostTags(ctx context.Context, _ *pb.HostTagRequest) (*pb.HostTagReply, error) {
	tags := hosttags.Get(ctx, true, s.configComp)
	return &pb.HostTagReply{System: tags.System, GoogleCloudPlatform: tags.GoogleCloudPlatform}, nil
}

func init() {
	grpclog.SetLoggerV2(grpc.NewLogger())
}
