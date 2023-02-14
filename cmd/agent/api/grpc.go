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

	workloadmetaServer "github.com/DataDog/datadog-agent/pkg/workloadmeta/server"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	dsdReplay "github.com/DataDog/datadog-agent/pkg/dogstatsd/replay"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/replay"
	taggerserver "github.com/DataDog/datadog-agent/pkg/tagger/server"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type server struct {
	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentSecureServer
	taggerServer       *taggerserver.Server
	workloadmetaServer *workloadmetaServer.Server
	configService      *remoteconfig.Service
}

func (s *server) GetHostname(ctx context.Context, in *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := hostname.Get(ctx)
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

func (s *serverSecure) TaggerStreamEntities(req *pb.StreamTagsRequest, srv pb.AgentSecure_TaggerStreamEntitiesServer) error {
	return s.taggerServer.TaggerStreamEntities(req, srv)
}

func (s *serverSecure) TaggerFetchEntity(ctx context.Context, req *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	return s.taggerServer.TaggerFetchEntity(ctx, req)
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

func (s *serverSecure) ClientGetConfigs(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	if s.configService == nil {
		log.Debug("Remote configuration service not initialized")
		return nil, errors.New("remote configuration service not initialized")
	}
	return s.configService.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigState(ctx context.Context, e *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	if s.configService == nil {
		log.Debug("Remote configuration service not initialized")
		return nil, errors.New("remote configuration service not initialized")
	}
	return s.configService.ConfigGetState()
}

// WorkloadmetaStreamEntities streams entities from the workloadmeta store applying the given filter
func (s *serverSecure) WorkloadmetaStreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}

func init() {
	grpclog.SetLoggerV2(grpc.NewLogger())
}
