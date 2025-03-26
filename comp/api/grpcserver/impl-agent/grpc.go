// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agentimpl implements the grpc component interface for the core agent
package agentimpl

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	googleGrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Requires defines the dependencies for the grpc component
type Requires struct {
	compdef.In

	DogstatsdServer     dogstatsdServer.Component
	Capture             replay.Component
	PidMap              pidmap.Component
	SecretResolver      secrets.Component
	RcService           option.Option[rcservice.Component]
	RcServiceMRF        option.Option[rcservicemrf.Component]
	AuthToken           authtoken.Component
	Tagger              tagger.Component
	Cfg                 config.Component
	AutoConfig          autodiscovery.Component
	WorkloadMeta        workloadmeta.Component
	Collector           option.Option[collector.Component]
	RemoteAgentRegistry remoteagentregistry.Component
}

type server struct {
	authToken           authtoken.Component
	taggerComp          tagger.Component
	workloadMeta        workloadmeta.Component
	configService       option.Option[rcservice.Component]
	configServiceMRF    option.Option[rcservicemrf.Component]
	dogstatsdServer     dogstatsdServer.Component
	capture             replay.Component
	pidMap              pidmap.Component
	remoteAgentRegistry remoteagentregistry.Component
	autodiscovery       autodiscovery.Component
	configComp          config.Component
}

func (s *server) BuildServer() http.Handler {
	authInterceptor := grpcutil.AuthInterceptor(parseToken)

	maxMessageSize := s.configComp.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size")

	opts := []googleGrpc.ServerOption{
		googleGrpc.Creds(credentials.NewTLS(s.authToken.GetTLSServerConfig())),
		googleGrpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
		googleGrpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
		googleGrpc.MaxRecvMsgSize(maxMessageSize),
		googleGrpc.MaxSendMsgSize(maxMessageSize),
	}

	// event size should be small enough to fit within the grpc max message size
	maxEventSize := maxMessageSize / 2
	grpcServer := googleGrpc.NewServer(opts...)
	pb.RegisterAgentServer(grpcServer, &agentServer{})
	pb.RegisterAgentSecureServer(grpcServer, &serverSecure{
		configService:    s.configService,
		configServiceMRF: s.configServiceMRF,
		taggerServer:     taggerserver.NewServer(s.taggerComp, maxEventSize, s.configComp.GetInt("remote_tagger.max_concurrent_sync")),
		taggerComp:       s.taggerComp,
		// TODO(components): decide if workloadmetaServer should be componentized itself
		workloadmetaServer:  workloadmetaServer.NewServer(s.workloadMeta),
		dogstatsdServer:     s.dogstatsdServer,
		capture:             s.capture,
		pidMap:              s.pidMap,
		remoteAgentRegistry: s.remoteAgentRegistry,
		autodiscovery:       s.autodiscovery,
		configComp:          s.configComp,
	})

	return grpcServer
}

func (s *server) BuildGatewayMux(cmdAddr string) (http.Handler, error) {
	dopts := []googleGrpc.DialOption{googleGrpc.WithTransportCredentials(credentials.NewTLS(s.authToken.GetTLSClientConfig()))}
	ctx := context.Background()
	gwmux := runtime.NewServeMux()
	err := pb.RegisterAgentHandlerFromEndpoint(
		ctx, gwmux, cmdAddr, dopts)
	if err != nil {
		return nil, fmt.Errorf("error registering agent handler from endpoint %s: %v", cmdAddr, err)
	}

	err = pb.RegisterAgentSecureHandlerFromEndpoint(
		ctx, gwmux, cmdAddr, dopts)
	if err != nil {
		return nil, fmt.Errorf("error registering agent secure handler from endpoint %s: %v", cmdAddr, err)
	}

	return gwmux, nil
}

// Provides defines the output of the grpc component
type Provides struct {
	Comp grpc.Component
}

// NewComponent creates a new grpc component
func NewComponent(reqs Requires) (Provides, error) {
	provides := Provides{
		Comp: &server{
			authToken:           reqs.AuthToken,
			configService:       reqs.RcService,
			configServiceMRF:    reqs.RcServiceMRF,
			taggerComp:          reqs.Tagger,
			workloadMeta:        reqs.WorkloadMeta,
			dogstatsdServer:     reqs.DogstatsdServer,
			capture:             reqs.Capture,
			pidMap:              reqs.PidMap,
			remoteAgentRegistry: reqs.RemoteAgentRegistry,
			autodiscovery:       reqs.AutoConfig,
			configComp:          reqs.Cfg,
		},
	}
	return provides, nil
}

// parseToken parses the token and validate it for our gRPC API, it returns an empty
// struct and an error or nil
func parseToken(token string) (interface{}, error) {
	if subtle.ConstantTimeCompare([]byte(token), []byte(util.GetAuthToken())) == 0 {
		return struct{}{}, errors.New("Invalid session token")
	}

	// Currently this empty struct doesn't add any information
	// to the context, but we could potentially add some custom
	// type.
	return struct{}{}, nil
}
